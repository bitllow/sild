// Package jobs runs the background work of the backend: the webhook outbox
// relay (§6.1) and the conversation archival sweep (§12). sild-worker and
// sild-standalone both drive it, so the schedule and the log lines are the same
// wherever the jobs happen to run.
//
// Every job here is safe on N processes at once: the relay claims outbox rows
// and the sweep takes a cluster-wide lease (ARCHITECTURE §4).
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
)

// Job names accepted in SILD_JOBS / --jobs.
const (
	Webhook = "webhook"
	Archive = "archive"
)

const (
	relayInterval = 5 * time.Second
	sweepInterval = 1 * time.Hour
	relayBatch    = 100
	sweepBatch    = 100
)

// Set is the selected job names.
type Set map[string]bool

// Parse reads a comma-separated job list. An unknown name is an error rather than
// a warning: SILD_JOBS=webhok,archive would otherwise leave webhook delivery off
// on a process that reports itself healthy, and nothing would surface it until
// someone noticed events had stopped arriving.
func Parse(list string) (Set, error) {
	s := Set{}
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		switch name {
		case "":
		case Webhook, Archive:
			s[name] = true
		default:
			return nil, fmt.Errorf("unknown job %q: valid jobs are %s, %s (empty runs none)", name, Webhook, Archive)
		}
	}
	return s, nil
}

// Names lists the enabled jobs in a stable order for logging.
func (s Set) Names() []string {
	var out []string
	for _, name := range []string{Webhook, Archive} {
		if s[name] {
			out = append(out, name)
		}
	}
	return out
}

// Start launches the selected jobs in background goroutines and returns
// immediately. They stop when ctx is done.
func Start(ctx context.Context, s Set, relay *webhook.Relay, sweep *archive.Job) {
	if s[Webhook] {
		go loop(ctx, relayInterval, func() { logErr(relayOnce(ctx, relay)) })
	}
	if s[Archive] {
		go loop(ctx, sweepInterval, func() { logErr(sweepOnce(ctx, sweep)) })
	}
}

// RunOnce runs each selected job exactly once and returns. It is what a scheduled
// runner needs — a Cloud Run Job, a k8s CronJob, `sild-worker --once` — where a
// process that never exits is a task that never succeeds.
func RunOnce(ctx context.Context, s Set, relay *webhook.Relay, sweep *archive.Job) error {
	var errs []error
	if s[Webhook] {
		errs = append(errs, relayOnce(ctx, relay))
	}
	if s[Archive] {
		errs = append(errs, sweepOnce(ctx, sweep))
	}
	return errors.Join(errs...)
}

func relayOnce(ctx context.Context, relay *webhook.Relay) error {
	if _, err := relay.ProcessOnce(ctx, relayBatch); err != nil {
		return fmt.Errorf("webhook relay: %w", err)
	}
	return nil
}

func sweepOnce(ctx context.Context, sweep *archive.Job) error {
	n, ran, err := sweep.RunSweep(ctx, sweepBatch)
	switch {
	case err != nil:
		return fmt.Errorf("archive sweep: %w", err)
	case !ran:
		log.Printf("archive sweep: another process holds the lease — skipping")
	case n > 0:
		log.Printf("archive sweep: archived %d conversations", n)
	}
	return nil
}

func logErr(err error) {
	if err != nil {
		log.Printf("%v", err)
	}
}

// loop runs fn immediately, then every interval until ctx is done.
func loop(ctx context.Context, interval time.Duration, fn func()) {
	fn()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}
