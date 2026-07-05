package domain

import (
	"context"
	"encoding/json"
	"errors"

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

// IsPeerConversation reports whether a conversation exists and carries no
// assignment (so it never entered the support queue). Used to admit peer_access
// agents at the authorization boundary and to gate implicit join.
func (s *Service) IsPeerConversation(ctx context.Context, tenantID, convID string) bool {
	if _, err := s.store.Conversations().Get(ctx, tenantID, convID); err != nil {
		return false
	}
	_, err := s.store.Assignments().GetByConversation(ctx, tenantID, convID)
	return errors.Is(err, store.ErrNotFound)
}

// ListPeerConversations returns the tenant's peer conversations (open, no
// assignment) newest-activity first, each as a conversation view with members +
// last_message. Mirrors ListContactConversations' shape but tenant-wide and
// assignment-less; reads the denormalized last-activity, never the messages table.
func (s *Service) ListPeerConversations(ctx context.Context, tenantID string) ([]map[string]any, error) {
	convs, err := s.store.Conversations().ListPeers(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(convs))
	for i := range convs {
		c := &convs[i]
		members, err := s.store.Members().ListActive(ctx, tenantID, c.ID)
		if err != nil {
			return nil, err
		}
		conv := views.Conversation(c, members, nil)
		lastAt := c.CreatedAt
		if c.LastMessageAt != nil {
			lastAt = *c.LastMessageAt
		}
		conv["last_activity"] = lastAt
		if c.LastMessagePreview != "" {
			conv["last_message"] = map[string]any{"body": c.LastMessagePreview, "created_at": c.LastMessageAt}
		}
		out = append(out, conv)
	}
	return out, nil
}

// PeerAgentSend posts an agent's message into a peer conversation, implicitly
// joining the operator on their first message (§ "implicit join"): the agent is
// added as an agent-kind participant and a system join-note is appended before
// the message. Subsequent messages skip the join. Returns the agent's message.
//
// This is the one peer-specific write path; reading peer messages reuses the
// shared GET /conversations/:id/messages (authz now admits peer_access agents).
func (s *Service) PeerAgentSend(ctx context.Context, tenantID, convID, adminID, body string, atts []AttachmentInput, clientMsgID string) (*models.Message, error) {
	if !s.IsPeerConversation(ctx, tenantID, convID) {
		return nil, ErrForbidden // not a peer conversation — use the support path
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
		SenderKind: models.SenderAgent, Internal: &adminID,
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
	if err := s.store.Members().Add(ctx, member); err != nil {
		return err
	}
	s.emit(ctx, realtime.Target{Conversation: convID},
		realtime.EventMemberAdded, convID,
		map[string]any{"internal_actor_id": adminID, "conv_role": "support", "member_kind": models.MemberAgent, "name": name})
	_ = s.fireWebhook(ctx, tenantID, convID, "member.added",
		map[string]any{"internal_actor_id": adminID, "conv_role": "support"})

	// System join-note. Authored by the agent's actor so end-user surfaces can
	// name who joined; SenderSystem is excluded from last-activity/preview.
	_, err = s.SendMessage(ctx, tenantID, convID, SendInput{
		SenderKind: models.SenderSystem, Internal: &adminID,
		Body: "joined to help. I can see the full history above.",
	})
	return err
}

func clientMsgIDPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
