package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/auth"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// Embedded sign-in error codes.
const (
	// CodeAccessDenied is a token refused by the validator or mapping to no usable member.
	CodeAccessDenied = "access_denied"
	// CodeValidatorFailed is a validator that gave no usable answer.
	CodeValidatorFailed = "validator_failed"
)

// exchangeEmbedded: POST /v1/admin/auth/embedded — trade the host app's token
// for an embedded session bearer.
func (h *Handler) exchangeEmbedded(c *gin.Context) {
	var req struct {
		EmbedKey string `json:"embed_key"`
		Token    string `json:"token"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if req.EmbedKey == "" || req.Token == "" {
		httpx.BadRequest(c, "embed_key and token are required")
		return
	}
	sess, err := h.svc.ExchangeEmbedToken(c.Request.Context(), req.EmbedKey, req.Token)
	var denied *domain.EmbedDenied
	var failed *auth.ValidatorError
	switch {
	case errors.As(err, &denied):
		httpx.FieldError(c, http.StatusForbidden, CodeAccessDenied, denied.Error(),
			map[string]string{"reason": denied.Reason, "sub": denied.Sub})
		return
	case errors.As(err, &failed):
		log.Printf("embedded sign-in: embed key %s: %v", req.EmbedKey, failed)
		httpx.Error(c, http.StatusBadGateway, CodeValidatorFailed, "your app's sign-in check did not answer; try again shortly")
		return
	case errors.Is(err, domain.ErrUnknownEmbedKey):
		httpx.Unauthorized(c, "unknown embed key")
		return
	case err != nil:
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"token": sess.Token, "expires_at": sess.ExpiresAt, "external_sub": sess.ExternalSub,
		"admin_id": sess.Admin.ID,
	})
}

// embedOrigins: GET /v1/admin/auth/embedded/:embed_key — the origins allowed to
// frame the inbox for this key, and to hand it a token.
func (h *Handler) embedOrigins(c *gin.Context) {
	origins, err := h.svc.EmbedOrigins(c.Request.Context(), c.Param("embed_key"))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	c.JSON(http.StatusOK, gin.H{"allowed_origins": origins})
}

func (h *Handler) getIdentityProvider(c *gin.Context) {
	v, err := h.svc.GetIdentityProvider(c.Request.Context(), apiutil.Tenant(c))
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

// putIdentityProvider: PUT /v1/identity-provider — replace the config whole. The
// validator token is write-only; omit it to keep the stored one.
func (h *Handler) putIdentityProvider(c *gin.Context) {
	var in domain.IdentityProviderInput
	if !httpx.DecodeJSON(c, &in) {
		return
	}
	v, err := h.svc.SetIdentityProvider(c.Request.Context(), apiutil.Tenant(c), in)
	if err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, v)
}

func (h *Handler) deleteIdentityProvider(c *gin.Context) {
	if err := h.svc.DeleteIdentityProvider(c.Request.Context(), apiutil.Tenant(c)); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// setExternalSub: PUT /v1/team/:id/external-sub — link a member to their
// identity at the integrator; empty unlinks. Linking lets a token sign in as
// them, so it is guarded like a password reset.
func (h *Handler) setExternalSub(c *gin.Context) {
	var req struct {
		ExternalSub string `json:"external_sub"`
	}
	if !httpx.DecodeJSON(c, &req) {
		return
	}
	if !h.guardOwnerMutation(c, c.Param("id"), nil) {
		return
	}
	if err := h.svc.SetExternalSub(c.Request.Context(), apiutil.Tenant(c), c.Param("id"), req.ExternalSub); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// endEmbeddedSession revokes the embedded bearer on the request, if any.
func (h *Handler) endEmbeddedSession(c *gin.Context) {
	if raw := middleware.Bearer(c); strings.HasPrefix(raw, auth.EmbedSessionPrefix) {
		_ = h.svc.EndEmbedSession(c.Request.Context(), raw)
	}
}
