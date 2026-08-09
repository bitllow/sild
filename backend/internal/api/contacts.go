package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/policy"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/views"
	"github.com/gin-gonic/gin"
)

// Contacts are people, conversations are threads — both searchable, as separate
// resources. A contact's profile is stored once and shared by every conversation
// they are in; the directory below still lists only people the caller's
// conversations admit. A contact's history is GET /v1/conversations?participant=<id>.

// expandContacts declares what an `expand=contacts` path may name. A contact has
// no id — its wire identity is external_user_id — so `contacts.id` is a 400.
//
// Default is who the person is, served from the narrow table. The profile blob
// lives in its own table and is named explicitly (`contacts.metadata`, or one
// key of it) — that is the difference the dot exists to make.
var expandContacts = apiutil.Expandable{
	Resource: resourceContacts,
	Default:  []string{"external_user_id", "name"},
	Fields:   []string{"metadata"},
	Keyed:    []string{"metadata"},
}

// wantsProfiles reports that the expansion named the blob, so the page must read
// contacts_meta. Nothing else on a conversation surface touches it.
func wantsProfiles(ex apiutil.Expansion) bool { return ex.Wants(resourceContacts, "metadata") }

// contactView renders a directory entry: the stored profile plus the aggregates
// only the scoped directory query can produce. No display name — the client
// already derives one, and duplicating that rule would give two sources of truth.
func contactView(c *store.Contact) map[string]any {
	out := views.Contact(c.ExternalUserID, c.Metadata)
	out["last_activity"] = c.LastActivity
	out["conversation_count"] = c.ConversationCount
	return out
}

// listContacts: GET /v1/contacts?q=&limit=&cursor=
func (h *Handler) listContacts(c *gin.Context) {
	scope, ok := apiutil.Scope(c, policy.ContactsList)
	if !ok {
		return
	}
	if scope.DenyAll() {
		apiutil.RespondPage(c, resourceContacts, store.Page[map[string]any]{})
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
	items := make([]map[string]any, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, contactView(&res.Items[i]))
	}
	apiutil.RespondPage(c, resourceContacts, store.Page[map[string]any]{
		Items: items, NextCursor: res.NextCursor, HasMore: res.HasMore,
	})
}

// getContact: GET /v1/contacts/:external_user_id
func (h *Handler) getContact(c *gin.Context) {
	scope, ok := apiutil.Scope(c, policy.ContactsRead)
	if !ok {
		return
	}
	ext, ok := contactPathID(c)
	if !ok {
		return
	}
	contact, err := h.svc.GetContact(c.Request.Context(), apiutil.Tenant(c), scope, ext)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, contactView(contact))
}

// putMyContact: PUT /v1/contacts/me — the SDK's first call on start-up, scoped
// to the token subject so no one can rewrite anyone else's identity.
func (h *Handler) putMyContact(c *gin.Context) {
	h.upsertContact(c, apiutil.Subject(c))
}

// putContact: PUT /v1/contacts/:external_user_id — the host's own backend
// writing a profile from its system of record, for any of its users.
func (h *Handler) putContact(c *gin.Context) {
	ext, ok := contactPathID(c)
	if !ok {
		return
	}
	h.upsertContact(c, ext)
}

// upsertContact replaces the profile whole: the configured metadata IS the
// profile, so a host never has to reason about what Sild merged it with.
func (h *Handler) upsertContact(c *gin.Context, externalUserID string) {
	if !apiutil.Authorize(c, policy.ContactsWrite) {
		return
	}
	var req struct {
		Metadata json.RawMessage `json:"metadata"`
		Locale   string          `json:"locale"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	// No metadata field asserts nothing about the profile — a widget recording only
	// its locale must not erase what the host's backend wrote. `null` still clears.
	if req.Metadata != nil {
		if err := h.svc.UpsertContact(c.Request.Context(), apiutil.Tenant(c), externalUserID, req.Metadata); err != nil {
			apiutil.Fail(c, err)
			return
		}
	}
	if req.Locale != "" {
		if err := h.svc.SetContactLocale(c.Request.Context(), apiutil.Tenant(c), externalUserID, req.Locale); err != nil {
			apiutil.Fail(c, err)
			return
		}
	}
	c.Status(http.StatusNoContent)
}

// setContactPush: PUT /v1/contacts/:external_user_id/push — the host's backend
// suppressing nudges for one of its users. Survives the app re-registering, and
// a profile write never disturbs it.
func (h *Handler) setContactPush(c *gin.Context) {
	ext, ok := contactPathID(c)
	if !ok {
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if err := h.svc.SetContactPush(c.Request.Context(), apiutil.Tenant(c), ext, req.Enabled); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// deleteContactPushTokens: DELETE /v1/contacts/:external_user_id/push-tokens —
// account deletion. Distinct from an opt-out: the app may register again.
func (h *Handler) deleteContactPushTokens(c *gin.Context) {
	ext, ok := contactPathID(c)
	if !ok {
		return
	}
	n, err := h.svc.DeleteUserPushTokens(c.Request.Context(), apiutil.Tenant(c), ext)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// contactPathID reads and validates the path identity. Same rule the write
// boundaries enforce; gin unescapes the path before matching, so a "/" sent as
// %2F never reaches here at all.
func contactPathID(c *gin.Context) (string, bool) {
	ext := c.Param("external_user_id")
	if err := domain.ValidateExternalUserID(ext); err != nil {
		apiutil.Fail(c, err)
		return "", false
	}
	return ext, true
}

// contactsBlock renders the `expand=contacts` block for a page's participants,
// from what the page already loaded. Profiles only: the directory aggregates
// need the scoped query the contacts resource itself is for, and computing them
// per page would restore the fan-out this removes.
//
// external_user_id is always carried: without it the block is a list of profiles
// nobody can attribute to a person.
func contactsBlock(ids []string, names map[string]string, profiles map[string][]byte, ex apiutil.Expansion) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		block := map[string]any{"external_user_id": id}
		if ex.Wants(resourceContacts, "name") && names[id] != "" {
			block["name"] = names[id]
		}
		if meta := selectedMetadata(profiles[id], ex); meta != nil {
			block["metadata"] = meta
		}
		out = append(out, block)
	}
	return out
}

// selectedMetadata narrows a stored profile to the keys the path named, or
// returns it whole. An absent key is simply absent — a profile no writer
// asserted must not read as one they did.
func selectedMetadata(blob []byte, ex apiutil.Expansion) any {
	if !ex.Wants(resourceContacts, "metadata") || len(blob) == 0 {
		return nil
	}
	keys := ex.Keys(resourceContacts, "metadata")
	if keys == nil {
		return json.RawMessage(blob)
	}
	var all map[string]json.RawMessage
	if json.Unmarshal(blob, &all) != nil {
		return nil
	}
	picked := map[string]json.RawMessage{}
	for k := range keys {
		if v, ok := all[k]; ok {
			picked[k] = v
		}
	}
	if len(picked) == 0 {
		return nil
	}
	return picked
}
