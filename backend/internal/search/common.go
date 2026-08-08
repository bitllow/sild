package search

import (
	"context"
	"strings"

	"github.com/bitllow/sild/backend/internal/config"
	"gorm.io/gorm"
)

// likeOpFor returns the case-insensitive match operator for a dialect.
func likeOpFor(d config.Driver) string {
	if d == config.Postgres {
		return "ILIKE"
	}
	return "LIKE" // wrapped in LOWER() for portability
}

// buildFilters applies the structured filters + keyword conditions shared by both
// backends. A conversation matches if ANY member's metadata matches (§4.3).
func buildFilters(db *gorm.DB, tenantID string, q Query, dialect config.Driver) *gorm.DB {
	op := likeOpFor(dialect)
	b := db.Table("conversations c").Where("c.tenant_id = ?", tenantID)

	// The kind restriction comes from the policy scope, not from a flag decided
	// here. It is a security boundary: without it a non-peer-access agent could
	// recover peer message and metadata content through the shared search even
	// though the peer list and message endpoints are gated. Reading the stored
	// kind means a CLOSED peer conversation does not leak regardless of status.
	// Search spans open AND closed conversations; a status: token still narrows.
	if len(q.Kinds) > 0 {
		b = b.Where("c.kind IN ?", q.Kinds)
	}
	// The rest of the authorization ceiling. Applied here, not after hydration:
	// filtering a page of already-limited ids yields sparse pages and hides later
	// matches.
	if q.Participant != "" {
		b = b.Where(`EXISTS (SELECT 1 FROM conversation_members pm
			WHERE pm.conversation_id = c.id AND pm.left_at IS NULL
			  AND pm.external_user_id = ?)`, q.Participant)
	}
	if q.RequiresAssignment {
		b = b.Where(`c.kind <> 'support' OR c.archived_at IS NOT NULL
			OR EXISTS (SELECT 1 FROM assignments ra WHERE ra.conversation_id = c.id)`)
	}
	if q.Status != nil {
		b = b.Where("c.status = ?", *q.Status)
	}
	if q.Assignee != nil {
		b = b.Where("EXISTS (SELECT 1 FROM assignments a WHERE a.conversation_id = c.id AND a.assignee_actor_id = ?)", *q.Assignee)
	}
	if q.Role != nil {
		b = b.Where("EXISTS (SELECT 1 FROM conversation_members m WHERE m.conversation_id = c.id AND m.left_at IS NULL AND m.conv_role = ?)", *q.Role)
	}
	if q.Channel != nil {
		b = b.Where("EXISTS (SELECT 1 FROM messages msg WHERE msg.conversation_id = c.id AND msg.channel = ?)", *q.Channel)
	}
	for k, v := range q.Meta {
		// contacts.search_text (fast, indexed for configured keys) OR live JSON
		// extraction (always works, slower) — §4.3 fallback for any key.
		cond, args := metaExists(dialect, k, like(v))
		b = b.Where(cond, args...)
	}
	for _, kw := range q.Keywords {
		// A keyword matches a message body, OR — for any active member — the
		// materialized member search text (the tenant's configured searchable
		// metadata keys) or the participant's external id.
		bodyCond := existsLike("messages", "msg", "msg.body", op)
		if !q.IncludeInternal {
			// A note the caller cannot read must not be findable either (§5.6).
			bodyCond = `EXISTS (SELECT 1 FROM messages msg
				WHERE msg.conversation_id = c.id AND msg.visibility <> 'internal'
				  AND ` + wrap("msg.body", op) + " " + op + " ?)"
		}
		memberInner := wrap("ct.search_text", op) + " " + op + " ? OR " +
			wrap("m.external_user_id", op) + " " + op + " ?"
		join := contactJoin
		args := []any{like(kw), like(kw), like(kw)} // body, search_text, external_user_id
		if q.MatchRawMetadata {
			// Peer participants are end users the operator is stepping in to help,
			// so peer search additionally matches the raw profile as text — any value
			// (name, phone, plate) is findable even without configured keys. This is
			// deliberately NOT done for the default support search, which stays bound
			// to the tenant's searchable_metadata_keys allowlist, and it is the one
			// keyword path that pays for the blob table.
			memberInner += " OR " + wrap(contactMetaText(dialect), op) + " " + op + " ?"
			join += contactMetaJoin
			args = []any{like(kw), like(kw), like(kw), like(kw)} // + raw profile
		}
		memberCond := "EXISTS (SELECT 1 FROM conversation_members m" + join +
			" WHERE m.conversation_id = c.id AND m.left_at IS NULL AND (" + memberInner + "))"
		cond := "(" + bodyCond + " OR " + memberCond + ")"
		b = b.Where(cond, args...)
	}
	return b
}

// A member's profile lives on their contact row, so a match on it is a join
// outward. Both are LEFT joins: a participant nobody wrote a profile for is
// still findable by external id.
const (
	contactJoin = ` LEFT JOIN contacts ct
		ON ct.tenant_id = m.tenant_id AND ct.external_user_id = m.external_user_id`
	contactMetaJoin = ` LEFT JOIN contacts_meta cm
		ON cm.tenant_id = m.tenant_id AND cm.external_user_id = m.external_user_id`
)

// contactMetaText is the dialect-specific expression that exposes a stored
// profile as text for a LIKE/ILIKE match — so any value is searchable even when
// the tenant hasn't declared searchable_metadata_keys (search_text is empty then).
func contactMetaText(d config.Driver) string {
	switch d {
	case config.Postgres:
		return "cm.metadata::text"
	case config.MySQL:
		return "CAST(cm.metadata AS CHAR)"
	default: // sqlite stores JSON as TEXT
		return "cm.metadata"
	}
}

func like(s string) string { return "%" + strings.ToLower(s) + "%" }

func wrap(col, op string) string {
	if op == "LIKE" {
		return "LOWER(" + col + ")"
	}
	return col
}

// existsLike builds EXISTS(...col LIKE/ILIKE...) for a child table.
func existsLike(table, alias, col, op string) string {
	return "EXISTS (SELECT 1 FROM " + table + " " + alias +
		" WHERE " + alias + ".conversation_id = c.id AND " + wrap(col, op) + " " + op + " ?)"
}

// metaExists matches a profile key against the materialized search text OR a
// live JSON extraction of that exact key (§4.3 generic meta fallback). The key
// is parameterized — never interpolated — so it is injection-safe.
func metaExists(dialect config.Driver, key, likeVal string) (string, []any) {
	op := likeOpFor(dialect)
	textExpr := wrap("ct.search_text", op)
	var jsonExpr, pathArg string
	switch dialect {
	case config.Postgres:
		jsonExpr = "(cm.metadata ->> ?)"
		pathArg = key
	case config.MySQL:
		jsonExpr = "JSON_UNQUOTE(JSON_EXTRACT(cm.metadata, ?))"
		pathArg = "$." + key
	default: // sqlite
		jsonExpr = "json_extract(cm.metadata, ?)"
		pathArg = "$." + key
	}
	cond := "EXISTS (SELECT 1 FROM conversation_members m" + contactJoin + contactMetaJoin +
		" WHERE m.conversation_id = c.id AND m.left_at IS NULL AND (" +
		textExpr + " " + op + " ? OR " + wrap(jsonExpr, op) + " " + op + " ?))"
	return cond, []any{likeVal, pathArg, likeVal}
}

// collectHits runs the filtered query and attaches a snippet per conversation.
func collectHits(ctx context.Context, db, filtered *gorm.DB, q Query, dialect config.Driver) (Results, error) {
	op := likeOpFor(dialect)
	if q.Before != "" {
		filtered = filtered.Where("c.id < ?", q.Before)
	}
	var ids []string
	// Order by the same column the keyset predicate above filters on. ULIDs are
	// chronological, so this is still newest-first, but ordering by created_at
	// while paging on id would let rows sharing a timestamp be skipped.
	if err := filtered.Order("c.id DESC").Limit(q.Limit).Pluck("c.id", &ids).Error; err != nil {
		return Results{}, err
	}
	res := Results{Conversations: make([]ConversationHit, 0, len(ids))}
	for _, id := range ids {
		hit := ConversationHit{ConversationID: id}
		if len(q.Keywords) > 0 {
			hit.Snippet = snippet(ctx, db, id, q.Keywords[0], op, q.IncludeInternal)
		}
		res.Conversations = append(res.Conversations, hit)
	}
	return res, nil
}

// snippet returns one matching message body for the result row.
func snippet(ctx context.Context, db *gorm.DB, convID, kw, op string, includeInternal bool) string {
	var body string
	q := db.WithContext(ctx).Table("messages").
		Where("conversation_id = ? AND "+wrap("body", op)+" "+op+" ?", convID, like(kw))
	if !includeInternal {
		q = q.Where("visibility <> ?", "internal")
	}
	q.Order("id DESC").Limit(1).Pluck("body", &body)
	return body
}
