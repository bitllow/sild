// Package search implements the admin mixed-token search (§4.3): one bar with
// field:value qualifiers + free keywords. The Backend interface has a Postgres
// trigram implementation and a portable LIKE fallback (capability tiers).
package search

import "strings"

// Query is a parsed search bar string.
type Query struct {
	Status   *string           // conversations.status
	Assignee *string           // assignments.assignee_actor_id ("me" → caller)
	Role     *string           // conversation_members.conv_role
	Channel  *string           // messages.channel
	Meta     map[string]string // member-metadata filters (phone, app_version, meta.<key>)
	Keywords []string          // free text → partial match on body + member text

	Before string
	Limit  int
}

// knownMetaShortcuts are field tokens that map onto member-metadata keys (§4.3).
var knownMetaShortcuts = map[string]bool{"phone": true, "app_version": true}

// Parse tokenizes a raw bar string. field:value tokens become structured
// filters; everything else is a keyword. Unknown field: prefixes are treated as
// literal keywords and never error (§4.3).
func Parse(raw string) Query {
	q := Query{Meta: map[string]string{}}
	for _, tok := range strings.Fields(raw) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok || val == "" {
			q.Keywords = append(q.Keywords, tok)
			continue
		}
		lk := strings.ToLower(key)
		switch {
		case lk == "status":
			v := strings.ToLower(val)
			q.Status = &v
		case lk == "assignee":
			v := val
			q.Assignee = &v
		case lk == "role":
			v := strings.ToLower(val)
			q.Role = &v
		case lk == "channel":
			v := strings.ToLower(val)
			q.Channel = &v
		case knownMetaShortcuts[lk]:
			q.Meta[lk] = val
		case strings.HasPrefix(lk, "meta."):
			q.Meta[strings.TrimPrefix(lk, "meta.")] = val
		default:
			// unknown field — treat the whole token as a literal keyword
			q.Keywords = append(q.Keywords, tok)
		}
	}
	return q
}

// ResolveAssignee replaces an "me" assignee with the calling agent id.
func (q *Query) ResolveAssignee(callerActorID string) {
	if q.Assignee != nil && *q.Assignee == "me" {
		q.Assignee = &callerActorID
	}
}
