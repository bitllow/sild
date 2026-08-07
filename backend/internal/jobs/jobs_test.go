package jobs_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/bitllow/sild/backend/internal/archive"
	"github.com/bitllow/sild/backend/internal/config"
	"github.com/bitllow/sild/backend/internal/connector/webhook"
	"github.com/bitllow/sild/backend/internal/jobs"
	"github.com/bitllow/sild/backend/internal/testutil"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name, list string
		want       []string
	}{
		{"default", "webhook,archive", []string{"webhook", "archive"}},
		{"order is normalized for logging", "archive,webhook", []string{"webhook", "archive"}},
		{"spaces", " webhook , archive ", []string{"webhook", "archive"}},
		{"one job", "webhook", []string{"webhook"}},
		// A pure serving replica: the documented way to keep background work off
		// the processes handling requests.
		{"empty means none", "", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set, err := jobs.Parse(tc.list)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.list, err)
			}
			if got := set.Names(); !slices.Equal(got, tc.want) {
				t.Fatalf("Parse(%q).Names() = %v, want %v", tc.list, got, tc.want)
			}
		})
	}
}

// A typo has to stop the process. Accepted silently, SILD_JOBS=webhok,archive
// leaves webhook delivery off on a replica that reports itself healthy, and
// nothing surfaces it until someone notices events stopped arriving.
func TestParseRejectsAnUnknownJob(t *testing.T) {
	for _, list := range []string{"webhok", "webhook,archiv", "push", "webhook,,nonsense"} {
		if _, err := jobs.Parse(list); err == nil {
			t.Fatalf("Parse(%q) accepted an unknown job", list)
		}
	}
}

// A scheduled runner needs a process that finishes: as a Cloud Run Job or a k8s
// CronJob, one that keeps looping is a task that never succeeds — and RunOnce
// drains, so "returns" is a claim about a loop with a termination condition.
func TestRunOnceReturns(t *testing.T) {
	h := testutil.New(t)
	set, err := jobs.Parse(config.DefaultJobs)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	deps := jobs.Deps{Relay: webhook.NewRelay(h.Store), Sweep: archive.NewJob(h.Store, h.Sink, h.Cfg)}
	if err := jobs.RunOnce(context.Background(), set, deps); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
}

// SILD_JOBS="" has to survive config loading as an empty selection, not fall back
// to the default — it is how a Cloud Run serving revision says "no jobs here".
func TestEmptySILDJobsSurvivesConfigLoad(t *testing.T) {
	t.Setenv("SILD_JOBS", "")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	set, err := jobs.Parse(cfg.Jobs.List(config.DefaultJobs))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := set.Names(); len(got) != 0 {
		t.Fatalf("SILD_JOBS=\"\" selected %v", got)
	}
}

func TestUnsetSILDJobsTakesTheBinaryDefault(t *testing.T) {
	t.Setenv("SILD_JOBS", "") // scoped: restored by t.Cleanup
	os.Unsetenv("SILD_JOBS")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	set, err := jobs.Parse(cfg.Jobs.List(config.DefaultJobs))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []string{"webhook", "archive"}
	if got := set.Names(); !slices.Equal(got, want) {
		t.Fatalf("default jobs = %v, want %v", got, want)
	}
}
