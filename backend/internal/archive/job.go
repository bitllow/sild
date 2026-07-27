package archive

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/id"
	"github.com/bitllow/sild/backend/internal/store"
	"github.com/bitllow/sild/backend/internal/store/models"
	"github.com/bitllow/sild/backend/internal/views"
	"gorm.io/datatypes"
)

// Job archives eligible conversations (§12). Write-then-delete, verified.
type Job struct {
	store    store.Store
	sink     Sink
	idleDays int
	now      func() time.Time
}

// NewJob constructs the archival job. dig provides it for sild-worker.
func NewJob(st store.Store, sink Sink, cfg *config.Config) *Job {
	return &Job{store: st, sink: sink, idleDays: cfg.Archive.IdleDays, now: time.Now}
}

// SetClock overrides the clock (tests).
func (j *Job) SetClock(fn func() time.Time) { j.now = fn }

// sweepLease names the cluster-wide lease for a full sweep, and bounds how long
// one worker may hold it. Without it, every replica sweeps every tenant on its
// own ticker and the passes overlap.
const (
	sweepLease    = "archive-sweep"
	sweepLeaseTTL = 30 * time.Minute
)

// RunSweep archives every tenant, once per cluster: a worker that cannot take
// the lease skips this tick rather than duplicating the pass. ran reports
// whether this worker did the sweep.
func (j *Job) RunSweep(ctx context.Context, limit int) (archived int, ran bool, err error) {
	owner := id.New(id.Holder)
	ok, err := j.store.Leases().Acquire(ctx, sweepLease, owner, sweepLeaseTTL)
	if err != nil || !ok {
		return 0, false, err
	}
	defer func() { _ = j.store.Leases().Release(context.WithoutCancel(ctx), sweepLease, owner) }()

	tenants, err := j.store.Tenants().AllIDs(ctx)
	if err != nil {
		return 0, true, err
	}
	for i, t := range tenants {
		// A fleet-wide sweep can outlast the lease, so renew as it goes and stop if
		// we have lost it — carrying on would be the overlapping sweep the lease
		// exists to prevent. The first tenant is covered by the lease just taken.
		if i > 0 {
			held, aerr := j.store.Leases().Acquire(ctx, sweepLease, owner, sweepLeaseTTL)
			if aerr != nil {
				return archived, true, aerr
			}
			if !held {
				return archived, true, nil
			}
		}
		n, rerr := j.RunOnce(ctx, t, limit)
		if rerr != nil {
			return archived, true, rerr
		}
		archived += n
	}
	return archived, true, nil
}

// RunOnce archives up to `limit` eligible conversations for a tenant and returns
// the count archived. Eligibility: status=closed AND idle past idleDays (§12).
func (j *Job) RunOnce(ctx context.Context, tenantID string, limit int) (int, error) {
	cutoff := id.MinForTime(id.Message, j.now().AddDate(0, 0, -j.idleDays))
	convs, err := j.store.Conversations().ListArchivable(ctx, tenantID, cutoff, limit)
	if err != nil {
		return 0, err
	}
	archived := 0
	for i := range convs {
		if err := j.archiveOne(ctx, &convs[i]); err == nil {
			archived++
		}
	}
	return archived, nil
}

func (j *Job) archiveOne(ctx context.Context, conv *models.Conversation) error {
	members, err := j.store.Members().ListActive(ctx, conv.TenantID, conv.ID)
	if err != nil {
		return err
	}
	msgs, err := j.allMessages(ctx, conv.TenantID, conv.ID)
	if err != nil {
		return err
	}

	ser := SerializedConversation{
		ConversationID: conv.ID, TenantID: conv.TenantID, Reference: conv.Reference,
		Status: string(conv.Status), Kind: string(conv.Kind), MessageCount: len(msgs),
	}
	if len(conv.Metadata) > 0 {
		ser.Metadata = json.RawMessage(conv.Metadata)
	}
	for i := range members {
		ser.Members = append(ser.Members, views.Member(&members[i]))
	}
	for i := range msgs {
		ser.Messages = append(ser.Messages, views.Message(&msgs[i], nil))
	}

	// 1) Write to the sink and VERIFY before deleting anything (§12).
	sinkRef, err := j.sink.Write(ctx, ser)
	if err != nil {
		return err
	}

	// membership snapshot preserves archived-read authorization (review finding).
	snapshot, _ := json.Marshal(ser.Members)

	// 2) In one transaction: tombstone, then purge hot rows.
	return j.store.Tx(ctx, func(tx store.Store) error {
		if err := tx.Archives().CreateTombstone(ctx, &models.ConversationArchive{
			ConversationID: conv.ID, TenantID: conv.TenantID, Sink: models.ArchiveSink(j.sink.Name()),
			SinkRef: sinkRef, MessageCount: len(msgs), MembersSnapshot: datatypes.JSON(snapshot),
			Kind: conv.Kind, ArchivedAt: j.now(),
		}); err != nil {
			return err
		}
		return tx.Archives().PurgeHot(ctx, conv.TenantID, conv.ID)
	})
}

// archiveBatchSize is the page size used to drain a conversation for the sink.
const archiveBatchSize = 500

// allMessages pages through every message (incl. internal) for serialization.
func (j *Job) allMessages(ctx context.Context, tenantID, convID string) ([]models.Message, error) {
	var all []models.Message
	after := ""
	for {
		batch, err := j.store.Messages().ListAfter(ctx, tenantID, convID, after, archiveBatchSize, true)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		after = batch[len(batch)-1].ID
		if len(batch) < 500 {
			break
		}
	}
	return all, nil
}
