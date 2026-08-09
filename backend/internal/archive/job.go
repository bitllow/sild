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
// whether this worker did the sweep. The lease is heartbeated for the whole
// sweep, so a pass longer than sweepLeaseTTL keeps its single owner.
func (j *Job) RunSweep(ctx context.Context, limit int) (archived int, ran bool, err error) {
	ran, err = store.RunLeased(ctx, j.store.Leases(), sweepLease, sweepLeaseTTL, func(ctx context.Context) error {
		tenants, terr := j.store.Tenants().AllIDs(ctx)
		if terr != nil {
			return terr
		}
		for _, t := range tenants {
			n, rerr := j.RunOnce(ctx, t, limit)
			archived += n
			if rerr != nil {
				return rerr
			}
		}
		return nil
	})
	return archived, ran, err
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
		// One conversation failing must not stop the batch, but a cancelled context
		// must: under RunSweep that is the lease going to another worker.
		if err := ctx.Err(); err != nil {
			return archived, err
		}
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
	// No profile: it lives in one place and is read live, so a copy here would be
	// a stale second answer. The snapshot preserves identity, for authorization.
	for i := range members {
		ser.Members = append(ser.Members, views.Member(&members[i], views.Profiles{}))
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
