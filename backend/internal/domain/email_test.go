package domain_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// ── Forwarding daemon (§6.2): tenant resolved by the forwarding token in the
// recipient local part; trusted path (no provider signature). ───────────────

// seedForwardTenant configures a tenant with a forwarding token + toggles and
// returns (tenantID, forwardingAddress).
func seedForwardTenant(t *testing.T, h *testutil.Harness, token string, autoReply, spamFilter bool) (string, string) {
	t.Helper()
	tenant := h.SeedTenant()
	cfg := &models.TenantEmailConfig{
		TenantID: tenant.ID, InboundToken: token,
		FromName: "Support", FromAddress: "support@inbound.test",
		AutoReply: autoReply, SpamFilter: spamFilter,
	}
	if err := h.Store.Tenants().SetEmailConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	return tenant.ID, token + "@inbound.test"
}

func TestHandleForwardedCreatesAndVerifies(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_route1", false, true)
	ctx := context.Background()

	msg, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{
		Recipient: addr, From: "cust@x.com", Subject: "Help me", TextBody: "hello",
	})
	if err != nil {
		t.Fatalf("forwarded create: %v", err)
	}
	if msg == nil || msg.Channel != models.ChannelEmail || msg.ConversationID == "" {
		t.Fatalf("unexpected message: %+v", msg)
	}
	// The conversation is queued for an agent (an assignment exists).
	if a, err := h.Store.Assignments().GetByConversation(ctx, tenantID, msg.ConversationID); err != nil || a == nil {
		t.Fatalf("expected a queued assignment: %v", err)
	}
	// First successful ingest verifies the forwarding setup.
	cfg, err := h.Store.Tenants().GetEmailConfig(ctx, tenantID)
	if err != nil || !cfg.Verified {
		t.Fatalf("expected verified=true after first ingest, got %+v (err %v)", cfg, err)
	}
}

func TestHandleForwardedThreadsAndSubaddress(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_route2", false, true)
	ctx := context.Background()

	first, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: "Hi", TextBody: "one"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	thread, err := h.Store.Email().Get(ctx, tenantID, first.ConversationID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}

	// A reply carrying the token, sent to a +subaddressed form of the forwarding
	// address, threads into the same conversation.
	plusAddr := "eml_route2+inbox@inbound.test"
	second, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{
		Recipient: plusAddr, From: "cust@x.com",
		Subject: "Re: Hi [sild#" + thread.ThreadToken + "]", TextBody: "two",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("reply opened a new conversation %s, want %s", second.ConversationID, first.ConversationID)
	}
}

func TestHandleForwardedUnknownToken(t *testing.T) {
	h := testutil.New(t)
	seedForwardTenant(t, h, "eml_route3", false, true)

	_, err := h.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
		Recipient: "eml_nobody@inbound.test", From: "cust@x.com", Subject: "x", TextBody: "y",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unknown forwarding address: want ErrNotFound, got %v", err)
	}
}

func TestForwardedSpamFilter(t *testing.T) {
	autoresponders := []map[string]string{
		{"Auto-Submitted": "auto-replied"},
		{"Precedence": "bulk"},
	}
	for _, hdrs := range autoresponders {
		h := testutil.New(t)
		_, addr := seedForwardTenant(t, h, "eml_spam", false, true)
		msg, err := h.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
			Recipient: addr, From: "ooo@x.com", Subject: "Out of office", TextBody: "away", Headers: hdrs,
		})
		if err != nil || msg != nil {
			t.Fatalf("autoresponder %v should be dropped silently, got msg=%v err=%v", hdrs, msg, err)
		}
	}

	// A mailer-daemon sender is also dropped...
	h := testutil.New(t)
	_, addr := seedForwardTenant(t, h, "eml_spam", false, true)
	if msg, err := h.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
		Recipient: addr, From: "MAILER-DAEMON@x.com", Subject: "bounce", TextBody: "failed",
	}); err != nil || msg != nil {
		t.Fatalf("mailer-daemon should be dropped, got msg=%v err=%v", msg, err)
	}

	// ...but a human whose local part merely CONTAINS "noreply" must NOT be
	// dropped (the match is on the exact local part, not a substring).
	if msg, err := h.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
		Recipient: addr, From: "jane.noreply@example.com", Subject: "Help", TextBody: "hi",
	}); err != nil || msg == nil {
		t.Fatalf("a human sender with 'noreply' in the local part must be ingested, got msg=%v err=%v", msg, err)
	}

	// ...but with the filter OFF the same message is ingested.
	h2 := testutil.New(t)
	_, addr2 := seedForwardTenant(t, h2, "eml_nospam", false, false)
	if msg, err := h2.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
		Recipient: addr2, From: "ooo@x.com", Subject: "Out of office", TextBody: "away",
		Headers: map[string]string{"Auto-Submitted": "auto-replied"},
	}); err != nil || msg == nil {
		t.Fatalf("with spam filter off the message must be ingested, got msg=%v err=%v", msg, err)
	}
}

// The SMTP daemon's handler acknowledges intentional drops and permanent
// rejects (returns nil so the MTA doesn't retry), and only surfaces transient
// failures — so a spam/unknown-recipient message is never retried forever, and a
// transient backend failure is never silently acknowledged (mail loss).
func TestForwardedMailHandlerAcks(t *testing.T) {
	h := testutil.New(t)
	_, addr := seedForwardTenant(t, h, "eml_ack", false, true)
	handler := h.Svc.ForwardedMailHandler()
	ctx := context.Background()

	cases := []struct {
		name      string
		recipient string
	}{
		{"successful ingest", addr},
		{"unknown forwarding address", "eml_nobody@inbound.test"},
		{"invalid recipient", "garbage"},
	}
	for _, tc := range cases {
		if err := handler(ctx, mail.InboundEmail{Recipient: tc.recipient, From: "a@x.com", Subject: "hi", TextBody: "x"}); err != nil {
			t.Fatalf("%s should be acknowledged (nil), got %v", tc.name, err)
		}
	}
}

func TestForwardedAutoReply(t *testing.T) {
	// AutoReply on → an acknowledgement is sent to the original sender, carrying
	// the thread token so their reply threads back.
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_ar", true, true)
	ctx := context.Background()
	msg, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: "Hi", TextBody: "hello"})
	if err != nil {
		t.Fatalf("forwarded: %v", err)
	}
	thread, _ := h.Store.Email().Get(ctx, tenantID, msg.ConversationID)
	sent := h.Mailer.Messages()
	if len(sent) != 1 || sent[0].To != "cust@x.com" || !strings.Contains(sent[0].Subject, thread.ThreadToken) {
		t.Fatalf("expected one auto-reply to the sender with the thread token, got %+v", sent)
	}

	// AutoReply off → no acknowledgement.
	h2 := testutil.New(t)
	_, addr2 := seedForwardTenant(t, h2, "eml_noar", false, true)
	if _, err := h2.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr2, From: "cust@x.com", Subject: "Hi", TextBody: "hello"}); err != nil {
		t.Fatalf("forwarded: %v", err)
	}
	if sent := h2.Mailer.Messages(); len(sent) != 0 {
		t.Fatalf("auto-reply off: expected no mail, got %+v", sent)
	}
}
