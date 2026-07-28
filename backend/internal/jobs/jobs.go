// Package jobs runs the background work of the backend: the webhook outbox relay
// (§6.1) and the conversation archival sweep (§12). sild-dev, sild-worker and
// sild-standalone all drive it, so the schedule is the same wherever the jobs run.
//
// Every job here is safe on N processes at once: the relay claims outbox rows and
// the sweep takes a cluster-wide lease (ARCHITECTURE §4).
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

// Deps are the components the jobs run against.
type Deps struct {
	Relay *webhook.Relay
	Sweep *archive.Job
}

// all is the single definition of what a job is called, how often it runs, and
// what it does. Adding a job means adding a row.
var all = []struct {
	name     string
	interval time.Duration
	run      func(context.Context, Deps) error
}{
	{Webhook, 5 * time.Second, relayOnce},
	{Archive, 1 * time.Hour, sweepOnce},
}

const (
	relayBatch = 100
	sweepBatch = 100
)

// Set is the selected job names.
type Set map[string]bool

// Parse reads a comma-separated job list. An unknown name is an error, not a
// warning: it would otherwise leave a job off a process that reports itself
// healthy, and nothing would surface it.
func Parse(list string) (Set, error) {
	s := Set{}
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !known(name) {
			return nil, fmt.Errorf("unknown job %q: valid jobs are %s, %s (empty runs none)", name, Webhook, Archive)
		}
		s[name] = true
	}
	return s, nil
}

func known(name string) bool {
	for _, j := range all {
		if j.name == name {
			return true
		}
	}
	return false
}

// Names lists the enabled jobs in a stable order for logging.
func (s Set) Names() []string {
	var out []string
	for _, j := range all {
		if s[j.name] {
			out = append(out, j.name)
		}
	}
	return out
}

// Start launches the selected jobs in background goroutines and returns
// immediately. They stop when ctx is done.
func Start(ctx context.Context, s Set, deps Deps) {
	for _, j := range all {
		if !s[j.name] {
			continue
		}
		run, interval := j.run, j.interval
		go loop(ctx, interval, func() {
			if err := run(ctx, deps); err != nil {
				log.Printf("%v", err)
			}
		})
	}
}

// RunOnce runs each selected job exactly once and returns. It is what a scheduled
// runner needs — a Cloud Run Job, a k8s CronJob, `sild-worker --once` — where a
// process that never exits is a task that never succeeds.
func RunOnce(ctx context.Context, s Set, deps Deps) error {
	var errs []error
	for _, j := range all {
		if s[j.name] {
			errs = append(errs, j.run(ctx, deps))
		}
	}
	return errors.Join(errs...)
}

func relayOnce(ctx context.Context, deps Deps) error {
	if _, err := deps.Relay.ProcessOnce(ctx, relayBatch); err != nil {
		return fmt.Errorf("webhook relay: %w", err)
	}
	return nil
}

func sweepOnce(ctx context.Context, deps Deps) error {
	n, ran, err := deps.Sweep.RunSweep(ctx, sweepBatch)
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
