package domain_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func hmacSign(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func seedEmailTenant(t *testing.T, h *testutil.Harness, secret string) string {
	t.Helper()
	tenant := h.SeedTenant()
	cfg := &models.TenantEmailConfig{
		TenantID: tenant.ID, InboundDomain: "support.test",
		FromName: "Support", FromAddress: "support@support.test", SigningSecret: secret,
		AllowedDomains: []models.TenantEmailDomain{{TenantID: tenant.ID, Domain: "support.test"}},
	}
	if err := h.Store.Tenants().SetEmailConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	return tenant.ID
}

// §6.2: inbound with no token creates a conversation; a reply embeds the thread
// token; a follow-up inbound with that token threads into the same conversation.
func TestEmailThreadingRoundTrip(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "") // no secret → dev allows
	ctx := context.Background()

	msg, err := h.Svc.HandleInbound(ctx, mail.InboundEmail{
		Recipient: "help@support.test", From: "cust@x.com", Subject: "Need help", TextBody: "hello",
	})
	if err != nil {
		t.Fatalf("inbound create: %v", err)
	}
	if msg.Channel != models.ChannelEmail || msg.ConversationID == "" {
		t.Fatalf("unexpected inbound message: %+v", msg)
	}
	convID := msg.ConversationID

	thread, err := h.Store.Email().Get(ctx, tenantID, convID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}

	// Agent replies (participants) → outbound email to the customer.
	agentID := "agent_1"
	if _, err := h.Svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &agentID, Body: "How can I help?",
		Visibility: models.VisibilityParticipants, AllowInternal: true,
	}); err != nil {
		t.Fatalf("agent reply: %v", err)
	}
	sent := h.Mailer.Messages()
	if len(sent) != 1 || sent[0].To != "cust@x.com" || !strings.Contains(sent[0].Subject, thread.ThreadToken) {
		t.Fatalf("expected outbound email to customer with token, got %+v", sent)
	}

	// Customer replies; subject carries the token → threads into same conv.
	msg2, err := h.Svc.HandleInbound(ctx, mail.InboundEmail{
		Recipient: "help@support.test", From: "cust@x.com",
		Subject: "Re: Need help [sild#" + thread.ThreadToken + "]", TextBody: "thanks",
	})
	if err != nil {
		t.Fatalf("inbound reply: %v", err)
	}
	if msg2.ConversationID != convID {
		t.Fatalf("reply should thread into %s, got %s", convID, msg2.ConversationID)
	}
}

// §6.2: the inbound endpoint requires a valid provider signature when a secret
// is configured.
func TestEmailInboundSignatureGate(t *testing.T) {
	h := testutil.New(t)
	seedEmailTenant(t, h, "topsecret")

	body := []byte(`{"recipient":"help@support.test","from":"cust@x.com","subject":"hi","text":"x"}`)

	// no signature → rejected
	w := h.Request("POST", "/v1/email/inbound").Raw(body, "application/json").Do()
	if w.Code == http.StatusOK {
		t.Fatalf("unsigned inbound must be rejected, got %d", w.Code)
	}

	// valid signature → accepted
	sig := "sha256=" + hmacSign("topsecret", body)
	w = h.Request("POST", "/v1/email/inbound").Raw(body, "application/json").Header("X-Signature", sig).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("signed inbound should be accepted, got %d %s", w.Code, w.Body)
	}
}
