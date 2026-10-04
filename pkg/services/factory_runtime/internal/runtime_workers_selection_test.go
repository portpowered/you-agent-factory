package internal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// This component proof observes the exact Workers operation, not replay
// storage or public composition. Both adapters share one execution dependency.
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
