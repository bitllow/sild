package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// pushFixture is a configured tenant with a peer conversation between two users,
// each holding one device.
type pushFixture struct {
	h      *testutil.Harness
	tenant *models.Tenant
	conv   *models.Conversation
}

func newPushFixture(t *testing.T) *pushFixture {
	t.Helper()
	h := testutil.New(t)
	tenant := h.SeedTenant()
	h.SeedPushCredential(tenant.ID, "acme-app")

	conv, err := h.Svc.CreateConversation(context.Background(), tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{
			{UserID: "u_alice", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Alice"}`)},
			{UserID: "u_bob", ConvRole: models.RoleClient, Metadata: json.RawMessage(`{"name":"Bob"}`)},
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	h.SeedPushToken(tenant.ID, "u_alice", "device-alice", models.PushAndroid)
	h.SeedPushToken(tenant.ID, "u_bob", "device-bob", models.PushIOS)
	return &pushFixture{h: h, tenant: tenant, conv: conv}
}

// send posts a message as a user and drains the queue, which is what a
// deployment does — the nudge is queued with the message and delivered by a job.
func (f *pushFixture) send(t *testing.T, from, body string) {
	t.Helper()
	_, err := f.h.Svc.SendMessage(context.Background(), f.tenant.ID, f.conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &from, Body: body,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	f.h.RunPushJob()
}

// §5.5: everyone in the conversation is nudged except whoever sent it.
func TestPushReachesTheOtherMemberNotTheSender(t *testing.T) {
	f := newPushFixture(t)
	f.send(t, "u_alice", "running late")

	sent := f.h.Notifier.Nudges()
	if len(sent) != 1 {
		t.Fatalf("expected 1 nudge, got %d: %+v", len(sent), sent)
	}
	if sent[0].Target.Token != "device-bob" {
		t.Fatalf("nudged %q, want bob's device", sent[0].Target.Token)
	}
	if sent[0].ProjectID != "acme-app" {
		t.Fatalf("sent through project %q, not the tenant's own", sent[0].ProjectID)
	}
	if sent[0].Nudge.ConversationID != f.conv.ID || sent[0].Nudge.Kind != string(models.KindPeer) {
		t.Fatalf("nudge does not identify the conversation for a tap: %+v", sent[0].Nudge)
	}
}

// §5.6: an internal note never leaves the agent channels.
func TestPushSkipsInternalNotes(t *testing.T) {
	f := newPushFixture(t)
	actor := "adm_1"
	_, err := f.h.Svc.SendMessage(context.Background(), f.tenant.ID, f.conv.ID, domain.SendInput{
		SenderKind: models.SenderAgent, Internal: &actor, Body: "internal only",
		Visibility: models.VisibilityInternal, AllowInternal: true,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	f.h.RunPushJob()

	if got := f.h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("an internal note produced %d nudges: %+v", len(got), got)
	}
}

// A tenant that has not configured push must not have messages stall in the
// queue, and must not notify anyone.
func TestNoCredentialMeansNoNudge(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	conv, err := h.Svc.CreateConversation(context.Background(), tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{
			{UserID: "u_alice", ConvRole: models.RoleClient},
			{UserID: "u_bob", ConvRole: models.RoleClient},
		},
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	h.SeedPushToken(tenant.ID, "u_bob", "device-bob", models.PushAndroid)

	from := "u_alice"
	if _, err := h.Svc.SendMessage(context.Background(), tenant.ID, conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &from, Body: "hi",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	h.RunPushJob()

	if got := h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("unconfigured tenant sent %d nudges", len(got))
	}
}

// The queue row commits with the message, so a nudge cannot be lost to a crash
// between the two.
func TestNudgeIsQueuedWithTheMessage(t *testing.T) {
	f := newPushFixture(t)
	from := "u_alice"
	msg, err := f.h.Svc.SendMessage(context.Background(), f.tenant.ID, f.conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &from, Body: "hi",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	var queued models.PushOutbox
	if err := f.h.DB.Where("message_id = ?", msg.ID).First(&queued).Error; err != nil {
		t.Fatalf("no nudge queued for the committed message: %v", err)
	}
	if queued.TenantID != f.tenant.ID || queued.ConversationID != f.conv.ID {
		t.Fatalf("queued row does not match the message: %+v", queued)
	}
}

// The webhook relay claims pending rows by status and availability alone, with
// no predicate on what kind of work they are. A nudge sharing that table would
// be claimed by a relay from the running release and POSTed to tenant webhook
// endpoints — which is why the queue is a separate table.
func TestTheWebhookRelayCannotClaimANudge(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	from := "u_alice"
	msg, err := f.h.Svc.SendMessage(ctx, f.tenant.ID, f.conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &from, Body: "hi",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	// Drain the webhook outbox the way the relay does, taking everything due.
	claimed, _, err := f.h.Store.Outbox().ClaimDue(ctx, 100)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	for _, e := range claimed {
		if e.ConversationID == f.conv.ID && e.EventType != "message.created" && e.EventType != "conversation.created" {
			t.Fatalf("relay claimed something that is not a webhook event: %s", e.EventType)
		}
	}

	// The nudge is untouched by that drain and still deliverable.
	f.h.RunPushJob()
	if got := f.h.Notifier.Nudges(); len(got) != 1 {
		t.Fatalf("nudge for %s was not delivered after a webhook drain: %d nudges", msg.ID, len(got))
	}
}

// Composition is server-side, so the settings are what a backgrounded device
// displays — it runs no app code.
func TestNudgeTextFollowsTenantSettings(t *testing.T) {
	cases := []struct {
		name         string
		sender, body bool
		wantTitle    string
		wantBody     string
	}{
		{"nothing revealed", false, false, "New message", ""},
		{"sender only", true, false, "Alice", "New message"},
		{"sender and body", true, true, "Alice", "running late"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPushFixture(t)
			err := f.h.Svc.SetPushSettings(context.Background(), f.tenant.ID, domain.PushSettings{
				IncludeSender: tc.sender, IncludeBody: tc.body, SenderSource: models.PushSenderBrand,
			})
			if err != nil {
				t.Fatalf("settings: %v", err)
			}
			f.send(t, "u_alice", "running late")

			sent := f.h.Notifier.Nudges()
			if len(sent) != 1 {
				t.Fatalf("expected 1 nudge, got %d", len(sent))
			}
			if sent[0].Nudge.Title != tc.wantTitle || sent[0].Nudge.Body != tc.wantBody {
				t.Fatalf("nudge = (%q, %q), want (%q, %q)",
					sent[0].Nudge.Title, sent[0].Nudge.Body, tc.wantTitle, tc.wantBody)
			}
		})
	}
}

// A dead device is the only thing that shrinks the token table besides an
// explicit deregistration, so the pruning is load-bearing rather than tidiness.
func TestDeadTokensArePruned(t *testing.T) {
	f := newPushFixture(t)
	f.h.Notifier.Fail = push.ErrTokenDead
	f.send(t, "u_alice", "hi")

	tokens, err := f.h.Store.PushTokens().ListForUser(context.Background(), f.tenant.ID, "u_bob")
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("dead token survived: %+v", tokens)
	}
}

// A misconfigured project answers with the same HTTP statuses as a dead device.
// Pruning on those would empty the tenant's device table over a few minutes and
// leave nothing to notify once the configuration was fixed.
func TestACredentialFailureDoesNotPruneTokens(t *testing.T) {
	f := newPushFixture(t)
	f.h.Notifier.Fail = push.ErrCredential
	f.send(t, "u_alice", "hi")

	tokens, err := f.h.Store.PushTokens().ListForUser(context.Background(), f.tenant.ID, "u_bob")
	if err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("a credential failure deleted %d of the user's devices", 1-len(tokens))
	}
}

// A database that blinks must not cost the notification: failing the row drops
// it for good, where retrying costs one more pass.
func TestATransientFailureIsRetriedNotDropped(t *testing.T) {
	f := newPushFixture(t)
	f.h.Notifier.Fail = push.ErrRetryable
	f.send(t, "u_alice", "hi")

	var queued models.PushOutbox
	if err := f.h.DB.Where("conversation_id = ?", f.conv.ID).First(&queued).Error; err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if queued.Status != models.DeliveryPending {
		t.Fatalf("a transient failure left the nudge %s, not pending", queued.Status)
	}

	// The next pass, once the provider is healthy again, delivers it.
	f.h.Notifier.Reset()
	f.h.DB.Model(&models.PushOutbox{}).Where("id = ?", queued.ID).Update("available_at", time.Now().Add(-time.Minute))
	f.h.RunPushJob()
	if got := f.h.Notifier.Nudges(); len(got) != 1 {
		t.Fatalf("the retry delivered %d nudges, want 1", len(got))
	}
}

// Losing the claim mid-send must not settle the row: the worker that now holds
// it is still working, and marking it delivered would drop the rest of the
// devices for good.
func TestLosingTheClaimLeavesTheRowAlone(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	from := "u_alice"
	if _, err := f.h.Svc.SendMessage(ctx, f.tenant.ID, f.conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &from, Body: "hi",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	// Another worker takes the row out from under this one.
	if err := f.h.DB.Model(&models.PushOutbox{}).Where("conversation_id = ?", f.conv.ID).
		Updates(map[string]any{"claim_token": "someone-else", "locked_until": time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatalf("steal claim: %v", err)
	}
	f.h.RunPushJob()

	var queued models.PushOutbox
	if err := f.h.DB.Where("conversation_id = ?", f.conv.ID).First(&queued).Error; err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if queued.Status == models.DeliveryDelivered {
		t.Fatal("a row held by another worker was marked delivered")
	}
}

// A user the tenant's backend has opted out gets nothing, and the opt-out
// survives the app registering again — that is what makes it a preference
// rather than a token deletion.
func TestOptOutSuppressesAndSurvivesReregistration(t *testing.T) {
	f := newPushFixture(t)
	ctx := context.Background()
	if err := f.h.Svc.SetUserPush(ctx, f.tenant.ID, "u_bob", false); err != nil {
		t.Fatalf("opt out: %v", err)
	}
	f.send(t, "u_alice", "hi")
	if got := f.h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("opted-out user got %d nudges", len(got))
	}

	// The app comes back and registers, as it does on every launch.
	f.h.SeedPushToken(f.tenant.ID, "u_bob", "device-bob", models.PushAndroid)
	f.send(t, "u_alice", "again")
	if got := f.h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("re-registering undid the opt-out: %d nudges", len(got))
	}

	// Lifting it resumes delivery without a reinstall.
	if err := f.h.Svc.SetUserPush(ctx, f.tenant.ID, "u_bob", true); err != nil {
		t.Fatalf("clear opt out: %v", err)
	}
	f.send(t, "u_alice", "third")
	if got := f.h.Notifier.Nudges(); len(got) == 0 {
		t.Fatal("lifting the opt-out did not resume delivery")
	}
}

// The host's account-deletion call, over its server credential.
func TestHostCanDeleteAUsersDevices(t *testing.T) {
	f := newPushFixture(t)
	key := f.h.SeedAPIKey(f.tenant.ID)

	w := f.h.Request("DELETE", "/v1/users/u_bob/push-tokens").Bearer(key).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("delete tokens = %d: %s", w.Code, w.Body)
	}
	var body struct {
		Deleted int `json:"deleted"`
	}
	testutil.DecodeJSON(t, w, &body)
	if body.Deleted != 1 {
		t.Fatalf("deleted %d devices, want 1", body.Deleted)
	}
	f.send(t, "u_alice", "hi")
	if got := f.h.Notifier.Nudges(); len(got) != 0 {
		t.Fatalf("a user with no devices got %d nudges", len(got))
	}
}

// The control surface belongs to the host's backend, not to the user whose
// notifications it governs.
func TestPushControlRejectsAUserToken(t *testing.T) {
	f := newPushFixture(t)
	tok := f.h.MintToken(f.tenant.ID, "u_bob")

	for _, tc := range []struct{ method, path string }{
		{"PUT", "/v1/users/u_bob/push"},
		{"DELETE", "/v1/users/u_bob/push-tokens"},
	} {
		w := f.h.Request(tc.method, tc.path).Bearer(tok).JSON(map[string]any{"enabled": false}).Do()
		if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s with a user token = %d, want it refused", tc.method, tc.path, w.Code)
		}
	}
}

// The credential is write-only: a read gives the project identity so a tenant
// can confirm the wiring, and never the secret.
func TestPushConfigNeverReturnsTheCredential(t *testing.T) {
	f := newPushFixture(t)
	f.h.SeedAdmin(f.tenant.ID, "owner@acme.test", models.PlatformOwner)
	sess := loginAs(t, f.h, "owner@acme.test")

	w := f.h.Request("GET", "/v1/channels/push").Cookie("sild_admin", sess).Do()
	if w.Code != http.StatusOK {
		t.Fatalf("get push config = %d: %s", w.Code, w.Body)
	}
	body := w.Body.String()
	if !strings.Contains(body, "acme-app") {
		t.Fatalf("config does not name the project: %s", body)
	}
	for _, leak := range []string{"private_key", "credential_sealed", "CredentialSealed", "BEGIN PRIVATE KEY"} {
		if strings.Contains(body, leak) {
			t.Fatalf("push config leaked %q: %s", leak, body)
		}
	}
}

// Uploading a credential is more than a settings write — it confers the ability
// to notify every user of the tenant — so an agent must not hold it.
func TestPushCredentialIsOwnerOnly(t *testing.T) {
	f := newPushFixture(t)
	f.h.SeedAdmin(f.tenant.ID, "agent@acme.test", models.PlatformAgent)
	sess := loginAs(t, f.h, "agent@acme.test")

	w := f.h.Request("PUT", "/v1/channels/push/credential").Cookie("sild_admin", sess).
		JSON(map[string]any{"credential": json.RawMessage(`{"type":"service_account"}`)}).Do()
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent setting a push credential = %d, want 403", w.Code)
	}
}
