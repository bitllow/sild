package domain_test

import (
	"context"
	"sync"
	"testing"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// A client retrying a send while the first attempt is still in flight must get
// the original message back, not a 500 from the unique index that exists
// precisely to stop the duplicate.
func TestConcurrentSendsWithOneClientMsgID(t *testing.T) {
	h := testutil.New(t)
	ctx := context.Background()
	tenant := h.SeedTenant()
	user := "u_client"
	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{{UserID: user, ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	key := "retry-me"
	const attempts = 6

	var wg sync.WaitGroup
	ids := make([]string, attempts)
	errs := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg, err := h.Svc.SendMessage(ctx, tenant.ID, conv.ID, domain.SendInput{
				SenderKind: models.SenderUser, External: &user, Channel: models.ChannelApp,
				Body: "same message", ClientMsgID: &key,
			})
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = msg.ID
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("attempt %d failed instead of returning the original: %v", i, err)
		}
	}
	for i, got := range ids {
		if got != ids[0] {
			t.Fatalf("attempt %d returned message %s, attempt 0 returned %s — the retry made a second message", i, got, ids[0])
		}
	}

	page, err := h.Svc.ListMessagesBefore(ctx, tenant.ID, conv.ID, "", 50, false)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("conversation holds %d messages, want 1", len(page.Messages))
	}
}
