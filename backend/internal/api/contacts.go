package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/gin-gonic/gin"
)

// Contacts are people, conversations are threads — both searchable, as separate
// resources. A contact's history is GET /v1/conversations?participant=<id>.

// contactView renders a contact. No display name: the client already derives one,
// and duplicating that rule would give two sources of truth.
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
	scope, ok := apiutil.Scope(c, policy.ContactsList)
	if !ok {
		return
	}
	if scope.DenyAll() {
		apiutil.RespondPage(c, resourceContacts, store.Page[gin.H]{})
		return
	}
	page, ok := apiutil.PageParams(c, apiutil.PageDefaults{
		Resource:   resourceContacts,
		Limit:      30,
		Sort:       store.SortLastActivity,
		Order:      store.OrderDesc,
		FixedOrder: true,
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
	scope, ok := apiutil.Scope(c, policy.ContactsRead)
	if !ok {
		return
	}
	// Same rule the write boundaries enforce; gin unescapes the path before
	// matching, so a "/" sent as %2F never reaches here at all.
	ext := c.Param("external_user_id")
	if err := domain.ValidateExternalUserID(ext); err != nil {
		apiutil.Fail(c, err)
		return
	}
	contact, err := h.svc.GetContact(c.Request.Context(), apiutil.Tenant(c), scope, ext)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, contactView(contact))
}
