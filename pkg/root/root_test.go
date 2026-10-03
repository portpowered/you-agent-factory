package root

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/spf13/cobra"
)

type nowOnlyProcessClock func() time.Time

func (clock nowOnlyProcessClock) Now() time.Time { return clock() }

func TestNormalizeProcessTimeSelection(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"omitted", "timer-capable", "legacy", "explicit", "legacy-explicit", "scheduler-only"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := time.Unix(42, 0).UTC()
			logical := platformclock.NewDeterministic(base, time.Second)
			explicit := platformclock.NewDeterministic(base.Add(time.Hour), time.Second)
			legacy := nowOnlyProcessClock(func() time.Time { return base })
			var clock platformclock.Source
			var scheduler platformclock.TimerSource
			switch name {
			case "timer-capable":
				clock = logical
			case "legacy":
				clock = legacy
			case "explicit":
				clock, scheduler = logical, explicit
			case "legacy-explicit":
				clock, scheduler = legacy, explicit
			case "scheduler-only":
				scheduler = explicit
			}
			gotClock, gotScheduler, err := normalizeProcessTime(clock, scheduler)
			if err != nil {
				t.Fatal(err)
			}
			if clock == nil {
				if _, ok := gotClock.(platformclock.Real); !ok {
					t.Fatalf("omitted Clock selected %T", gotClock)
				}
			} else if got := gotClock.Now(); !got.Equal(base) {
				t.Fatalf("selected timestamp = %s, want %s", got, base)
			}
			switch name {
			case "explicit", "legacy-explicit", "scheduler-only":
				if gotScheduler != explicit {
					t.Fatal("explicit scheduler replaced")
				}
				assertSelectedLogicalScheduler(t, gotScheduler, explicit)
			case "timer-capable":
				if gotClock != logical || gotScheduler != logical {
					t.Fatal("timer-capable clock replaced")
				}
				assertSelectedLogicalScheduler(t, gotScheduler, logical)
			default:
				if _, ok := gotScheduler.(platformclock.Real); !ok {
					t.Fatalf("wall scheduler selected %T", gotScheduler)
				}
			}
		})
	}
}

func assertSelectedLogicalScheduler(t *testing.T, scheduler platformclock.TimerSource, logical *platformclock.Deterministic) {
	t.Helper()
	base := logical.Now()
	after, timer := scheduler.After(time.Second), scheduler.NewTimer(time.Second)
	logical.SetTick(1)
	for _, channel := range []<-chan time.Time{after, timer.C()} {
		select {
		case got := <-channel:
			if !got.Equal(base.Add(time.Second)) {
				t.Fatalf("scheduler delivery = %s", got)
			}
		default:
			t.Fatal("selected scheduler did not advance logically")
		}
	}
}

func TestNormalizeProcessTimeRejectsTypedNil(t *testing.T) {
	t.Parallel()
	var pointer *platformclock.Deterministic
	var function nowOnlyProcessClock
	for _, test := range []struct {
		name      string
		clock     platformclock.Source
		scheduler platformclock.TimerSource
		field     string
	}{
		{"clock-pointer", pointer, nil, "Clock"},
		{"clock-function", function, nil, "Clock"},
		{"scheduler", nil, pointer, "ProcessScheduler"},
		{"invalid-scheduler-with-valid-clock", platformclock.Real{}, pointer, "ProcessScheduler"},
		{"invalid-clock-with-valid-scheduler", pointer, platformclock.Real{}, "Clock"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clock, scheduler, err := normalizeProcessTime(test.clock, test.scheduler)
			if err == nil || !strings.Contains(err.Error(), test.field+" must not be typed-nil") {
				t.Fatalf("expected actionable %s error, got %v", test.field, err)
			}
			if clock != nil || scheduler != nil {
				t.Fatal("invalid override produced a default time pair")
			}
		})
	}
}

func TestBuildProcessRejectsInvalidContextAndTimeBeforeComposition(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var typedNil *platformclock.Deterministic
	for _, test := range []struct {
		name    string
		ctx     context.Context
		edges   serviceedges.Edges
		message string
	}{
		{"nil-context", nil, serviceedges.Edges{}, "context is required"},
		{"cancelled-context", ctx, serviceedges.Edges{Clock: typedNil}, "context canceled"},
		{"invalid-clock", context.Background(), serviceedges.Edges{Clock: typedNil}, "Clock must not be typed-nil"},
		{"invalid-scheduler", context.Background(), serviceedges.Edges{ProcessScheduler: typedNil}, "ProcessScheduler must not be typed-nil"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			process, err := BuildProcess(test.ctx, test.edges)
			if process != nil || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("BuildProcess invalid input = %v, %v", process, err)
			}
			if test.name == "cancelled-context" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled context classification lost: %v", err)
			}
		})
	}
}

func TestMain(m *testing.M) {
	// Process tests execute Cobra commands in-process. Explorer launch behavior
	// is outside this package's contract, and its Windows process scan dominates
	// the cost of repeated Execute calls.
	cobra.MousetrapHelpText = ""
	m.Run()
}
