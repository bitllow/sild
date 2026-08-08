// Package views renders models into the JSON shapes used by REST responses
// (§4), realtime envelopes (§5.3), and webhook payloads (§6.1) — one definition
// so all three stay consistent.
package views

import (
	"encoding/json"

	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
)

// URLFunc mints a download URL for an attachment object key ("" to omit).
type URLFunc func(objectKey string) string

func rawJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

// participantID adds whichever identity column is set.
func participantID(m map[string]any, ext, internal *string) {
	if ext != nil {
		m["external_user_id"] = *ext
	}
	if internal != nil {
		m["internal_actor_id"] = *internal
	}
}

// Message renders a message (§5.3 message.created payload).
func Message(m *models.Message, urlFn URLFunc) map[string]any {
	out := map[string]any{
		"id":              m.ID,
		"conversation_id": m.ConversationID,
		"sender_kind":     m.SenderKind,
		"visibility":      m.Visibility,
		"channel":         m.Channel,
		"body":            m.Body,
		"created_at":      m.CreatedAt,
	}
	participantID(out, m.ExternalUserID, m.InternalActorID)
	if m.ClientMsgID != nil {
		out["client_msg_id"] = *m.ClientMsgID
	}
	atts := make([]map[string]any, 0, len(m.Attachments))
	for i := range m.Attachments {
		atts = append(atts, Attachment(&m.Attachments[i], urlFn))
	}
	out["attachments"] = atts
	return out
}

// Attachment renders an attachment with an optional signed URL (§11).
func Attachment(a *models.MessageAttachment, urlFn URLFunc) map[string]any {
	out := map[string]any{
		"object_key":  a.ObjectKey,
		"disposition": a.Disposition,
		"mime_type":   a.MimeType,
		"size_bytes":  a.SizeBytes,
		"filename":    a.Filename,
	}
	if urlFn != nil {
		if u := urlFn(a.ObjectKey); u != "" {
			out["url"] = u
		}
	}
	return out
}

// Profiles resolves the metadata a member view renders inline. Contacts are the
// stored profiles, keyed by external_user_id; Agents are operator display names,
// keyed by internal_actor_id — an agent has no contact row, so their member
// metadata is synthesized rather than joined.
type Profiles struct {
	Contacts map[string][]byte
	Agents   map[string]string
}

// Contact renders a contact's stored profile — the shape shared by the
// `contact.updated` event and the `expand=contacts` block.
func Contact(externalUserID string, metadata []byte) map[string]any {
	return map[string]any{
		"external_user_id": externalUserID,
		"metadata":         rawJSON(metadata),
	}
}

// Member renders a conversation member, with the participant's profile inline.
func Member(m *models.ConversationMember, p Profiles) map[string]any {
	out := map[string]any{
		"member_kind": m.MemberKind,
		"conv_role":   m.ConvRole,
		"metadata":    memberMetadata(m, p),
		"joined_at":   m.JoinedAt,
	}
	participantID(out, m.ExternalUserID, m.InternalActorID)
	return out
}

// memberMetadata joins the profile for an end user and synthesizes one for an
// agent, so the inbox reads one field whoever the participant is.
func memberMetadata(m *models.ConversationMember, p Profiles) any {
	if m.ExternalUserID != nil {
		return rawJSON(p.Contacts[*m.ExternalUserID])
	}
	if m.InternalActorID == nil {
		return nil
	}
	name := p.Agents[*m.InternalActorID]
	if name == "" {
		return nil
	}
	out := map[string]any{"name": name}
	if m.ConvRole != "" {
		out["role"] = string(m.ConvRole)
	}
	return out
}

// Assignment renders an assignment (§5.3 assignment.updated data is a subset).
func Assignment(a *models.Assignment) map[string]any {
	out := map[string]any{
		"id":              a.ID,
		"conversation_id": a.ConversationID,
		"status":          a.Status,
		"created_at":      a.CreatedAt,
	}
	if a.AssigneeActorID != nil {
		out["assignee_actor_id"] = *a.AssigneeActorID
	}
	if a.ClosedAt != nil {
		out["closed_at"] = a.ClosedAt
	}
	return out
}

// QueueRow renders one inbox queue row: the assignment + its conversation
// (members + last message preview + last activity), but NO message history —
// the client fetches that lazily when the conversation is opened (§4.3).
func QueueRow(it *store.QueueItem, p Profiles) map[string]any {
	conv := Conversation(&it.Conversation, it.Members, nil, p)
	conv["last_activity"] = it.LastActivity
	if it.Conversation.LastMessagePreview != "" {
		conv["last_message"] = map[string]any{
			"body":       it.Conversation.LastMessagePreview,
			"created_at": it.Conversation.LastMessageAt,
		}
	}
	return map[string]any{
		"assignment":   Assignment(&it.Assignment),
		"conversation": conv,
	}
}

// Conversation renders the full conversation (§4.1 fetch, §4.2 GET).
func Conversation(c *models.Conversation, members []models.ConversationMember, assignment *models.Assignment, p Profiles) map[string]any {
	out := map[string]any{
		"id":         c.ID,
		"status":     c.Status,
		"reference":  c.Reference,
		"metadata":   rawJSON(c.Metadata),
		"created_at": c.CreatedAt,
	}
	ms := make([]map[string]any, 0, len(members))
	for i := range members {
		ms = append(ms, Member(&members[i], p))
	}
	out["members"] = ms
	if assignment != nil {
		out["assignment"] = Assignment(assignment)
	}
	return out
}
