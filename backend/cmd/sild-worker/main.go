// Command sild-worker runs background jobs (§6.1 webhook relay, §12 archival,
// §6.2 outbound email, §5.5 push fan-out). --jobs selects a subset (§3a).
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/store"
)

func main() {
	jobsFlag := flag.String("jobs", "webhook,archive", "comma-separated: webhook,archive,push,email")
	flag.Parse()
	jobs := map[string]bool{}
	for _, j := range strings.Split(*jobsFlag, ",") {
		jobs[strings.TrimSpace(j)] = true
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	err = c.Invoke(func(cfg *config.Config, st store.Store, relay *webhook.Relay, job *archive.Job) error {
		log.Printf("sild-worker: jobs=%v (driver=%s)", keys(jobs), cfg.DB.Driver)

		if jobs["webhook"] {
			go loop(ctx, 5*time.Second, func() {
				if _, err := relay.ProcessOnce(ctx, 100); err != nil {
					log.Printf("webhook relay: %v", err)
				}
			})
		}
		if jobs["archive"] {
			go loop(ctx, 1*time.Hour, func() {
				tenants, err := st.Tenants().AllIDs(ctx)
				if err != nil {
					log.Printf("archive: list tenants: %v", err)
					return
				}
				for _, t := range tenants {
					if n, err := job.RunOnce(ctx, t, 100); err != nil {
						log.Printf("archive %s: %v", t, err)
					} else if n > 0 {
						log.Printf("archive %s: archived %d conversations", t, n)
					}
				}
			})
		}
		if jobs["push"] || jobs["email"] {
			// Push fan-out and outbound email run inline on the message path
			// today (see domain.SendMessage). Worker-driven delivery keyed on
			// Centrifuge presence is the remaining wiring (§5.5, §3a).
			log.Printf("sild-worker: push/email selected — handled inline on the message path for now")
		}

		<-ctx.Done()
		log.Printf("sild-worker: shutting down")
		return nil
	})
	if err != nil {
		log.Fatalf("sild-worker: %v", err)
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

func keys(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}
