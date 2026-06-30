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
	"github.com/bitllow/sild/backend/internal/realtime"
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

// §6.2: inbound creates a conversation; an agent reply goes out with a clean
// "Re: <subject>"; the customer's reply threads back by sender + subject (no
// special token).
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

	// Agent replies (participants) → outbound email with a clean "Re:" subject.
	agentID := "agent_1"
	if _, err := h.Svc.SendMessage(ctx, tenantID, convID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &agentID, Body: "How can I help?",
		Visibility: models.VisibilityParticipants, AllowInternal: true,
	}); err != nil {
		t.Fatalf("agent reply: %v", err)
	}
	sent := h.Mailer.Messages()
	if len(sent) != 1 || sent[0].To != "cust@x.com" || sent[0].Subject != "Re: Need help" {
		t.Fatalf("expected one clean-subject reply to the customer, got %+v", sent)
	}

	// Customer replies with the same subject (modulo Re:) → threads by sender +
	// subject into the same conversation.
	msg2, err := h.Svc.HandleInbound(ctx, mail.InboundEmail{
		Recipient: "help@support.test", From: "cust@x.com",
		Subject: "Re: Need help", TextBody: "thanks",
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

// A new email conversation must nudge the agents channel (tenant-targeted) so
// the inbox surfaces it live; without it the conversation only appears on
// refresh.
func TestForwardedCreateNudgesAgentsChannel(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_rt", false, true)
	if _, err := h.Svc.HandleForwarded(context.Background(), mail.InboundEmail{
		Recipient: addr, From: "cust@x.com", Subject: "Hi", TextBody: "hello",
	}); err != nil {
		t.Fatalf("forwarded: %v", err)
	}
	found := false
	for _, e := range h.Pub.OfType(realtime.EventAssignmentUpdated) {
		if e.Target.Tenant == tenantID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a tenant-targeted %s event for the new email conversation", realtime.EventAssignmentUpdated)
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
	_ = tenantID

	// A reply from the same sender with the same subject (modulo "Re:"), sent to a
	// +subaddressed form of the forwarding address (tenant resolution strips the
	// subaddress), threads into the same conversation — by sender + subject.
	plusAddr := "eml_route2+inbox@inbound.test"
	second, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{
		Recipient: plusAddr, From: "cust@x.com",
		Subject: "Re: Hi", TextBody: "two",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("reply opened a new conversation %s, want %s", second.ConversationID, first.ConversationID)
	}
}

// Threading is by sender + normalized subject among OPEN conversations; the
// original subject is stored for display.
func TestForwardedThreadsBySenderSubject(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_ss", false, true)
	ctx := context.Background()
	send := func(from, subject string) string {
		m, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: from, Subject: subject, TextBody: "x"})
		if err != nil || m == nil {
			t.Fatalf("send %q/%q: msg=%v err=%v", from, subject, m, err)
		}
		return m.ConversationID
	}

	a := send("cust@x.com", "Order 9001")
	if got := send("cust@x.com", "RE: Order 9001"); got != a { // same sender + subject (modulo Re:)
		t.Fatalf("same sender+subject should thread into %s, got %s", a, got)
	}
	if got := send("cust@x.com", "Order 9002"); got == a { // different subject
		t.Fatalf("a different subject should open a new conversation")
	}
	if got := send("other@x.com", "Order 9001"); got == a { // different sender
		t.Fatalf("a different sender should open a new conversation")
	}
	if subj := h.Svc.EmailSubject(ctx, tenantID, a); subj != "Order 9001" {
		t.Fatalf("EmailSubject = %q, want %q", subj, "Order 9001")
	}

	// Closed is terminal (§1): a later email with the same sender+subject starts a
	// new conversation rather than reopening the closed one.
	if err := h.Svc.CloseConversation(ctx, tenantID, a); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := send("cust@x.com", "Order 9001"); got == a {
		t.Fatalf("a closed conversation must not be reused; expected a new conversation")
	}
}

// The thread token (carried back in the Reply-To +subaddress) takes precedence
// over sender+subject — a reply threads even if the subject changed.
func TestForwardedThreadsByToken(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_tok", false, true)
	ctx := context.Background()
	first, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: "Order 9001", TextBody: "one"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	thread, err := h.Store.Email().Get(ctx, tenantID, first.ConversationID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}

	second, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{
		Recipient: addr, From: "someone-else@x.com", // different sender
		Subject: "a completely different subject", TextBody: "two", // different subject
		Headers: map[string]string{"To": "support+sild#" + thread.ThreadToken + "@inbound.test"},
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ConversationID != first.ConversationID {
		t.Fatalf("token reply should thread into %s, got %s", first.ConversationID, second.ConversationID)
	}
}

// A long Subject header must still ingest: the full subject is stored for
// display, and the threading key is truncated to the column width (so the insert
// can't fail on a width-enforcing backend) while replies still thread.
func TestForwardedLongSubject(t *testing.T) {
	h := testutil.New(t)
	tenantID, addr := seedForwardTenant(t, h, "eml_long", false, true)
	ctx := context.Background()
	long := strings.Repeat("supercalifragilistic ", 60) // > 512 chars

	m1, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: long, TextBody: "one"})
	if err != nil || m1 == nil {
		t.Fatalf("a long-subject email must ingest, got msg=%v err=%v", m1, err)
	}
	th, err := h.Store.Email().Get(ctx, tenantID, m1.ConversationID)
	if err != nil {
		t.Fatalf("thread: %v", err)
	}
	if n := len([]rune(th.SubjectKey)); n > 255 {
		t.Fatalf("subject key not truncated: %d runes", n)
	}
	if th.Subject != long {
		t.Fatalf("the full subject should be stored for display")
	}

	// A reply with the same (long) subject threads into the same conversation.
	m2, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: "Re: " + long, TextBody: "two"})
	if err != nil || m2 == nil || m2.ConversationID != m1.ConversationID {
		t.Fatalf("same long subject should thread; got msg=%v err=%v", m2, err)
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
	if _, err := h.Svc.HandleForwarded(ctx, mail.InboundEmail{Recipient: addr, From: "cust@x.com", Subject: "Hi", TextBody: "hello"}); err != nil {
		t.Fatalf("forwarded: %v", err)
	}
	_ = tenantID
	sent := h.Mailer.Messages()
	if len(sent) != 1 || sent[0].To != "cust@x.com" || sent[0].Subject != "Re: Hi" {
		t.Fatalf("expected one auto-reply to the sender with a clean \"Re:\" subject, got %+v", sent)
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
