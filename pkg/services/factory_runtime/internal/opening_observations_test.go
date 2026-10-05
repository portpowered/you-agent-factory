package internal_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// The resource effect captures the scoped capabilities it receives. Calling
// those capabilities after another opening tests their lifetime, without
// assembling peer services or claiming public durable persistence evidence.
func TestBundleOpeningKeepsMutationAndProgressObserversWithTheirOpening(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	loaded, err := loadedFactoryFixture(dir)
	if err != nil {
		t.Fatal(err)
	}
	effect := &observationOpeningEffect{}
	sessions := &observationOpeningSessions{stubWorkerSessionsService: &stubWorkerSessionsService{}}
	opening, err := factoryinternal.NewBundleOpening(effect.Open, platformclock.Real{}, observationOpeningWorker{}, sessions, sessions, nil, &testRuntimeScopeServiceStub{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var mutations, progress []string
	mutationErr := errors.New("first observer persistence failure")
	cases := []struct {
		identity, sessionID string
		mutationErr         error
	}{{"first", "first", mutationErr}, {"second", "second", nil}, {"retry", "first", nil}}
	for _, cell := range cases {
		spec := factory.SessionBuildSpec{Dir: dir, FolderPath: dir, SessionID: cell.sessionID,
			RuntimeInstanceID: "runtime-" + cell.sessionID, LoadedFactoryCfg: loaded, Clock: clockwork.NewFakeClock(), BaseLogger: zap.NewNop(),
			PetriMutationRecorder: func(sessionID string, records []definitions.TokenMutationRecord) error {
				mutations = append(mutations, cell.identity+":"+sessionID+":"+records[0].TokenID)
				return cell.mutationErr
			},
		}
		_, err := opening.Open(t.Context(), spec, "", factory.RuntimeLogStorageConfig{}, factory.RuntimeFileLoggingPolicyDisabled,
			factory.RuntimeMetricsPolicyDisabled, "", factory.RuntimeMetricsStorageConfig{}, 0, cell.sessionID,
			definitions.RuntimeModeBatch, nil, false, "", "", false, false, nil, nil,
			func(fragment workers.ProgressFragment) {
				progress = append(progress, cell.identity+":"+fragment.Payload)
			},
			nil)
		if !errors.Is(err, effect.failure) {
			t.Fatalf("%s opening error = %v, want controlled resource failure", cell.identity, err)
		}
	}
	for index, cell := range cases {
		observer := effect.calls[index]
		err := observer.mutation(cell.sessionID, []definitions.TokenMutationRecord{{TokenID: "token-" + cell.identity}})
		if !errors.Is(err, cell.mutationErr) {
			t.Fatalf("%s mutation callback error = %v, want %v", cell.identity, err, cell.mutationErr)
		}
		if _, err := observer.worker.Execute(t.Context(), workers.ExecuteRequest{
			Correlation: workers.ExecutionCorrelation{DispatchID: "dispatch-" + cell.identity},
		}); err != nil {
			t.Fatalf("%s retained Worker execution: %v", cell.identity, err)
		}
	}
	if want := []string{"first:first:token-first", "second:second:token-second", "retry:first:token-retry"}; !reflect.DeepEqual(mutations, want) {
		t.Fatalf("mutation observations = %v, want %v", mutations, want)
	}
	if want := []string{"first:dispatch-first", "second:dispatch-second", "retry:dispatch-retry"}; !reflect.DeepEqual(progress, want) {
		t.Fatalf("progress observations = %v, want %v", progress, want)
	}
	if want := []string{"runtime-first:dispatch-first", "runtime-second:dispatch-second", "runtime-first:dispatch-retry"}; !reflect.DeepEqual(sessions.progressScopes, want) {
		t.Fatalf("supervised progress scopes = %v, want %v", sessions.progressScopes, want)
	}
}

type observationOpeningCall struct {
	mutation factory.PetriMutationRecorder
	worker   workers.Service
}

type observationOpeningEffect struct {
	calls   []observationOpeningCall
	failure error
}

func (effect *observationOpeningEffect) Open(
	_ context.Context, _ *zap.Logger,
	_, _, _, _, _ string, _ definitions.RuntimeMode, _ bool, _ factory.Scheduler, _ bool,
	_ string, _ factory.RuntimeLogStorageConfig, _ factory.RuntimeFileLoggingPolicy,
	_ factory.RuntimeMetricsPolicy, _ string, _ factory.RuntimeMetricsStorageConfig,
	_ factory.LoadedConfig, _, _ string, _ factory.Clock, _ string,
	_ *definitions.FactorySnapshot, _ *definitions.FactoryWorldState, _ bool,
	_ []factory.SubmissionHook, _ factory.CompletionDeliveryPlanner,
	mutation factory.PetriMutationRecorder,
	_ time.Duration, _ []definitions.FactoryEvent, worker workers.Service, _ workersessions.Service,
	_ factory.WorkerAttemptOpener, _ func(string), _ ...*workers.MockWorkersConfig,
) (*factoryhost.Bundle, error) {
	effect.calls = append(effect.calls, observationOpeningCall{mutation: mutation, worker: worker})
	if effect.failure == nil {
		effect.failure = errors.New("controlled resource opening failure")
	}
	return nil, effect.failure
}

type observationOpeningWorker struct{ workers.Service }

func (observationOpeningWorker) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	request.Input.ProgressPublisher(workers.ProgressFragment{Payload: request.Correlation.DispatchID, Correlation: request.Correlation})
	return workers.ExecuteResult{}, nil
}

type observationOpeningSessions struct {
	*stubWorkerSessionsService
	progressScopes []string
}

func (sessions *observationOpeningSessions) PublishRuntimeProgress(_ context.Context, key workersessions.RuntimeAttemptKey,
	fragment workers.ProgressFragment, next workers.ProgressPublisher,
) error {
	sessions.progressScopes = append(sessions.progressScopes, key.RuntimeID+":"+key.DispatchID)
	next(fragment)
	return nil
}
