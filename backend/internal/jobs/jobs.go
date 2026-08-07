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
	"github.com/bitllow/sild/backend/internal/push"
)

// Job names accepted in SILD_JOBS / --jobs.
const (
	Webhook = "webhook"
	Archive = "archive"
	Push    = "push"
)

// Deps are the components the jobs run against.
type Deps struct {
	Relay *webhook.Relay
	Sweep *archive.Job
	Push  *push.FanOut
}

// all is the single definition of what a job is called, how often it runs, and
// what it does. Adding a job means adding a row.
var all = []struct {
	name     string
	interval time.Duration
	// run reports how much work the pass did, so RunOnce knows to come back.
	run func(context.Context, Deps) (int, error)
}{
	{Webhook, 5 * time.Second, relayOnce},
	{Archive, 1 * time.Hour, sweepOnce},
	{Push, 5 * time.Second, pushOnce},
}

// batchSize bounds one pass of a job, so a tick stays short and a claim is not
// held over more rows than it can deliver.
const batchSize = 100

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
			return nil, fmt.Errorf("unknown job %q: valid jobs are %s (empty runs none)", name, strings.Join(allNames(), ", "))
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

func allNames() []string {
	out := make([]string, len(all))
	for i, j := range all {
		out[i] = j.name
	}
	return out
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
		run := j.run
		go loop(ctx, j.interval, func() {
			if _, err := run(ctx, deps); err != nil {
				log.Printf("%v", err)
			}
		})
	}
}

// RunOnce drains each selected job and returns. It is what a scheduled runner
// needs — a Cloud Run Job, a k8s CronJob, `sild-worker --once` — where a process
// that never exits is a task that never succeeds. It drains rather than running a
// single pass because one pass is sized for a 5-second tick: on a cron schedule
// that caps throughput at a batch per run and the backlog grows unnoticed.
func RunOnce(ctx context.Context, s Set, deps Deps) error {
	var errs []error
	for _, j := range all {
		if s[j.name] {
			errs = append(errs, drain(ctx, j.run, deps))
		}
	}
	return errors.Join(errs...)
}

// drain repeats a job until a pass does no work, ctx ends, or it errors. Both
// jobs leave what they could not do rescheduled or unclaimed, so "no work" is a
// terminating condition rather than "nothing is left".
func drain(ctx context.Context, run func(context.Context, Deps) (int, error), deps Deps) error {
	for {
		n, err := run(ctx, deps)
		if err != nil {
			return err
		}
		if n == 0 || ctx.Err() != nil {
			return nil
		}
	}
}

func relayOnce(ctx context.Context, deps Deps) (int, error) {
	n, err := deps.Relay.ProcessOnce(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("webhook relay: %w", err)
	}
	return n, nil
}

func pushOnce(ctx context.Context, deps Deps) (int, error) {
	n, err := deps.Push.ProcessOnce(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("push fan-out: %w", err)
	}
	return n, nil
}

func sweepOnce(ctx context.Context, deps Deps) (int, error) {
	n, ran, err := deps.Sweep.RunSweep(ctx, batchSize)
	switch {
	case err != nil:
		return 0, fmt.Errorf("archive sweep: %w", err)
	case !ran:
		log.Printf("archive sweep: another process holds the lease — skipping")
	case n > 0:
		log.Printf("archive sweep: archived %d conversations", n)
	}
	return n, nil
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
