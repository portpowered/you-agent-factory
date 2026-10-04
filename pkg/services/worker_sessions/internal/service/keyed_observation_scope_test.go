package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestKeyedRuntimeObservationReadsRejectForeignFactoryScopeBeforeEffects(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"replay-session", "   "} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			reader := &observationEventReaderFake{}
			registry := newObservationRegistry(projector, reader)
			registry.sessions["recorded-worker"] = observationSession("recorded-worker", workersessions.StateCompleted)
			metadata := observationMetadata()
			metadata.factorySessionID = "recording-session"
			registry.observations["recorded-worker"] = metadata
			ctx := context.Background()
			before, err := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: "recording-session",
			})
			if err != nil {
				t.Fatal(err)
			}
			wantErr := workersessions.ErrObservationSessionNotFound
			if scope == "   " {
				wantErr = workersessions.ErrInvalidObservationFactorySessionID
			}
			assertForeignObservationReads(t, registry, scope, wantErr)
			if projector.calls != 0 || reader.readCalls != 0 || reader.subscribeCalls != 0 {
				t.Fatalf("foreign reads reached provider/events: %d/%d/%d", projector.calls, reader.readCalls, reader.subscribeCalls)
			}
			after, err := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: "recording-session",
			})
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("retained recording observation changed: before=%+v after=%+v err=%v", before, after, err)
			}
		})
	}
}

func assertForeignObservationReads(t *testing.T, registry *registry, scope string, wantErr error) {
	t.Helper()
	ctx := context.Background()
	_, getErr := registry.GetObservationByWorkerSessionID(ctx, workersessions.GetObservationByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, transcriptErr := registry.ReadTranscriptByWorkerSessionID(ctx, workersessions.ReadTranscriptByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, canonicalTranscriptErr := registry.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	_, providerTranscriptErr := registry.ReadTranscript(ctx, workersessions.ReadTranscriptRequest{
		ProviderSession: observationProviderRef(), FactorySessionID: scope,
	})
	_, streamErr := registry.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope, ReplayOnly: true,
	})
	_, liveErr := registry.StreamObservationsByWorkerSessionID(ctx, workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "recorded-worker", FactorySessionID: scope,
	})
	for _, err := range []error{getErr, transcriptErr, canonicalTranscriptErr, providerTranscriptErr, streamErr, liveErr} {
		if !errors.Is(err, wantErr) {
			t.Errorf("foreign scope read error = %v, want %v", err, wantErr)
		}
	}
}

func TestKeyedRuntimeObservationReadsRetainSelectedScopeAndDirectCompatibility(t *testing.T) {
	t.Parallel()
	for _, scope := range []string{"", "recording-session", " recording-session "} {
		t.Run(scope, func(t *testing.T) {
			t.Parallel()
			projector := &trackingObservationProjector{}
			registry := newObservationRegistry(projector, nil)
			registry.sessions["recorded-worker"] = observationSession("recorded-worker", workersessions.StateCompleted)
			metadata := observationMetadata()
			metadata.factorySessionID = "recording-session"
			registry.observations["recorded-worker"] = metadata
			result, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{
				WorkerSessionID: "recorded-worker", FactorySessionID: scope,
			})
			if err != nil || result.WorkerSessionID != "recorded-worker" || projector.calls != 1 || projector.request.Session != observationProviderRef() {
				t.Fatalf("selected transcript = %+v, %v; provider calls=%d request=%+v", result, err, projector.calls, projector.request)
			}
		})
	}
}
