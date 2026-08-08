package api

import (
	"encoding/json"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/gin-gonic/gin"
)

// getPushConfig: GET /v1/channels/push (§5.5). Returns the credential's identity
// and the notification settings — never the credential itself.
func (h *Handler) getPushConfig(c *gin.Context) {
	cfg, err := h.svc.GetPushConfig(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// setPushCredential: PUT /v1/channels/push/credential. The credential is checked
// against the provider before it is stored, so a tenant learns at save time.
func (h *Handler) setPushCredential(c *gin.Context) {
	var req struct {
		Credential json.RawMessage `json:"credential"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if err := h.svc.SetPushCredential(c.Request.Context(), apiutil.Tenant(c), req.Credential); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// deletePushCredential: DELETE /v1/channels/push/credential.
func (h *Handler) deletePushCredential(c *gin.Context) {
	if err := h.svc.DeletePushCredential(c.Request.Context(), apiutil.Tenant(c)); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// updatePushSettings: PATCH /v1/channels/push — what a nudge may reveal.
func (h *Handler) updatePushSettings(c *gin.Context) {
	var req struct {
		IncludeSender bool                    `json:"include_sender"`
		IncludeBody   bool                    `json:"include_body"`
		SenderSource  models.PushSenderSource `json:"sender_source"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	in := domain.PushSettings{IncludeSender: req.IncludeSender, IncludeBody: req.IncludeBody, SenderSource: req.SenderSource}
	if err := h.svc.SetPushSettings(c.Request.Context(), apiutil.Tenant(c), in); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// testPushSend: POST /v1/channels/push/test — deliver a labelled test nudge to
// one device, so setup can be confirmed before shipping.
func (h *Handler) testPushSend(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if err := h.svc.TestPushSend(c.Request.Context(), apiutil.Tenant(c), req.Token); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// setPushOptOut: PUT /v1/users/:userID/push — the host's backend suppressing
// nudges for one of its users. Survives the app re-registering.
func (h *Handler) setPushOptOut(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	err := h.svc.SetUserPush(c.Request.Context(), apiutil.Tenant(c), c.Param("userID"), req.Enabled)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// deleteUserPushTokens: DELETE /v1/users/:userID/push-tokens — account deletion.
// Distinct from an opt-out: the app may register again.
func (h *Handler) deleteUserPushTokens(c *gin.Context) {
	n, err := h.svc.DeleteUserPushTokens(c.Request.Context(), apiutil.Tenant(c), c.Param("userID"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}
