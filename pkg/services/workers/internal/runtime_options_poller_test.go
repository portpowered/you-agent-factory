package internal

import (
	"fmt"
	"testing"
	"time"

	runtimefixtures "github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// TestBuildRuntimeExecutorsOmitsAutomationsOwnedPollerWorkers proves hosted and
// poller Worker shapes never enter Workers executor construction, including the
// former NoopExecutor silent-accept fallback.
func TestBuildRuntimeExecutorsOmitsAutomationsOwnedPollerWorkers(t *testing.T) {
	t.Parallel()

	for _, workerType := range []string{
		interfaces.WorkerTypeHosted,
		interfaces.WorkerTypePoller,
	} {
		workerType := workerType
		t.Run(workerType, func(t *testing.T) {
			t.Parallel()

			worker := &interfaces.FactoryWorkerConfig{
				Name:     "linear-poller",
				Type:     workerType,
				Provider: interfaces.HostedWorkerProviderLinear,
			}
			factoryConfig := &interfaces.FactoryConfig{
				Workers: []interfaces.FactoryWorkerConfig{*worker},
				Workstations: []interfaces.FactoryWorkstationConfig{{
					Name:           "poll-linear",
					Kind:           interfaces.WorkstationKindPoller,
					WorkerTypeName: worker.Name,
				}},
			}
			runtimeConfig := runtimefixtures.RuntimeConfigLookupFixture{
				Factory: factoryConfig,
				Workers: map[string]*interfaces.FactoryWorkerConfig{
					worker.Name: worker,
				},
				Workstations: map[string]*interfaces.FactoryWorkstationConfig{
					"poll-linear": {
						Name:           "poll-linear",
						Kind:           interfaces.WorkstationKindPoller,
						WorkerTypeName: worker.Name,
					},
				},
			}
			builder := &pollerOmissionBuilder{t: t}
			service := &Service{
				executorBuilder:   builder,
				clock:             time.Now,
				executableLocator: stubExecutableLocator{},
			}

			executors, err := service.BuildRuntimeExecutors(
				runtimeConfig,
				factoryConfig,
				"",
				nil,
				logging.NoopLogger{},
				true,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				time.Now,
			)
			if err != nil {
				t.Fatalf("BuildRuntimeExecutors() error = %v", err)
			}
			if builder.calls != 0 {
				t.Fatalf("executor builder calls = %d, want zero for Automations-owned pollers", builder.calls)
			}
			if _, ok := executors[worker.Name]; ok {
				t.Fatalf("executors[%q] present, want omitted (not NoopExecutor)", worker.Name)
			}
			if len(executors) != 0 {
				t.Fatalf("executors = %#v, want empty map", executors)
			}
		})
	}
}

type pollerOmissionBuilder struct {
	t     *testing.T
	calls int
}

func (b *pollerOmissionBuilder) Build(
	interfaces.RuntimeConfigLookup,
	string,
	string,
	*workers.Context,
	logging.Logger,
	*bool,
	workers.Provider,
	workers.ProgressPublisher,
	workerexecutor.ScriptEventRecorder,
	workers.InferenceEventRecorder,
	workeragentrun.AgentRunEventRecorder,
	func() time.Time,
	func() []string,
	func() (string, error),
	[]workerconstruction.RunnerDecorator,
) (workerconstruction.Result, error) {
	b.calls++
	b.t.Fatal("executor builder must not construct Automations-owned poller workers")
	return workerconstruction.Result{}, fmt.Errorf("unexpected poller executor construction")
}

func (b *pollerOmissionBuilder) BuildLogical(
	interfaces.RuntimeConfigLookup,
	string,
	string,
	*workers.Context,
	logging.Logger,
	func() time.Time,
	func() []string,
	func() (string, error),
) workerconstruction.Result {
	b.calls++
	b.t.Fatal("logical builder must not construct Automations-owned poller workers")
	return workerconstruction.Result{}
}

type stubExecutableLocator struct{}

func (stubExecutableLocator) LookPath(string) (string, error) {
	return "", fmt.Errorf("executable lookup disabled in poller omission test")
}
