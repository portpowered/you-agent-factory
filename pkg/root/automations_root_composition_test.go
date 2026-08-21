package root

import (
	"context"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	costs "github.com/portpowered/infinite-you/pkg/services/costs"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
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
	if query := CostsQueryFromProcess(process); query == nil {
		t.Fatal("CostsQueryFromProcess(composed process) returned nil")
	}
}

func TestCostsQueryFromProcessResolvesTypedCapability(t *testing.T) {
	t.Parallel()

	if query := CostsQueryFromProcess(nil); query != nil {
		t.Fatalf("CostsQueryFromProcess(nil) = %#v, want nil", query)
	}
	want := costs.CostsQuery(func(context.Context, costs.QueryRequest) (costs.Report, error) {
		return costs.Report{Status: costs.StatusNoUsage}, nil
	})
	process, err := initializerapplication.NewProcessWithRuntimeCostsAndExecution(
		nil, nil, rootWorkerProcessRegistry{}, rootWorkerProcessLifecycle{}, nil, nil, nil, nil, nil,
		rootRuntimeCostsQueryCapabilityProbe{query: want},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecution() error = %v", err)
	}
	got := CostsQueryFromProcess(process)
	if got == nil {
		t.Fatal("CostsQueryFromProcess() returned nil query")
	}
	if result, err := got.Query(context.Background(), costs.QueryRequest{}); err != nil || result.Status != costs.StatusNoUsage {
		t.Fatalf("CostsQueryFromProcess() result = %#v, error = %v", result, err)
	}

	wrongType, err := initializerapplication.NewProcessWithRuntimeCostsAndExecution(
		nil, nil, rootWorkerProcessRegistry{}, rootWorkerProcessLifecycle{}, nil, nil, nil, nil, nil,
		rootRuntimeCostsQueryCapabilityProbe{query: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecution(wrong type) error = %v", err)
	}
	if got := CostsQueryFromProcess(wrongType); got != nil {
		t.Fatalf("CostsQueryFromProcess(wrong type) = %#v, want nil", got)
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

func TestExecutionRuntimeOpeningFromProcessResolvesTypedCapability(t *testing.T) {
	t.Parallel()

	if opening := ExecutionRuntimeOpeningFromProcess(nil); opening != nil {
		t.Fatalf("ExecutionRuntimeOpeningFromProcess(nil) = %#v, want nil", opening)
	}
	processWithoutOpening, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("NewProcess(without execution opening) error = %v", err)
	}
	if opening := ExecutionRuntimeOpeningFromProcess(processWithoutOpening); opening != nil {
		t.Fatalf("ExecutionRuntimeOpeningFromProcess(without capability) = %#v, want nil", opening)
	}

	wrongType, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil, nil, nil,
		rootExecutionRuntimeOpeningCapabilityProbe{opening: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcess(wrong-type execution opening) error = %v", err)
	}
	if opening := ExecutionRuntimeOpeningFromProcess(wrongType); opening != nil {
		t.Fatalf("ExecutionRuntimeOpeningFromProcess(wrong type) = %#v, want nil", opening)
	}

	want := factorysessions.OpenedExecutionRuntime{Close: func() error { return nil }}
	request := factorysessions.ExecutionRuntimeOpeningRequest{
		ProjectRoot:      "project-root",
		SystemConfigHome: "system-config-home",
		FactorySessionID: "session-1",
		ReplayPath:       "recording.json",
	}
	process, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil, nil, nil, nil,
		rootExecutionRuntimeOpeningCapabilityProbe{
			opening: factorysessions.ExecutionRuntimeOpeningFunc(func(ctx context.Context, got factorysessions.ExecutionRuntimeOpeningRequest) (factorysessions.OpenedExecutionRuntime, error) {
				if ctx == nil || got.ProjectRoot != request.ProjectRoot || got.ReplayPath != request.ReplayPath {
					t.Errorf("execution opening request = (%v, %#v), want context and %#v", ctx, got, request)
				}
				return want, nil
			}),
		},
	)
	if err != nil {
		t.Fatalf("NewProcess(execution opening) error = %v", err)
	}
	opening := ExecutionRuntimeOpeningFromProcess(process)
	if opening == nil {
		t.Fatal("ExecutionRuntimeOpeningFromProcess(composed capability) returned nil")
	}
	got, err := opening.OpenExecutionRuntime(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenExecutionRuntime() error = %v", err)
	}
	if got.Execution != want.Execution || got.Close == nil {
		t.Fatalf("OpenExecutionRuntime() = %#v, want execution and close capability", got)
	}
}

type rootExecutionRuntimeOpeningCapabilityProbe struct {
	opening any
}

func (probe rootExecutionRuntimeOpeningCapabilityProbe) ExecutionRuntimeOpening() any {
	return probe.opening
}

type rootRuntimeMetricsQueryCapabilityProbe struct {
	query any
}

type rootRuntimeCostsQueryCapabilityProbe struct {
	query any
}

func (probe rootRuntimeCostsQueryCapabilityProbe) RuntimeCostsQuery() any {
	return probe.query
}

func (probe rootRuntimeMetricsQueryCapabilityProbe) RuntimeMetricsQuery() any {
	return probe.query
}
