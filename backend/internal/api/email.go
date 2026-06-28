package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/bitllow/sild/backend/internal/apiutil"
	"github.com/bitllow/sild/backend/internal/httpx"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/gin-gonic/gin"
)

// emailInbound: POST /v1/email/inbound (§6.2). The provider posts a parsed email
// here; the tenant is resolved by recipient domain and the signature is verified
// inside the domain layer. Unauthenticated (signature is the gate).
func (h *Handler) emailInbound(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.BadRequest(c, "unreadable body")
		return
	}
	var body struct {
		Recipient string `json:"recipient"`
		From      string `json:"from"`
		Subject   string `json:"subject"`
		Text      string `json:"text"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		httpx.BadRequest(c, "invalid body")
		return
	}
	in := mail.InboundEmail{
		Recipient: body.Recipient, From: body.From, Subject: body.Subject,
		TextBody: body.Text, RawBody: raw,
		Headers: map[string]string{"X-Signature": c.GetHeader("X-Signature")},
	}
	if _, err := h.svc.HandleInbound(c.Request.Context(), in); err != nil {
		apiutil.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "accepted"})
}
