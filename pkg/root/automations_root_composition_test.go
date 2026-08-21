package root

import (
	"context"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

func TestAutomationsRootFromEdgesComposesPublishedRoot(t *testing.T) {
	t.Parallel()

	root, err := AutomationsRootFromEdges(
		serviceedges.Edges{},
		"root-automations-root",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("AutomationsRootFromEdges() error = %v", err)
	}
	if root.Operations == nil {
		t.Fatal("AutomationsRootFromEdges() returned root without operations")
	}

	result, err := root.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "root-automations-root",
			SourceID:     "source-a",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
		Observed: []automations.ObservedInstance{{
			AutomationID: "root-automations-root",
			SourceID:     "source-a",
			InstanceID:   "instance-a",
			State:        automations.ObservedLifecycleRunning,
		}},
	})
	if err != nil {
		t.Fatalf("Root.Reconcile() error = %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Root.Reconcile() outcomes = %+v, want one converged source", result.Outcomes)
	}
	if result.Outcomes[0].Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf(
			"Root.Reconcile() convergence = %q, want %q",
			result.Outcomes[0].Convergence,
			automations.ConvergenceStatusConverged,
		)
	}
}

func TestBuildProcessComposesRuntimeMetricsQuery(t *testing.T) {
	t.Parallel()

	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		WorkerRecordingWriter: &rootWorkerRecordingReaderProbe{},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	query := RuntimeMetricsQueryFromProcess(process)
	if query == nil {
		t.Fatal("RuntimeMetricsQueryFromProcess(composed process) returned nil")
	}
	result, err := query.QueryRuntimeMetrics(t.Context(), factoryvisualization.RuntimeMetricsQueryRequest{
		MetricsRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess.QueryRuntimeMetrics() error = %v", err)
	}
	if result.Cost.Availability != factoryvisualization.RuntimeMetricsCostUnavailable {
		t.Fatalf("RuntimeMetricsQueryFromProcess result cost = %#v, want unavailable", result.Cost)
	}
	if len(result.Workstations) != 0 || len(result.WorkerTypes) != 0 || len(result.Providers) != 0 {
		t.Fatalf("RuntimeMetricsQueryFromProcess empty result groups = (%d, %d, %d), want empty",
			len(result.Workstations), len(result.WorkerTypes), len(result.Providers))
	}
}

func TestRuntimeMetricsQueryFromProcessResolvesTypedCapability(t *testing.T) {
	t.Parallel()

	if query := RuntimeMetricsQueryFromProcess(nil); query != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess(nil) = %#v, want nil", query)
	}
	var typedNil *initializerapplication.Process
	if query := RuntimeMetricsQueryFromProcess(typedNil); query != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess(typed nil) = %#v, want nil", query)
	}

	want := factoryvisualization.RuntimeMetricsQuery(func(
		context.Context,
		factoryvisualization.RuntimeMetricsQueryRequest,
	) (factoryvisualization.RuntimeMetricsQueryResult, error) {
		return factoryvisualization.RuntimeMetricsQueryResult{}, nil
	})
	process, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		rootRuntimeMetricsQueryCapabilityProbe{query: want},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(runtime metrics query capability) error = %v", err)
	}
	got := RuntimeMetricsQueryFromProcess(process)
	if got == nil {
		t.Fatal("RuntimeMetricsQueryFromProcess() returned nil query")
	}
	if _, err := got.QueryRuntimeMetrics(context.Background(), factoryvisualization.RuntimeMetricsQueryRequest{}); err != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess() query error = %v, want nil", err)
	}

	wrongType, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		rootRuntimeMetricsQueryCapabilityProbe{query: struct{}{}},
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(wrong runtime metrics query capability) error = %v", err)
	}
	if got := RuntimeMetricsQueryFromProcess(wrongType); got != nil {
		t.Fatalf("RuntimeMetricsQueryFromProcess(wrong type) = %#v, want nil", got)
	}
}

type rootRuntimeMetricsQueryCapabilityProbe struct {
	query any
}

func (probe rootRuntimeMetricsQueryCapabilityProbe) RuntimeMetricsQuery() any {
	return probe.query
}
