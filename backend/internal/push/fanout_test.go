package push_test

import (
	"context"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/push"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

type fakeNotifier struct{ notified [][]push.Target }

func (f *fakeNotifier) Notify(_ context.Context, targets []push.Target, _ push.Payload) error {
	f.notified = append(f.notified, targets)
	return nil
}

type onlineSet map[string]bool

func (s onlineSet) Online(_ context.Context, _, userID string) (bool, error) { return s[userID], nil }

// §5.5: fan-out goes only to members with no live connection, and never to the
// sender.
func TestPushFanoutOfflineOnly(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{
			{UserID: "u_sender", ConvRole: models.RoleClient},
			{UserID: "u_online", ConvRole: models.RoleClient},
			{UserID: "u_offline", ConvRole: models.RoleClient},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"u_online", "u_offline"} {
		uid := u
		if err := h.Store.PushTokens().Upsert(ctx, &models.PushToken{
			TenantID: tenant.ID, MemberKind: models.MemberUser, ExternalUserID: &uid,
			Platform: models.PushIOS, Token: "tok_" + u,
		}); err != nil {
			t.Fatal(err)
		}
	}

	notifier := &fakeNotifier{}
	fan := push.NewFanOut(h.Store, notifier, onlineSet{"u_online": true})
	if err := fan.Deliver(ctx, tenant.ID, conv.ID, "u_sender", push.Payload{MessageID: "m1"}); err != nil {
		t.Fatal(err)
	}

	// only u_offline should be notified (sender excluded, online skipped)
	if len(notifier.notified) != 1 {
		t.Fatalf("expected exactly 1 notify (u_offline), got %d", len(notifier.notified))
	}
	if notifier.notified[0][0].Token != "tok_u_offline" {
		t.Fatalf("expected u_offline token, got %s", notifier.notified[0][0].Token)
	}
}
