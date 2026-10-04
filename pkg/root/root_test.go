package root

import (
	"context"
	"errors"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/spf13/cobra"
)

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
