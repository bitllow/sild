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

	if q.PeerOnly {
		// The peer surface's search spans every peer-kind conversation, open OR
		// closed — reading a closed peer conversation's history is authorized (only
		// WRITING is gated on open status, in PeerAgentSend), so it must stay
		// findable rather than vanishing the moment it closes. A status: filter in
		// the query still narrows it. Mirrors support search, which likewise spans
		// closed conversations.
		b = b.Where("c.kind = ?", "peer")
	} else {
		// Default (support) search must EXCLUDE peer conversations — otherwise a
		// non-peer-access agent could recover peer message/metadata content via the
		// shared search even though the peer list + message endpoints are gated.
		// Reads the stored kind, so a CLOSED peer conversation (still kind='peer')
		// does not leak here regardless of status.
		b = b.Where("c.kind = ?", "support")
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
		// member_search_text (fast, indexed for configured keys) OR live JSON
		// extraction (always works, slower) — §4.3 fallback for any key.
		cond, args := metaExists(dialect, k, like(v))
		b = b.Where(cond, args...)
	}
	for _, kw := range q.Keywords {
		// A keyword matches a message body, OR — for any active member — the
		// materialized member search text (the tenant's configured searchable
		// metadata keys) or the participant's external id.
		memberInner := wrap("m.member_search_text", op) + " " + op + " ? OR " +
			wrap("m.external_user_id", op) + " " + op + " ?"
		args := []any{like(kw), like(kw), like(kw)} // body, member_search_text, external_user_id
		if q.PeerOnly {
			// Peer participants are end users the operator is stepping in to help,
			// so peer search additionally matches the raw metadata as text — any
			// value (name, phone, plate) is findable even without configured keys.
			// This is deliberately NOT done for the default support search, which
			// stays bound to the tenant's searchable_metadata_keys allowlist.
			memberInner += " OR " + wrap(memberMetaText(dialect), op) + " " + op + " ?"
			args = []any{like(kw), like(kw), like(kw), like(kw)} // + raw metadata
		}
		memberCond := "EXISTS (SELECT 1 FROM conversation_members m WHERE m.conversation_id = c.id AND m.left_at IS NULL AND (" + memberInner + "))"
		cond := "(" + existsLike("messages", "msg", "msg.body", op) + " OR " + memberCond + ")"
		b = b.Where(cond, args...)
	}
	return b
}

// memberMetaText is the dialect-specific expression that exposes a member's raw
// metadata as text for a LIKE/ILIKE match — so any metadata value is searchable
// even when the tenant hasn't declared searchable_metadata_keys (member_search_text
// is empty then).
func memberMetaText(d config.Driver) string {
	switch d {
	case config.Postgres:
		return "m.metadata::text"
	case config.MySQL:
		return "CAST(m.metadata AS CHAR)"
	default: // sqlite stores JSON as TEXT
		return "m.metadata"
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

// metaExists matches a member-metadata key against the materialized search text
// OR a live JSON extraction of that exact key (§4.3 generic meta fallback). The
// key is parameterized — never interpolated — so it is injection-safe.
func metaExists(dialect config.Driver, key, likeVal string) (string, []any) {
	op := likeOpFor(dialect)
	textExpr := wrap("m.member_search_text", op)
	var jsonExpr, pathArg string
	switch dialect {
	case config.Postgres:
		jsonExpr = "(m.metadata ->> ?)"
		pathArg = key
	case config.MySQL:
		jsonExpr = "JSON_UNQUOTE(JSON_EXTRACT(m.metadata, ?))"
		pathArg = "$." + key
	default: // sqlite
		jsonExpr = "json_extract(m.metadata, ?)"
		pathArg = "$." + key
	}
	cond := "EXISTS (SELECT 1 FROM conversation_members m WHERE m.conversation_id = c.id AND m.left_at IS NULL AND (" +
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
	if err := filtered.Order("c.created_at DESC").Limit(q.Limit).Pluck("c.id", &ids).Error; err != nil {
		return Results{}, err
	}
	res := Results{Conversations: make([]ConversationHit, 0, len(ids))}
	for _, id := range ids {
		hit := ConversationHit{ConversationID: id}
		if len(q.Keywords) > 0 {
			hit.Snippet = snippet(ctx, db, id, q.Keywords[0], op)
		}
		res.Conversations = append(res.Conversations, hit)
	}
	return res, nil
}

// snippet returns one matching message body for the result row.
func snippet(ctx context.Context, db *gorm.DB, convID, kw, op string) string {
	var body string
	db.WithContext(ctx).Table("messages").
		Where("conversation_id = ? AND "+wrap("body", op)+" "+op+" ?", convID, like(kw)).
		Order("id DESC").Limit(1).Pluck("body", &body)
	return body
}
