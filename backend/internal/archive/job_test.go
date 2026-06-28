package archive_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/domain"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/testutil"
)

// §12: a closed, idle conversation is written to the sink THEN deleted from hot
// storage (verified), leaving a tombstone with a membership snapshot; the sink
// can rehydrate it.
func TestArchiveWriteThenDelete(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()

	conv, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{{UserID: "u1", ConvRole: models.RoleClient}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ext := "u1"
	if _, err := h.Svc.SendMessage(ctx, tenant.ID, conv.ID, domain.SendInput{
		SenderKind: models.SenderUser, External: &ext, Body: "hello",
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Svc.CloseConversation(ctx, tenant.ID, conv.ID); err != nil {
		t.Fatal(err)
	}

	sink, err := archive.New(h.Cfg)
	if err != nil {
		t.Fatal(err)
	}
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) }) // make it idle

	n, err := job.RunOnce(ctx, tenant.ID, 100)
	if err != nil || n != 1 {
		t.Fatalf("archive: n=%d err=%v", n, err)
	}

	// hot rows are gone
	if _, err := h.Store.Conversations().Get(ctx, tenant.ID, conv.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected hot conversation deleted, got %v", err)
	}
	// tombstone exists with the membership snapshot + message count
	tomb, err := h.Store.Archives().GetTombstone(ctx, tenant.ID, conv.ID)
	if err != nil {
		t.Fatalf("tombstone: %v", err)
	}
	if tomb.MessageCount != 1 || len(tomb.MembersSnapshot) == 0 {
		t.Fatalf("bad tombstone: count=%d snapshot=%s", tomb.MessageCount, tomb.MembersSnapshot)
	}
	// sink rehydrates the conversation
	ser, err := sink.Read(ctx, tomb.SinkRef)
	if err != nil || ser.MessageCount != 1 || len(ser.Messages) != 1 {
		t.Fatalf("sink read: %+v err=%v", ser, err)
	}
}

// §12: an OPEN conversation is never eligible regardless of age.
func TestArchiveSkipsOpen(t *testing.T) {
	h := testutil.New(t)
	tenant := h.SeedTenant()
	ctx := context.Background()
	if _, err := h.Svc.CreateConversation(ctx, tenant.ID, domain.CreateConversationInput{
		Members: []domain.MemberInput{{UserID: "u1", ConvRole: models.RoleClient}},
	}); err != nil {
		t.Fatal(err)
	}
	sink, _ := archive.New(h.Cfg)
	job := archive.NewJob(h.Store, sink, h.Cfg)
	job.SetClock(func() time.Time { return time.Now().Add(60 * 24 * time.Hour) })
	n, err := job.RunOnce(ctx, tenant.ID, 100)
	if err != nil || n != 0 {
		t.Fatalf("open conversation must not archive: n=%d err=%v", n, err)
	}
}
