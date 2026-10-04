package runtime

import (
	"context"
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func (service *runtimeWorkerSessionsService) CloseRuntimeAttempts(context.Context, string) error {
	return nil
}

type scopedWorkerReadProbe struct {
	workersessions.Service
	scopes []string
	err    error
}

func (probe *scopedWorkerReadProbe) GetObservationByWorkerSessionID(_ context.Context, req workersessions.GetObservationByWorkerSessionIDRequest) (workersessions.Observation, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.Observation{}, probe.err
}

func (probe *scopedWorkerReadProbe) ReadTranscript(_ context.Context, req workersessions.ReadTranscriptRequest) (workersessions.ReadTranscriptResult, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.ReadTranscriptResult{}, probe.err
}

func (probe *scopedWorkerReadProbe) StreamObservationsByWorkerSessionID(_ context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	probe.scopes = append(probe.scopes, req.FactorySessionID)
	return workersessions.ObservationSubscription{}, probe.err
}

func TestWorkerSessionReadFallbackCarriesOwningFactoryScope(t *testing.T) {
	t.Parallel()
	for _, requested := range []string{"", " recording-session ", "replay-session"} {
		t.Run(requested, func(t *testing.T) {
			t.Parallel()
			probe := &scopedWorkerReadProbe{err: errors.New("selected read effect")}
			reader := &recordedWorkerSessionObservation{Service: probe, factorySessionID: "recording-session"}
			ctx := context.Background()
			_, getErr := reader.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			_, transcriptErr := reader.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			_, streamErr := reader.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: requested,
			})
			wantErr, wantCalls := probe.err, 3
			if requested == "replay-session" {
				wantErr, wantCalls = workersessions.ErrObservationSessionNotFound, 0
			}
			for _, err := range []error{getErr, transcriptErr, streamErr} {
				if !errors.Is(err, wantErr) {
					t.Errorf("read error = %v, want %v", err, wantErr)
				}
			}
			if len(probe.scopes) != wantCalls {
				t.Fatalf("read calls = %d, want %d", len(probe.scopes), wantCalls)
			}
			for _, scope := range probe.scopes {
				if scope != "recording-session" {
					t.Errorf("selected read scope = %q, want recording-session", scope)
				}
			}
		})
	}
}
