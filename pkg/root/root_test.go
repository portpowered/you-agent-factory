package root

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
			assertSelectedProcessTimestamp(t, gotClock, clock, base)
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

func assertSelectedProcessTimestamp(t *testing.T, selected, supplied platformclock.Source, want time.Time) {
	t.Helper()
	if supplied == nil {
		if _, ok := selected.(platformclock.Real); !ok {
			t.Fatalf("omitted Clock selected %T", selected)
		}
		return
	}
	if got := selected.Now(); !got.Equal(want) {
		t.Fatalf("selected timestamp = %s, want %s", got, want)
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
			test.edges.RecordingsRootObserver = func(recordings.Service) { panic("constructor observed rejected time override") }
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

func TestConstructionEffectPresence(t *testing.T) {
	t.Parallel()
	var pointer *int
	var function func()
	var channel chan int
	var values []int
	var mapping map[string]int
	tests := []struct {
		name    string
		value   any
		missing bool
	}{
		{"nil", nil, true}, {"pointer", pointer, true},
		{"function", function, true}, {"channel", channel, true},
		{"slice", values, true}, {"map", mapping, true},
		{"valid pointer", new(int), false}, {"value", 0, false},
		{"disabled function", func() { panic("must not invoke") }, false},
		{"empty slice", []int{}, false}, {"empty map", map[string]int{}, false},
		{"valid channel", make(chan int), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isNilConstructionEffect(test.value); got != test.missing {
				t.Fatalf("missing = %v, want %v", got, test.missing)
			}
		})
	}
}

type uncalledContext struct{ context.Context }
type uncalledRunner struct{ platformprocess.CommandRunner }
type uncalledClock struct{ platformclock.Source }
type uncalledProcessClock struct{ platformprocess.Clock }
type uncalledDirectory struct {
	platformfilesystem.WorkingDirectory
}

func TestBuildProcessRejectsMissingRegistrationIntegration(t *testing.T) {
	t.Parallel()
	edges := serviceedges.Edges{
		RecordingsRootObserver: func(recordings.Service) { panic("constructor ran") },
	}
	// Allocate a zero registration through the caller's Edges contract without
	// importing the Providers-private construction package. Its omitted
	// Integration is invalid even though top-level optional effects may be nil.
	registrations := reflect.ValueOf(&edges.ProviderRegistrations).Elem()
	registrations.Set(reflect.MakeSlice(registrations.Type(), 1, 1))
	process, err := BuildProcess(context.Background(), edges)
	if process != nil || err == nil || !strings.Contains(err.Error(), "integration is required") {
		t.Fatalf("BuildProcess = (%v, %v), want integration required", process, err)
	}
}

func TestACPWireLogSettingsCaptureNormalizedConfiguration(t *testing.T) {
	t.Parallel()
	for _, disabled := range []string{"off", " OFF ", "on", ""} {
		t.Run(disabled, func(t *testing.T) {
			calls := 0
			settings := resolveACPWireLogSettings(func(key string) string {
				calls++
				switch key {
				case "YOU_ACP_WIRE_LOG":
					return disabled
				case "YOU_ACP_WIRE_LOG_DIR":
					return " /owned/transcripts "
				default:
					t.Fatalf("unexpected lookup %q", key)
					return ""
				}
			})
			if settings.Disabled != strings.EqualFold(strings.TrimSpace(disabled), "off") ||
				settings.Directory != "/owned/transcripts" || calls != 2 {
				t.Fatalf("settings = %+v, lookups = %d", settings, calls)
			}
		})
	}
}

func TestBuildProcessRejectsInvalidOverridesBeforeEffects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		edges serviceedges.Edges
		want  string
	}{
		{"early", serviceedges.Edges{ProviderCommandRunner: (*uncalledRunner)(nil)}, "Edges.ProviderCommandRunner"},
		{"middle", serviceedges.Edges{FactorySessionsWorkingDirectory: (*uncalledDirectory)(nil)}, "Edges.FactorySessionsWorkingDirectory"},
		{"clock", serviceedges.Edges{Clock: (*uncalledClock)(nil)}, "construct Recordings: clock is required"},
		{"scheduler", serviceedges.Edges{ProcessScheduler: (*platformclock.Deterministic)(nil)}, "ProcessScheduler must not be typed-nil"},
		{"late", serviceedges.Edges{FactoryVisualizationSink: factoryvisualization.SinkFunc(nil)}, "Edges.FactoryVisualizationSink"},
		{"first error", serviceedges.Edges{ProviderCommandRunner: (*uncalledRunner)(nil), Clock: (*uncalledClock)(nil)}, "Edges.ProviderCommandRunner"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Valid early effects must remain untouched even for a late invalid port.
			test.edges.PlatformProcessClock = &uncalledProcessClock{}
			test.edges.RecordingsRootObserver = func(recordings.Service) { panic("constructor observer invoked") }
			process, err := BuildProcess(context.Background(), test.edges)
			if process != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("BuildProcess = (%v, %v), want %q", process, err, test.want)
			}
		})
	}
}

func TestBuildProcessRejectsMissingAndCancelledContext(t *testing.T) {
	t.Parallel()
	for _, ctx := range []context.Context{nil, (*uncalledContext)(nil)} {
		process, err := BuildProcess(ctx, serviceedges.Edges{})
		if process != nil || err == nil || err.Error() != "build application process: context is required" {
			t.Fatalf("BuildProcess = (%v, %v), want context required", process, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process, err := BuildProcess(ctx, serviceedges.Edges{})
	if process != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("BuildProcess = (%v, %v), want cancellation cause", process, err)
	}
}

func TestConstructionOverridesAllowOmissionAndValidEffects(t *testing.T) {
	t.Parallel()
	for _, edges := range []serviceedges.Edges{
		{},
		{ProviderCommandRunner: &uncalledRunner{}, Clock: &uncalledClock{}},
		{FactoryVisualizationSink: factoryvisualization.SinkFunc(func(factoryvisualization.View) { panic("must not invoke") })},
	} {
		if err := validateConstructionOverrides(edges); err != nil {
			t.Fatalf("validateConstructionOverrides = %v", err)
		}
	}
}
