package internal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// This component proof observes the exact Workers operation, not replay
// storage or public composition. Both adapters share one execution dependency.
func TestRuntimeWorkerProviderSelectionPreservesNativeLiveExecution(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		executor string
		replay   bool
		override bool
	}{
		{name: "live native", executor: "codex"},
		{name: "live legacy", override: true},
		{name: "live script wrapper", executor: "SCRIPT_WRAP", override: true},
		{name: "replay native", executor: "codex", replay: true, override: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			provider := &struct{ providers.Service }{}
			selected := runtimeWorkersServiceWithProgress{
				providerOverride: provider,
				Service: selectionWorkers{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
					if got := request.Input.ProviderOverride != nil; got != test.override {
						t.Fatalf("provider override selected=%t, want %t", got, test.override)
					}
					return workers.ExecuteResult{}, nil
				}},
			}
			if test.replay {
				selected.replayCommandRunner = &selectionCommandRunner{}
			}
			_, err := selected.Execute(t.Context(), workers.ExecuteRequest{Target: workers.ExecutionTarget{ExecutorProvider: test.executor}})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeWorkerAttemptReplaySelectionPreservesLivePeer(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			admitted := make(chan workers.ExecuteRequest, 3)
			release := make(chan struct{})
			failure := errors.New("selected replay failure")
			execution := selectedReplayWorkers(admitted, release, failure, failed)
			production, replay, live, original := &selectionCommandRunner{}, &selectionCommandRunner{}, &selectionCommandRunner{}, &selectionCommandRunner{}
			peer := runtimeWorkersServiceWithProgress{Service: execution, commandRunnerOverride: live}
			selected := runtimeWorkersServiceWithProgress{Service: execution, commandRunnerOverride: production, replayCommandRunner: replay}
			peerRequest := selectionRequest("live", original)
			peerDone := make(chan error, 1)
			go func() {
				result, err := peer.Execute(ctx, peerRequest)
				if err == nil {
					assertSelectionResult(t, result, peerRequest.Correlation)
				}
				peerDone <- err
			}()
			assertSelectionRequest(t, awaitSelectionRequest(t, ctx, admitted), peerRequest, live)
			replayRequest := selectionRequest("replay", original)
			result, err := selected.Execute(ctx, replayRequest)
			if (failed && !errors.Is(err, failure)) || (!failed && err != nil) {
				t.Fatalf("replay error = %v, failed=%v", err, failed)
			}
			assertSelectionResult(t, result, replayRequest.Correlation)
			assertSelectionRequest(t, awaitSelectionRequest(t, ctx, admitted), replayRequest, replay)
			select {
			case err := <-peerDone:
				t.Fatalf("replay completed gated peer: %v", err)
			default:
			}
			// The shared dependency remains usable with the caller's original
			// runner, despite both runtime-local selections being active.
			direct := selectionRequest("direct", original)
			result, err = execution.Execute(ctx, direct)
			if err != nil {
				t.Fatal(err)
			}
			assertSelectionResult(t, result, direct.Correlation)
			assertSelectionRequest(t, awaitSelectionRequest(t, ctx, admitted), direct, original)
			close(release)
			select {
			case err := <-peerDone:
				if err != nil {
					t.Errorf("peer completion = %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("peer execution did not join")
			}
		})
	}
}

func selectedReplayWorkers(admitted chan<- workers.ExecuteRequest, release <-chan struct{}, failure error, failed bool) selectionWorkers {
	return selectionWorkers{execute: func(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		admitted <- request
		if request.Correlation.RuntimeID == "live" {
			select {
			case <-release:
			case <-ctx.Done():
				return workers.ExecuteResult{}, ctx.Err()
			}
		}
		result := workers.ExecuteResult{Correlation: request.Correlation, StructuredResult: request.Correlation.RuntimeID}
		if failed && request.Correlation.RuntimeID == "replay" {
			return result, failure
		}
		return result, nil
	}}
}

func selectionRequest(runtimeID string, runner platformprocess.CommandRunner) workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{RuntimeID: runtimeID, FactorySessionID: runtimeID + "-factory", GenerationID: runtimeID + "-generation", DispatchID: "same-logical", AttemptID: runtimeID + "-physical", TraceID: runtimeID + "-trace"},
		Input:       workers.ExecutionInput{CommandRunnerOverride: runner},
	}
}

func awaitSelectionRequest(t *testing.T, ctx context.Context, admitted <-chan workers.ExecuteRequest) workers.ExecuteRequest {
	t.Helper()
	select {
	case request := <-admitted:
		return request
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return workers.ExecuteRequest{}
	}
}

func assertSelectionRequest(t *testing.T, got, original workers.ExecuteRequest, runner platformprocess.CommandRunner) {
	t.Helper()
	want := original
	want.Input.CommandRunnerOverride = runner
	if got.Input.CommandRunnerOverride != runner || !reflect.DeepEqual(got, want) {
		t.Errorf("selected Workers request = %+v, want unchanged request with selected runner %p", got, runner)
	}
}

func assertSelectionResult(t *testing.T, result workers.ExecuteResult, correlation workers.ExecutionCorrelation) {
	t.Helper()
	if result.Correlation != correlation || result.StructuredResult != correlation.RuntimeID {
		t.Errorf("selected Workers result changed: %+v, want correlation %+v", result, correlation)
	}
}

type selectionWorkers struct {
	workers.Service
	execute func(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

func (s selectionWorkers) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return s.execute(ctx, request)
}

type selectionCommandRunner struct{ marker byte }

func (*selectionCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("component test must only observe the selected effect")
}

func TestRuntimeWorkerSessionBoundaryPreservesRecordingReader(t *testing.T) {
	t.Parallel()
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failed], func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			failure := errors.New("owned recording read failure")
			source := &selectedWorkerRecordingReader{load: func(actual context.Context, id string) (recordings.WorkerRecordingSnapshot, error) {
				if actual != ctx || id != "owned-recording" {
					t.Fatalf("recording read correlation = %v, %s", actual, id)
				}
				if failed {
					return recordings.WorkerRecordingSnapshot{}, failure
				}
				return recordings.WorkerRecordingSnapshot{RecordingID: id}, nil
			}}
			boundary := runtimeWorkerSessionBoundary{Service: source}
			snapshot, err := boundary.LoadWorkerRecording(ctx, "owned-recording")
			if failed {
				if !errors.Is(err, failure) {
					t.Fatal(err)
				}
				return
			}
			if err != nil || snapshot.RecordingID != "owned-recording" {
				t.Fatalf("selected recording = %#v, %v", snapshot, err)
			}
		})
	}
}

type selectedWorkerRecordingReader struct {
	workersessions.Service
	load func(context.Context, string) (recordings.WorkerRecordingSnapshot, error)
}

func (reader *selectedWorkerRecordingReader) LoadWorkerRecording(ctx context.Context, id string) (recordings.WorkerRecordingSnapshot, error) {
	return reader.load(ctx, id)
}

func TestRuntimeWorkerResolutionRetainsDirectExecutionContext(t *testing.T) {
	t.Parallel()
	original := &workers.Context{SessionID: "resolved-owner"}
	resolver := directSelectionResolver{context: original}
	correlation := workers.ExecutionCorrelation{FactorySessionID: "worker-session", RuntimeID: "selected-runtime", GenerationID: "selected-runtime", DispatchID: "dispatch", AttemptID: "dispatch", RequestID: "request"}
	progress := func(workers.ProgressFragment) {}
	selected := runtimeWorkersServiceWithProgress{factorySessionID: "resolved-owner", workstationResolver: resolver, Service: selectionWorkers{execute: func(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
		if request.Correlation != correlation || request.Input.WorkflowContext.SessionID != correlation.FactorySessionID || request.Input.ProgressPublisher == nil {
			t.Errorf("resolved execution lost admitted correlation: %#v", request)
		}
		return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeAccepted}, nil
	}}}
	if _, err := selected.Execute(context.Background(), workers.ExecuteRequest{Correlation: correlation, Input: workers.ExecutionInput{ProgressPublisher: progress}}); err != nil {
		t.Fatal(err)
	}
	if original.SessionID != "resolved-owner" {
		t.Fatal("resolution mutated the retained workflow context")
	}
}

func TestRuntimeWorkerResolutionDeliversAdmittedAttemptControl(t *testing.T) {
	t.Parallel()
	for _, withProgress := range []bool{false, true} {
		t.Run(map[bool]string{false: "without progress", true: "with progress"}[withProgress], func(t *testing.T) {
			t.Parallel()
			owned := &projectedAttemptControl{identity: "owned-attempt"}
			var captured providers.AttemptControl
			request := workers.ExecuteRequest{Input: workers.ExecutionInput{
				AttemptControlObserver: func(control providers.AttemptControl) { captured = control },
			}}
			if withProgress {
				request.Input.ProgressPublisher = func(workers.ProgressFragment) {}
			}
			selected := runtimeWorkersServiceWithProgress{
				workstationResolver: directSelectionResolver{
					context: &workers.Context{},
					observe: func(projected workers.WorkstationExecutionRequest) {
						if projected.AttemptControlObserver == nil {
							t.Fatal("resolver request dropped the admitted attempt observer")
						}
					},
				},
				Service: selectionWorkers{execute: func(_ context.Context, resolved workers.ExecuteRequest) (workers.ExecuteResult, error) {
					if resolved.Input.AttemptControlObserver == nil {
						t.Fatal("target resolution dropped the admitted attempt observer")
					}
					resolved.Input.AttemptControlObserver(owned)
					return workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, nil
				}},
			}
			if _, err := selected.Execute(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			if captured != owned {
				t.Fatal("admitted observer did not receive the selected execution's handle")
			}
		})
	}
}

type projectedAttemptControl struct{ identity string }

func (*projectedAttemptControl) ForceKill(context.Context) (bool, error) { return false, nil }

type directSelectionResolver struct {
	context *workers.Context
	observe func(workers.WorkstationExecutionRequest)
}

func (resolver directSelectionResolver) ResolveExecutionRequest(request workers.WorkstationExecutionRequest) (workers.ExecuteRequest, error) {
	if resolver.observe != nil {
		resolver.observe(request)
	}
	return workers.ExecuteRequest{Input: workers.ExecutionInput{WorkflowContext: resolver.context}}, nil
}

type artifactAttemptsFake struct {
	factory.WorkerAttemptOpener
	observed string
}

func (f *artifactAttemptsFake) AdmitRuntimeAttemptAsync(_ context.Context, request workersessions.StartRequest, _ workers.Service, _ platformclock.Source, _ platformclock.TimerSource) (workersessions.StartResult, error) {
	f.observed = request.Execution.Execution.OriginatingArtifact
	return workersessions.StartResult{}, nil
}
func (f *artifactAttemptsFake) BeginRuntimeAttempt(_ context.Context, request workersessions.RuntimeAttemptRequest, _ workers.Service, _ platformclock.Source, _ platformclock.TimerSource, _ func(context.Context) (workers.WorkstationDispatchCancelOutcome, error)) (workersessions.RuntimeAttempt, error) {
	f.observed = request.Execution.Execution.OriginatingArtifact
	return nil, nil
}
func (f *artifactAttemptsFake) InvokeRuntimeSession(_ context.Context, request workersessions.RuntimeAttemptRequest, _ workersessions.RetryPolicy, _ workers.Service, _ platformclock.Source, _ platformclock.TimerSource) (workersessions.InvokeSessionResult, error) {
	f.observed = request.Execution.Execution.OriginatingArtifact
	return workersessions.InvokeSessionResult{}, nil
}

func TestWorkerWorkAttributionOriginatingArtifactAdmission(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"async", "begin", "invoke"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			fake := &artifactAttemptsFake{}
			bound := artifactWorkerAttempts{WorkerAttemptOpener: fake, artifact: "selected-origin.jsonl"}
			request := workersessions.RuntimeAttemptRequest{}
			request.Execution.Execution.OriginatingArtifact = "caller-substitution"
			var err error
			switch mode {
			case "async":
				_, err = bound.AdmitRuntimeAttemptAsync(t.Context(), workersessions.StartRequest{Execution: request.Execution}, nil, nil, nil)
			case "begin":
				_, err = bound.BeginRuntimeAttempt(t.Context(), request, nil, nil, nil, nil)
			case "invoke":
				_, err = bound.InvokeRuntimeSession(t.Context(), request, workersessions.RetryPolicy{}, nil, nil, nil)
			}
			if err != nil || fake.observed != bound.artifact || request.Execution.Execution.OriginatingArtifact != "caller-substitution" {
				t.Fatalf("capture provenance=%q, request=%q, error=%v", fake.observed, request.Execution.Execution.OriginatingArtifact, err)
			}
		})
	}
}
