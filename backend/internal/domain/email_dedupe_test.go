package domain_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/mail"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func inboundWithID(messageID string) mail.InboundEmail {
	return mail.InboundEmail{
		Recipient: "help@support.test", From: "cust@x.com",
		Subject: "Need help", TextBody: "hello",
		Headers: map[string]string{"Message-Id": messageID},
	}
}

func openConversations(t *testing.T, h *testutil.Harness, tenantID string) int64 {
	t.Helper()
	n, err := h.Store.Conversations().CountOpen(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// An MTA redelivering after a transient failure must land in the same
// conversation, not open a second one for the same mail.
func TestRedeliveredMailIsNotIngestedTwice(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	if _, err := h.Svc.HandleInbound(ctx, inboundWithID("<abc@sender>")); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	_, err := h.Svc.HandleInbound(ctx, inboundWithID("<abc@sender>"))
	if !errors.Is(err, domain.ErrAlreadyIngested) {
		t.Fatalf("redelivery returned %v, want ErrAlreadyIngested", err)
	}
	if n := openConversations(t, h, tenantID); n != 1 {
		t.Fatalf("%d conversations after one mail delivered twice, want 1", n)
	}
}

// Distinct mail with the same sender and subject is not a redelivery — dedupe
// keys on Message-ID alone and must not swallow a real second message.
func TestDifferentMessageIDsBothIngest(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	if _, err := h.Svc.HandleInbound(ctx, inboundWithID("<one@sender>")); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := h.Svc.HandleInbound(ctx, inboundWithID("<two@sender>")); err != nil {
		t.Fatalf("second: %v", err)
	}
	// Both thread into the one open conversation by sender + subject (§6.2), so
	// what proves the second was ingested is the message, not a new conversation.
	if n := openConversations(t, h, tenantID); n != 1 {
		t.Fatalf("threading broke: %d conversations, want 1", n)
	}
}

// A claim must not outlive a failed ingest, or the MTA's retry would be refused
// as a duplicate and the mail would be lost outright — worse than a duplicate.
func TestReleasedClaimLetsTheRetryThrough(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	first, err := h.Store.Email().ClaimIngest(ctx, tenantID, "<transient@sender>", time.Hour)
	if err != nil || first.Owner == "" {
		t.Fatalf("claim: %+v %v", first, err)
	}
	if err := h.Store.Email().ReleaseIngest(ctx, tenantID, "<transient@sender>", first.Owner); err != nil {
		t.Fatalf("release: %v", err)
	}
	retry, err := h.Store.Email().ClaimIngest(ctx, tenantID, "<transient@sender>", time.Hour)
	if err != nil || retry.Owner == "" {
		t.Fatalf("retry after a released claim was refused: %+v %v", retry, err)
	}
}

// Mail without a Message-ID cannot be deduped, and must still be ingested rather
// than dropped.
func TestMailWithoutMessageIDStillIngests(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	in := inboundWithID("")
	if _, err := h.Svc.HandleInbound(ctx, in); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if n := openConversations(t, h, tenantID); n != 1 {
		t.Fatalf("%d conversations, want 1", n)
	}
}

// The failure this guard must not create: a mail process that dies mid-ingest
// leaves a claim behind. If that claim were permanent, every redelivery would be
// acknowledged as a duplicate and the mail would be lost with no conversation
// anywhere — strictly worse than the duplicate the dedupe prevents.
func TestCrashedIngestLetsTheRedeliveryThrough(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	// A delivery claims the id and then dies: the claim is never completed.
	claimed, err := h.Store.Email().ClaimIngest(ctx, tenantID, "<crash@sender>", time.Hour)
	if err != nil || claimed.Owner == "" {
		t.Fatalf("claim: %+v %v", claimed, err)
	}

	// A concurrent delivery must be told the ingest is still running, NOT that it
	// is done — that attempt may yet fail, and nothing is stored yet.
	again, _ := h.Store.Email().ClaimIngest(ctx, tenantID, "<crash@sender>", time.Hour)
	if again.Owner != "" {
		t.Fatal("a live claim was handed to a second delivery")
	}
	if again.Done || !again.InFlight {
		t.Fatalf("a live claim reported as finished (%+v) — the sender would be told we have mail nobody stored", again)
	}

	// Once it goes stale, the MTA's retry must be able to ingest after all.
	retry, err := h.Store.Email().ClaimIngest(ctx, tenantID, "<crash@sender>", 0)
	if err != nil || retry.Owner == "" {
		t.Fatalf("a stale claim from a dead process permanently swallowed the mail: %+v %v", retry, err)
	}
}

// A completed claim, by contrast, is permanent: age must never let a genuine
// duplicate back in.
func TestCompletedIngestNeverGoesStale(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()

	if _, err := h.Svc.HandleInbound(ctx, inboundWithID("<done@sender>")); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	again, _ := h.Store.Email().ClaimIngest(ctx, tenantID, "<done@sender>", 0)
	if again.Owner != "" {
		t.Fatal("a completed ingest was re-claimed once its claim aged")
	}
	if !again.Done {
		t.Fatalf("a completed ingest did not report Done: %+v", again)
	}
}

// A superseded attempt must not finish or drop the claim that replaced it — the
// takeover would otherwise be undone by the very process it recovered from.
func TestSupersededAttemptCannotTouchTheNewClaim(t *testing.T) {
	h := testutil.New(t)
	tenantID := seedEmailTenant(t, h, "")
	ctx := context.Background()
	const mid = "<superseded@sender>"

	stale, err := h.Store.Email().ClaimIngest(ctx, tenantID, mid, time.Hour)
	if err != nil || stale.Owner == "" {
		t.Fatalf("first claim: %+v %v", stale, err)
	}
	// It hangs past the staleness bound and a second delivery takes over.
	fresh, err := h.Store.Email().ClaimIngest(ctx, tenantID, mid, 0)
	if err != nil || fresh.Owner == "" {
		t.Fatalf("takeover: %+v %v", fresh, err)
	}

	// The abandoned attempt now wakes up and tries to finish and clean up.
	if err := h.Store.Email().CompleteIngest(ctx, tenantID, mid, stale.Owner); err != nil {
		t.Fatalf("stale complete: %v", err)
	}
	if err := h.Store.Email().ReleaseIngest(ctx, tenantID, mid, stale.Owner); err != nil {
		t.Fatalf("stale release: %v", err)
	}

	// Neither may take effect: the new owner still holds an in-flight claim.
	third, err := h.Store.Email().ClaimIngest(ctx, tenantID, mid, time.Hour)
	if err != nil {
		t.Fatalf("third claim: %v", err)
	}
	if third.Owner != "" {
		t.Fatal("the superseded attempt released the claim that replaced it")
	}
	if third.Done {
		t.Fatal("the superseded attempt marked its successor's claim complete")
	}
}
