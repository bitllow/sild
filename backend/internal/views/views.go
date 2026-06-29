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

// Member renders a conversation member.
func Member(m *models.ConversationMember) map[string]any {
	out := map[string]any{
		"member_kind": m.MemberKind,
		"conv_role":   m.ConvRole,
		"metadata":    rawJSON(m.Metadata),
		"joined_at":   m.JoinedAt,
	}
	participantID(out, m.ExternalUserID, m.InternalActorID)
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
func QueueRow(it *store.QueueItem) map[string]any {
	conv := Conversation(&it.Conversation, it.Members, nil)
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
func Conversation(c *models.Conversation, members []models.ConversationMember, assignment *models.Assignment) map[string]any {
	out := map[string]any{
		"id":         c.ID,
		"status":     c.Status,
		"reference":  c.Reference,
		"metadata":   rawJSON(c.Metadata),
		"created_at": c.CreatedAt,
	}
	ms := make([]map[string]any, 0, len(members))
	for i := range members {
		ms = append(ms, Member(&members[i]))
	}
	out["members"] = ms
	if assignment != nil {
		out["assignment"] = Assignment(assignment)
	}
	return out
}
