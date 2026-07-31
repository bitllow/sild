// Command sild-worker runs background jobs (§6.1 webhook relay, §12 archival).
// --jobs, or SILD_JOBS, selects a subset (§3a).
package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/di"
	"github.com/bitllow/sild/backend/internal/jobs"
)

func main() {
	jobsFlag := flag.String("jobs", "", "comma-separated: webhook,archive (default: SILD_JOBS)")
	once := flag.Bool("once", false, "run each job a single time and exit (cron / Cloud Run Jobs)")
	flag.Parse()
	// gcloud splits --args on commas, so `--jobs webhook,archive` arrives as a flag
	// plus a stray "archive" — refuse it rather than silently drop a job.
	if flag.NArg() > 0 {
		log.Fatalf("sild-worker: unexpected argument %q (use -jobs=a,b as one argument)", flag.Arg(0))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	c, err := di.New()
	if err != nil {
		log.Fatalf("di: %v", err)
	}

	err = c.Invoke(func(cfg *config.Config, relay *webhook.Relay, sweep *archive.Job) error {
		list := cfg.Jobs.Enabled
		if *jobsFlag != "" {
			list = *jobsFlag
		}
		selected, err := jobs.Parse(list)
		if err != nil {
			return err
		}
		log.Printf("sild-worker: jobs=%v once=%v (driver=%s)", selected.Names(), *once, cfg.DB.Driver)
		if *once {
			return jobs.RunOnce(ctx, selected, jobs.Deps{Relay: relay, Sweep: sweep})
		}
		jobs.Start(ctx, selected, jobs.Deps{Relay: relay, Sweep: sweep})

		<-ctx.Done()
		log.Printf("sild-worker: shutting down")
		return nil
	})
	if err != nil {
		log.Fatalf("sild-worker: %v", err)
	}
}
