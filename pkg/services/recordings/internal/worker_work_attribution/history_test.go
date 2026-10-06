package workerworkattribution

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type canonicalQueryFake struct {
	requests []recordings.HistoricalRecordingQueryRequest
	result   recordings.HistoricalRecordingQueryResult
	err      error
	cancel   context.CancelFunc
}

func (f *canonicalQueryFake) QueryHistoricalRecording(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	f.requests = append(f.requests, request)
	if f.cancel != nil {
		f.cancel()
	}
	return f.result, f.err
}

func TestWorkerWorkAttributionExactAndLegacyArtifact(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"/original/selected.json", ""} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			query := &canonicalQueryFake{result: recordings.HistoricalRecordingQueryResult{Recording: recordings.HistoricalRecordingIdentity{RecordingID: "capture"}}}
			var scopes []string
			reader := NewArtifactHistoryReader(query, func(_ context.Context, scope string) (recordings.RecordingArtifactReference, error) {
				scopes = append(scopes, scope)
				return "/current/scope.json", nil
			})
			page := recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{RecordingID: "capture", FactorySessionID: "scope", OriginatingArtifact: artifact}}
			got, err := reader.ReadWorkerFactoryHistory(t.Context(), page)
			wantArtifact := artifact
			wantScopes := []string(nil)
			if artifact == "" {
				wantArtifact = "/current/scope.json"
				wantScopes = []string{"scope"}
			}
			wantRequest := recordings.HistoricalRecordingQueryRequest{InferFactorySessionScope: true, Recording: recordings.HistoricalRecordingIdentity{RecordingID: "capture", Artifact: recordings.RecordingArtifactReference(wantArtifact), Scope: recordings.CanonicalEventScope{FactorySessionID: "scope"}}}
			if err != nil || !reflect.DeepEqual(got, query.result) || !reflect.DeepEqual(query.requests, []recordings.HistoricalRecordingQueryRequest{wantRequest}) || !reflect.DeepEqual(scopes, wantScopes) {
				t.Fatalf("history = %+v, %v; requests=%+v scopes=%v", got, err, query.requests, scopes)
			}
		})
	}
}

func TestWorkerWorkAttributionArtifactFailuresAndCancellation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"missing-legacy", "unavailable-legacy", "corrupt-exact", "canceled", "cancel-during-read"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			query := &canonicalQueryFake{}
			page := recordings.WorkerCapturedActivityPage{Catalog: recordings.WorkerSessionCatalogEntry{RecordingID: "capture", FactorySessionID: "scope"}}
			var fallbackErr error
			var want error
			switch scenario {
			case "unavailable-legacy":
				fallbackErr = &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorUnavailable}
				want = fallbackErr
			case "corrupt-exact":
				page.Catalog.OriginatingArtifact = "exact.json"
				query.err = &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorCorruptHistory}
				want = query.err
			case "canceled":
				cancel()
				want = context.Canceled
			case "cancel-during-read":
				page.Catalog.OriginatingArtifact = "exact.json"
				query.cancel = cancel
				want = context.Canceled
			}
			reader := NewArtifactHistoryReader(query, func(context.Context, string) (recordings.RecordingArtifactReference, error) { return "", fallbackErr })
			_, err := reader.ReadWorkerFactoryHistory(ctx, page)
			if scenario == "missing-legacy" {
				var typed *recordings.HistoricalRecordingQueryError
				if !errors.As(err, &typed) || typed.Kind != recordings.HistoricalRecordingQueryErrorMissingHistory {
					t.Fatalf("missing legacy = %v", err)
				}
			} else if !errors.Is(err, want) {
				t.Fatalf("error = %v; want %v", err, want)
			}
			if page.Catalog.OriginatingArtifact == "" && len(query.requests) != 0 {
				t.Fatal("unavailable legacy guessed a history artifact")
			}
		})
	}
}
