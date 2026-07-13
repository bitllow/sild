package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/bitllow/sild/backend/internal/realtime"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
)

// Peer conversations are the same untyped conversation primitive as support —
// participants + messages — but with NO assignment and NO agent. They are direct
// chats between end-user parties (rider↔driver, rider↔rider, any tenant-defined
// role pair). "Support" is just a conversation that has gained an agent; a peer
// conversation is one that hasn't. Agents with peer_access observe them and may
// step in, which implicitly adds them as an agent participant.

// conversationIsPeer reports whether a conversation exists and is peer-kind. This
// reads the stored classifier (set once at creation), so it is a single indexed
// lookup and can't drift from the queue/search view of the same conversation.
func (s *Service) conversationIsPeer(ctx context.Context, tenantID, convID string) bool {
	c, err := s.store.Conversations().Get(ctx, tenantID, convID)
	return err == nil && c.Kind == models.KindPeer
}

// AgentAccess is the single-pass classification of a platform agent's access to a
// conversation, so AuthorizeConversation needn't issue overlapping lookups.
type AgentAccess struct {
	// Peer is true when the conversation (hot OR archived) is peer-kind — access
	// is then gated on the operator's peer_access, never on an assignment.
	Peer bool
	// SupportOK is true when the conversation is a support conversation the agent
	// may access: a live assignment, or an archived formerly-support conversation.
	SupportOK bool
}

// ClassifyAgentAccess resolves an agent's access to a conversation in ONE pass —
// a single hot lookup plus at most one tombstone lookup — the sole authz entry
// point for the agent branch (rather than several overlapping peer/assignment/
// archive predicates that made a naive sequence hit the tombstone twice on an
// archived-support read). Peer takes precedence: a peer conversation is always
// the peer path regardless of any (never-present) assignment.
func (s *Service) ClassifyAgentAccess(ctx context.Context, tenantID, convID string) AgentAccess {
	if conv, err := s.store.Conversations().Get(ctx, tenantID, convID); err == nil {
		if conv.Kind == models.KindPeer {
			return AgentAccess{Peer: true}
		}
		// Hot support conversation: agents reach it only if it carries an assignment.
		return AgentAccess{SupportOK: s.HasAssignment(ctx, tenantID, convID)}
	}
	// Not hot — a tombstone (archived) still carries the durable kind.
	if tomb, ok := s.tombstone(ctx, tenantID, convID); ok {
		if tomb.Kind == models.KindPeer {
			return AgentAccess{Peer: true}
		}
		return AgentAccess{SupportOK: true} // archived formerly-support
	}
	return AgentAccess{}
}

// PeerPage is one keyset-paginated page of peer conversations for the inbox: the
// rendered rows plus the cursor + has-more flag driving infinite scroll.
type PeerPage struct {
	Conversations []map[string]any
	NextCursor    *store.QueueCursor
	HasMore       bool
}

// ListPeerConversations returns one page of the tenant's peer conversations
// (open, no assignment) newest-activity first, each as a conversation view with
// members + last_message. Same pagination/limits/search contract as the
// assignment queue — the peer surface is not a fetch-all. Members come batched
// from the store (no per-row query); reads denormalized last-activity only.
func (s *Service) ListPeerConversations(ctx context.Context, tenantID string, params store.PeerParams) (PeerPage, error) {
	page, err := s.store.Conversations().ListPeers(ctx, tenantID, params)
	if err != nil {
		return PeerPage{}, err
	}
	out := make([]map[string]any, 0, len(page.Items))
	for i := range page.Items {
		it := &page.Items[i]
		conv := views.Conversation(&it.Conversation, it.Members, nil)
		conv["last_activity"] = it.LastActivity
		if it.Conversation.LastMessagePreview != "" {
			conv["last_message"] = map[string]any{
				"body": it.Conversation.LastMessagePreview, "created_at": it.Conversation.LastMessageAt,
			}
		}
		out = append(out, conv)
	}
	return PeerPage{Conversations: out, NextCursor: page.NextCursor, HasMore: page.HasMore}, nil
}

// PeerAgentSend posts an agent's message into a peer conversation, implicitly
// joining the operator on their first message (§ "implicit join"): the agent is
// added as an agent-kind participant and a system join-note is appended before
// the message. Subsequent messages skip the join. Returns the agent's message.
//
// This is the one peer-specific write path; reading peer messages reuses the
// shared GET /conversations/:id/messages (authz now admits peer_access agents).
func (s *Service) PeerAgentSend(ctx context.Context, tenantID, convID, adminID, body string, atts []AttachmentInput, clientMsgID string) (*models.Message, error) {
	conv, err := s.store.Conversations().Get(ctx, tenantID, convID)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	if conv.Kind != models.KindPeer {
		return nil, ErrForbidden // a support conversation — use the support send path
	}
	if conv.Status != models.ConversationOpen {
		return nil, ErrForbidden // a closed peer conversation is read-only
	}
	// Reject an empty submit BEFORE any side effect: an implicit join is
	// user-visible (a "joined to help" note webhooked/emailed to both end-user
	// parties), so it must never fire for a message that carries nothing.
	if strings.TrimSpace(body) == "" && len(atts) == 0 {
		return nil, invalid("a message body or attachment is required")
	}
	// Validate the attachment keys up front — before the implicit join and before
	// completing any upload. Doing the join/complete first and letting SendMessage
	// reject an unknown key would leave a phantom join (member + broadcast join
	// note) and an orphaned completed upload behind a rejected send.
	for _, a := range atts {
		if _, err := s.store.Uploads().GetByObjectKey(ctx, tenantID, a.ObjectKey); err != nil {
			return nil, invalid("unknown attachment object_key")
		}
	}
	// Finalize the uploads (keys verified above; still after the peer/open guards
	// so a rejected send never commits an upload).
	for _, a := range atts {
		if err := s.CompleteUpload(ctx, tenantID, a.ObjectKey); err != nil {
			return nil, err
		}
	}

	members, err := s.store.Members().ListActive(ctx, tenantID, convID)
	if err != nil {
		return nil, err
	}
	joined := false
	for i := range members {
		m := &members[i]
		if m.MemberKind == models.MemberAgent && m.InternalActorID != nil && *m.InternalActorID == adminID {
			joined = true
			break
		}
	}

	if !joined {
		if err := s.agentJoinPeer(ctx, tenantID, convID, adminID); err != nil {
			return nil, err
		}
	}

	return s.SendMessage(ctx, tenantID, convID, SendInput{
		SenderKind: models.SenderAgent, Internal: &adminID, Kind: models.KindPeer,
		Body: body, Attachments: atts, ClientMsgID: clientMsgIDPtr(clientMsgID),
	})
}

// agentJoinPeer adds the operator as an agent participant and appends a system
// join-note, so both parties see who stepped in and the details panel lists the
// agent. The join-note is a system message (excluded from queue ordering) authored
// by the agent's actor, so the widget can render "<name> joined to help".
func (s *Service) agentJoinPeer(ctx context.Context, tenantID, convID, adminID string) error {
	name := s.AgentDisplayName(ctx, tenantID, adminID)
	meta, _ := json.Marshal(map[string]string{"name": name, "role": "support"})
	member, err := s.buildMember(ctx, tenantID, convID, MemberInput{
		UserID: adminID, Kind: models.MemberAgent, ConvRole: models.ConvRole("support"), Metadata: meta,
	})
	if err != nil {
		return err
	}
	// Deterministic id keyed on (conversation, operator) so two concurrent first
	// sends by the same operator (double-click / retry / a second tab) collide on
	// the primary key instead of both inserting. The insert is idempotent and only
	// the winner proceeds to broadcast the join + append the "joined to help" note,
	// so a race can't double-add the participant or double-post the note.
	member.ID = peerAgentMemberID(convID, adminID)
	added, err := s.store.Members().AddIfAbsent(ctx, member)
	if err != nil {
		return err
	}
	if !added {
		return nil // another concurrent send already joined this operator
	}
	// agentJoinPeer only ever runs for a peer conversation, so also fan the join
	// out to the peer channel where observing operators (non-members) watch.
	s.emit(ctx, realtime.Target{Conversation: convID, Peer: tenantID},
		realtime.EventMemberAdded, convID,
		map[string]any{"internal_actor_id": adminID, "conv_role": "support", "member_kind": models.MemberAgent, "name": name})
	_ = s.fireWebhook(ctx, tenantID, convID, "member.added",
		map[string]any{"internal_actor_id": adminID, "conv_role": "support"})

	// System join-note. Authored by the agent's actor so end-user surfaces can
	// name who joined; SenderSystem is excluded from last-activity/preview.
	_, err = s.SendMessage(ctx, tenantID, convID, SendInput{
		SenderKind: models.SenderSystem, Internal: &adminID, Kind: models.KindPeer,
		Body: "joined to help. I can see the full history above.",
	})
	return err
}

// peerAgentMemberID derives a stable participant id for an operator stepping into
// a peer conversation, so the implicit-join insert is idempotent on the primary
// key (see agentJoinPeer). 32-byte digest → 64 hex chars, trimmed to fit size:40.
func peerAgentMemberID(convID, adminID string) string {
	sum := sha256.Sum256([]byte(convID + "|" + adminID))
	return "m_" + hex.EncodeToString(sum[:])[:38]
}

func clientMsgIDPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
