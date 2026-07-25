package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// Contacts are people, conversations are threads — both searchable, as separate
// resources. A contact's history is GET /v1/conversations?participant=<id>, so
// there is no /contacts/:id/conversations.

// contactView renders a contact. The response carries external_user_id and the
// winning metadata blob; the client derives the display name with the logic it
// already has, so there is one source of truth for what a person is called.
func contactView(c *store.Contact) gin.H {
	out := gin.H{
		"external_user_id":   c.ExternalUserID,
		"last_activity":      c.LastActivity,
		"conversation_count": c.ConversationCount,
	}
	if len(c.Metadata) > 0 {
		out["metadata"] = json.RawMessage(c.Metadata)
	}
	return out
}

// listContacts: GET /v1/contacts?q=&limit=&cursor=
func (h *Handler) listContacts(c *gin.Context) {
	scope := apiutil.Scope(c, policy.ContactsList)
	if scope.DenyAll() {
		apiutil.RespondPage(c, resourceContacts, store.Page[gin.H]{})
		return
	}
	page, ok := apiutil.PageParams(c, apiutil.PageDefaults{
		Resource: resourceContacts,
		Limit:    30,
		Sort:     store.SortLastActivity,
		Order:    store.OrderDesc,
	})
	if !ok {
		return
	}
	res, err := h.svc.ListContacts(c.Request.Context(), apiutil.Tenant(c), scope, store.ContactQuery{
		PageParams: page,
		Search:     c.Query("q"),
	})
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	items := make([]gin.H, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, contactView(&res.Items[i]))
	}
	apiutil.RespondPage(c, resourceContacts, store.Page[gin.H]{
		Items: items, NextCursor: res.NextCursor, HasMore: res.HasMore,
	})
}

// getContact: GET /v1/contacts/:external_user_id
func (h *Handler) getContact(c *gin.Context) {
	scope := apiutil.Scope(c, policy.ContactsRead)
	ext := c.Param("external_user_id")
	if !validExternalUserID(ext) {
		httpx.BadRequest(c, "invalid external_user_id")
		return
	}
	contact, err := h.svc.GetContact(c.Request.Context(), apiutil.Tenant(c), scope, ext)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, contactView(contact))
}

// validExternalUserID guards the path form. The id is host-supplied and opaque,
// so the charset rule is enforced where ids ENTER the system (token mint, member
// add, remap) — gin unescapes the path before matching, so an id containing "/"
// sent as %2F never reaches a handler that could reject it. This check is the
// second line, not the first.
func validExternalUserID(s string) bool {
	if s == "" || len(s) > 128 || !utf8.ValidString(s) {
		return false
	}
	if strings.ContainsRune(s, '/') || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}
