package search

import (
	"context"
	"strings"

	"gorm.io/gorm"
)

// buildFilters applies the structured filters + keyword conditions shared by
// both backends. likeOp is "LIKE" (portable, wrapped in LOWER) or "ILIKE"
// (Postgres). A conversation matches if ANY member's metadata matches (§4.3).
func buildFilters(db *gorm.DB, tenantID string, q Query, likeOp string) *gorm.DB {
	b := db.Table("conversations c").Where("c.tenant_id = ?", tenantID)

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
	for _, v := range q.Meta {
		b = b.Where(memberLikeExists("m.member_search_text", likeOp), like(v))
	}
	for _, kw := range q.Keywords {
		// keyword matches body OR member metadata text, AND'd across keywords.
		cond := "(" +
			existsLike("messages", "msg", "msg.body", likeOp) + " OR " +
			memberLikeExists("m.member_search_text", likeOp) + ")"
		b = b.Where(cond, like(kw), like(kw))
	}
	return b
}

func like(s string) string { return "%" + strings.ToLower(s) + "%" }

// existsLike builds an EXISTS(...col LIKE/ILIKE...) for a child table.
func existsLike(table, alias, col, op string) string {
	left := col
	right := "?"
	if op == "LIKE" {
		left = "LOWER(" + col + ")"
	}
	return "EXISTS (SELECT 1 FROM " + table + " " + alias +
		" WHERE " + alias + ".conversation_id = c.id AND " + left + " " + op + " " + right + ")"
}

func memberLikeExists(col, op string) string {
	left := col
	if op == "LIKE" {
		left = "LOWER(" + col + ")"
	}
	return "EXISTS (SELECT 1 FROM conversation_members m WHERE m.conversation_id = c.id AND m.left_at IS NULL AND " + left + " " + op + " ?)"
}

// collectHits runs the filtered query and attaches a snippet per conversation.
func collectHits(ctx context.Context, db, filtered *gorm.DB, q Query, likeOp string) (Results, error) {
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
			hit.Snippet = snippet(ctx, db, id, q.Keywords[0], likeOp)
		}
		res.Conversations = append(res.Conversations, hit)
	}
	return res, nil
}

// snippet returns one matching message body for the result row.
func snippet(ctx context.Context, db *gorm.DB, convID, kw, likeOp string) string {
	col := "body"
	if likeOp == "LIKE" {
		col = "LOWER(body)"
	}
	var body string
	db.WithContext(ctx).Table("messages").
		Where("conversation_id = ? AND "+col+" "+likeOp+" ?", convID, like(kw)).
		Order("id DESC").Limit(1).Pluck("body", &body)
	return body
}
