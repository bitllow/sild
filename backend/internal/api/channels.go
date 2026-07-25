package api

import (
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

// ── Channels: email (§6.2, §8 Settings → Channels, owner/admin only) ─────────

// getEmailChannel returns the tenant's email-channel config for the Channels
// settings: the forwarding address an org points its support mailbox at, the
// verification status, and the per-tenant toggles.
func (h *Handler) getEmailChannel(c *gin.Context) {
	ch, version, err := h.svc.GetEmailChannelVersioned(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("ETag", version)
	c.JSON(http.StatusOK, emailChannelView(ch))
}

// updateEmailChannel applies the Channels-UI toggles / sender fields. Absent
// fields are left unchanged (PATCH semantics via pointers).
func (h *Handler) updateEmailChannel(c *gin.Context) {
	var req struct {
		AutoReply   *bool   `json:"auto_reply"`
		SpamFilter  *bool   `json:"spam_filter"`
		FromName    *string `json:"from_name"`
		FromAddress *string `json:"from_address"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	expect, ok := apiutil.RequireIfMatch(c)
	if !ok {
		return
	}
	ch, version, err := h.svc.UpdateEmailChannel(c.Request.Context(), apiutil.Tenant(c), domain.EmailChannelUpdate{
		AutoReply: req.AutoReply, SpamFilter: req.SpamFilter, FromName: req.FromName, FromAddress: req.FromAddress,
	}, expect)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	// The NEW version: without it a client stores null and falls back to
	// If-Match: * on every later edit, silently losing the protection.
	c.Header("ETag", version)
	c.JSON(http.StatusOK, emailChannelView(ch))
}

func emailChannelView(ch *domain.EmailChannel) gin.H {
	return gin.H{
		"channel":            "email",
		"forwarding_address": ch.ForwardingAddress,
		"inbound_domain":     ch.InboundDomain,
		"verified":           ch.Verified,
		"auto_reply":         ch.AutoReply,
		"spam_filter":        ch.SpamFilter,
		"from_name":          ch.FromName,
		"from_address":       ch.FromAddress,
	}
}
