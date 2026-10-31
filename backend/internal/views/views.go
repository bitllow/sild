// Package views renders models into the JSON shapes used by REST responses
// (§4), realtime envelopes (§5.3), and webhook payloads (§6.1) — one definition
// so all three stay consistent.
package views

import (
	"encoding/json"
	"maps"

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

// Message renders a message (§5.3 message.created payload) without the
// internal step fields, for any reader who may not see internal notes.
func Message(m *models.Message, urlFn URLFunc) map[string]any {
	return message(m, urlFn, false)
}

// MessageFull renders a message with its internal step fields, for operators
// and the message's author.
func MessageFull(m *models.Message, urlFn URLFunc) map[string]any {
	return message(m, urlFn, true)
}

func message(m *models.Message, urlFn URLFunc, full bool) map[string]any {
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
	if len(m.Parts) > 0 {
		var parts []map[string]any
		if json.Unmarshal(m.Parts, &parts) == nil {
			if !full {
				parts, _ = stripParts(parts)
			}
			out["parts"] = parts
		}
	}
	if m.UpdatedAt != nil {
		out["updated_at"] = *m.UpdatedAt
	}
	return out
}

// WithoutInternalParts returns a rendered message without internal step fields,
// and whether any were removed. The input is never modified.
func WithoutInternalParts(msg map[string]any) (map[string]any, bool) {
	parts, stripped := stripParts(partList(msg["parts"]))
	if !stripped {
		return msg, false
	}
	out := maps.Clone(msg)
	out["parts"] = parts
	return out, true
}

// Part fields whose items may carry an internal field.
var internalLists = []string{"steps", "evidence"}

// stripParts returns a copy of parts without the internal fields of their items.
func stripParts(parts []map[string]any) ([]map[string]any, bool) {
	out := make([]map[string]any, 0, len(parts))
	stripped := false
	for _, p := range parts {
		cp := p
		for _, key := range internalLists {
			items, ok := p[key].([]any)
			if !ok {
				continue
			}
			clean, removed := withoutInternal(items)
			if !removed {
				continue
			}
			stripped = true
			cp = maps.Clone(cp)
			cp[key] = clean
		}
		out = append(out, cp)
	}
	return out, stripped
}

func withoutInternal(items []any) ([]any, bool) {
	clean := make([]any, 0, len(items))
	removed := false
	for _, it := range items {
		if m, ok := it.(map[string]any); ok {
			if _, has := m["internal"]; has {
				m = maps.Clone(m)
				delete(m, "internal")
				it, removed = m, true
			}
		}
		clean = append(clean, it)
	}
	return clean, removed
}

// partList reads parts as rendered here or as decoded back from an archive.
func partList(v any) []map[string]any {
	switch ps := v.(type) {
	case []map[string]any:
		return ps
	case []any:
		out := make([]map[string]any, 0, len(ps))
		for _, p := range ps {
			if m, ok := p.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
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
	Names    map[string]string
	Agents   map[string]string
	// ExternalSubs maps a staff member to their identity at the tenant's
	// integrator, so a data provider can authorize the person, not the assistant.
	ExternalSubs map[string]string
}

// Contact renders a contact's stored profile. An absent profile omits the field
// rather than emitting null, so a person nobody described reads the same on
// every contact surface.
func Contact(externalUserID string, metadata []byte) map[string]any {
	out := map[string]any{"external_user_id": externalUserID}
	if len(metadata) > 0 {
		out["metadata"] = rawJSON(metadata)
	}
	return out
}

// Member renders a conversation member: who they are, not who they are to the
// tenant. The profile blob is a contacts resource, reached through
// `expand=contacts.metadata`; only the display name is inline, because every
// surface renders it and it costs no join.
func Member(m *models.ConversationMember, p Profiles) map[string]any {
	out := map[string]any{
		"member_kind": m.MemberKind,
		"conv_role":   m.ConvRole,
		"joined_at":   m.JoinedAt,
	}
	if name := memberName(m, p); name != "" {
		out["name"] = name
	}
	participantID(out, m.ExternalUserID, m.InternalActorID)
	// Only a requester's: an operator's integrator identity is no member's business.
	if m.InternalActorID != nil && m.ConvRole == models.RoleClient {
		if sub, ok := p.ExternalSubs[*m.InternalActorID]; ok {
			out["external_sub"] = sub
		}
	}
	return out
}

// memberName reads the narrow name for an end user and the actor's name for an
// agent, so a client renders one field whoever the participant is.
func memberName(m *models.ConversationMember, p Profiles) string {
	if m.ExternalUserID != nil {
		return p.Names[*m.ExternalUserID]
	}
	if m.InternalActorID == nil {
		return ""
	}
	return p.Agents[*m.InternalActorID]
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

// Conversation renders the full conversation (§4.1 fetch, §4.2 GET).
func Conversation(c *models.Conversation, members []models.ConversationMember, assignment *models.Assignment, p Profiles) map[string]any {
	out := map[string]any{
		"id":         c.ID,
		"status":     c.Status,
		"reference":  c.Reference,
		"metadata":   rawJSON(c.Metadata),
		"created_at": c.CreatedAt,
		"queue":      c.Queue,
	}
	if c.CloseReason != nil {
		out["close_reason"] = *c.CloseReason
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
