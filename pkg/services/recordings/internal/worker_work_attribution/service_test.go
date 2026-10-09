package workerworkattribution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type captureFake struct {
	pages map[string]recordings.WorkerCapturedActivityPage
	err   error
	calls int
}

func (f *captureFake) ReadWorkerCapturedActivity(_ context.Context, request recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	f.calls++
	if f.err != nil {
		return recordings.WorkerCapturedActivityPage{}, f.err
	}
	page, exists := f.pages[request.WorkerSessionID]
	if !exists {
		return page, os.ErrNotExist
	}
	return page, nil
}

type historyFake struct {
	histories map[string]recordings.HistoricalRecordingQueryResult
	err       error
	calls     int
	cancel    context.CancelFunc
}

type preparationCatalog struct {
	captureFake
	pages    []recordings.WorkerCapturedCatalogPage
	requests []recordings.WorkerCapturedCatalogRequest
	err      error
}

func (f *preparationCatalog) ListWorkerSessionCaptures(_ context.Context, request recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return recordings.WorkerCapturedCatalogPage{}, f.err
	}
	page := f.pages[0]
	f.pages = f.pages[1:]
	return page, nil
}

func TestPrepareRetainedNamesUsesCommittedCatalogAcrossPages(t *testing.T) {
	t.Parallel()
	first := capturePage(t, "worker-a", "scope", "recording", "dispatch", "work")
	second := capturePage(t, "worker-b", "scope", "recording", "dispatch", "work")
	first.Catalog.OriginatingArtifact, second.Catalog.OriginatingArtifact = "exact.json", "exact.json"
	catalog := &preparationCatalog{pages: []recordings.WorkerCapturedCatalogPage{
		{Items: []recordings.WorkerCapturedCatalogItem{{Catalog: first.Catalog, Opening: first.Opening}}, NextToken: "next"},
		{Items: []recordings.WorkerCapturedCatalogItem{{Catalog: second.Catalog, Opening: second.Opening}}},
	}}
	history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": namedHistory(t, "scope", "worker-a", "dispatch", "work", "Alpha")}}
	if err := New(catalog, history).PrepareWorkerWorkAttribution(t.Context()); err != nil {
		t.Fatal(err)
	}
	if catalog.calls != 0 || history.calls != 1 || len(catalog.requests) != 2 || catalog.requests[1].NextToken != "next" {
		t.Fatalf("capture reads=%d, history reads=%d, pages=%+v", catalog.calls, history.calls, catalog.requests)
	}
}

func TestPrepareRetainedNamesUnavailableAndCanceledRemainRetryable(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	item := recordings.WorkerCapturedCatalogItem{Catalog: page.Catalog, Opening: page.Opening}
	catalog := &preparationCatalog{pages: []recordings.WorkerCapturedCatalogPage{{Items: []recordings.WorkerCapturedCatalogItem{item}}}}
	history := &historyFake{err: &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorUnavailable}}
	service := New(catalog, history)
	if err := service.PrepareWorkerWorkAttribution(t.Context()); err != nil {
		t.Fatalf("optional history blocked startup: %v", err)
	}
	history.err = nil
	history.histories = map[string]recordings.HistoricalRecordingQueryResult{"recording": namedHistory(t, "scope", "worker", "dispatch", "work", "Restored")}
	catalog.pages = []recordings.WorkerCapturedCatalogPage{{Items: []recordings.WorkerCapturedCatalogItem{item}}}
	if err := service.PrepareWorkerWorkAttribution(t.Context()); err != nil || history.calls != 2 {
		t.Fatalf("restored source was not retried: %v, reads=%d", err, history.calls)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	history.cancel = cancel
	catalog.pages = []recordings.WorkerCapturedCatalogPage{{Items: []recordings.WorkerCapturedCatalogItem{item}}}
	if err := service.PrepareWorkerWorkAttribution(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("activation cancellation=%v", err)
	}
	history.cancel = nil
	catalog.pages = []recordings.WorkerCapturedCatalogPage{{Items: []recordings.WorkerCapturedCatalogItem{item}}}
	if err := service.PrepareWorkerWorkAttribution(t.Context()); err != nil || history.calls != 4 {
		t.Fatalf("canceled preparation poisoned retry: %v, reads=%d", err, history.calls)
	}
}

func (f *historyFake) ReadWorkerFactoryHistory(_ context.Context, page recordings.WorkerCapturedActivityPage) (recordings.HistoricalRecordingQueryResult, error) {
	f.calls++
	if f.cancel != nil {
		f.cancel()
	}
	return f.histories[page.Catalog.RecordingID], f.err
}

func TestWorkerWorkAttributionMaterializedNames(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"captured", "prepared", "cold", "changed-generation", "changed-scope"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			page.Catalog.RecordingGenerationID = "generation"
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
			history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": namedHistory(t, "scope", "worker", "dispatch", "work", "Prepared")}}
			service := New(captures, history)
			want := ""
			if mode != "cold" && mode != "captured" {
				query := attributionQuery{service: service, projections: make(map[historyIdentity]projectionResult)}
				query.prepareCapturedNames(t.Context(), recordings.WorkerCapturedCatalogItem{Catalog: page.Catalog, Opening: page.Opening})
				want = "Prepared"
			}
			if mode == "captured" {
				page.Catalog.WorkName = "Captured"
				want = "Captured"
			}
			if mode == "changed-generation" {
				page.Catalog.RecordingGenerationID = "other"
				want = ""
			}
			if mode == "changed-scope" {
				page = capturePage(t, "worker", "other", "recording", "dispatch", "work")
				want = ""
			}
			captures.pages["worker"] = page
			history.calls = 0
			history.err = errors.New("Factory recording reads forbidden")
			request := recordings.WorkerWorkAttributionRequest{WorkerSessionID: "worker", FactorySessionID: page.Catalog.FactorySessionID, WorkID: "work"}
			for range 2 {
				got, err := service.ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{request, request})
				if err != nil || len(got) != 2 || got[0].WorkName != want || got[1].WorkName != want || got[0].HistoryUnavailable || history.calls != 0 {
					t.Fatalf("materialized attribution=%+v err=%v history reads=%d", got, err, history.calls)
				}
			}
		})
	}
}

func TestWorkerWorkAttributionRejectsInvalidCapture(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"scope", "work", "worker", "malformed", "position"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			page.Catalog.WorkName = "Captured"
			switch mode {
			case "scope":
				page.Catalog.FactorySessionID = "foreign"
			case "work":
				page = capturePage(t, "worker", "scope", "recording", "dispatch", "foreign")
			case "worker":
				page.Catalog.WorkerSessionID = "foreign"
			case "malformed":
				page.Opening.Payload = json.RawMessage("{")
			case "position":
				page.Opening.ID.Position = 2
			}
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
			history := &historyFake{err: errors.New("forbidden")}
			got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}})
			if !errors.Is(err, recordings.ErrWorkerRecordingReplay) || got != nil || history.calls != 0 {
				t.Fatalf("invalid capture=%+v %v reads=%d", got, err, history.calls)
			}
		})
	}
}

func TestWorkerWorkAttributionOptionalAbsenceAndCancellation(t *testing.T) {
	t.Parallel()
	captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{}}
	service := New(captures, nil)
	requests := []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "unknown", FactorySessionID: "scope", WorkID: "work"}, {WorkerSessionID: "direct"}, {WorkerSessionID: "scopeless", WorkID: "work"}}
	got, err := service.ResolveWorkerWorkAttribution(t.Context(), requests)
	want := []recordings.WorkerWorkAttribution{{WorkerSessionID: "unknown", FactorySessionID: "scope", WorkID: "work"}, {WorkerSessionID: "direct"}, {WorkerSessionID: "scopeless", WorkID: "work"}}
	if err != nil || !reflect.DeepEqual(got, want) || captures.calls != 1 {
		t.Fatalf("absence=%+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = service.ResolveWorkerWorkAttribution(ctx, requests); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// A missing context is invalid input; retain this rejection witness.
	var missingContext context.Context
	if _, err = service.ResolveWorkerWorkAttribution(missingContext, requests); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatal(err)
	}
	if _, err = service.ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: " "}}); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
		t.Fatal(err)
	}
	captures.err = recordings.ErrWorkerRecordingReplay
	if _, err = service.ResolveWorkerWorkAttribution(t.Context(), requests); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatal(err)
	}
}

func capturePage(t *testing.T, worker, factory, recording, dispatch, id string) recordings.WorkerCapturedActivityPage {
	t.Helper()
	payload := workers.SessionPayload{WorkerSessionID: worker, FactorySessionID: factory, RecordingID: recording, DispatchID: dispatch, WorkIDs: []string{id}}
	return recordings.WorkerCapturedActivityPage{
		Catalog: recordings.WorkerSessionCatalogEntry{WorkerSessionID: worker, FactorySessionID: factory, RecordingID: recording, OriginatingArtifact: "original.json"},
		Opening: events.Record{ID: events.RecordID{Position: 1}, Payload: marshal(t, workers.Draft{Kind: workers.KindSession, Phase: workers.PhaseStarted, Payload: marshal(t, payload)})},
	}
}

func namedHistory(t *testing.T, factory, worker, dispatch, id, name string) recordings.HistoricalRecordingQueryResult {
	t.Helper()
	ids := []string{id}
	scope := recordings.CanonicalEventScope{FactorySessionID: factory}
	return recordings.HistoricalRecordingQueryResult{
		Recording: recordings.HistoricalRecordingIdentity{Scope: scope},
		Events: []recordings.CanonicalEvent{
			{Scope: scope, Kind: recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeWorkRequest), SourceContext: string(marshal(t, factorydefinitions.FactoryEventContext{WorkIDs: &ids})), Payload: string(marshal(t, work.WorkRequestEventPayload{Works: []work.WorkRequestEventWork{{Name: name}}}))},
			{Scope: scope, Kind: recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchRequest), SourceContext: string(marshal(t, factorydefinitions.FactoryEventContext{DispatchID: &dispatch, WorkIDs: &ids})), Payload: "{}"},
			{Scope: scope, Kind: recordings.CanonicalEventKind(factorydefinitions.FactoryEventTypeDispatchWorkerSessionAssoc), SourceContext: string(marshal(t, factorydefinitions.FactoryEventContext{DispatchID: &dispatch})), Payload: string(marshal(t, factorydefinitions.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: worker}))},
		},
	}
}

func marshal(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestStartupNameProjectionRejectsForeignAndAmbiguousFacts(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"history-scope", "event-scope", "association", "dispatch-work", "ambiguous-worker", "ambiguous-name", "malformed-name"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := namedHistory(t, "scope", "worker", "dispatch", "work", "Name")
			wantErr := recordings.ErrInvalidProjectionInput
			switch scenario {
			case "history-scope":
				h.Recording.Scope.FactorySessionID = "foreign"
				wantErr = recordings.ErrInvalidProjectionScope
			case "event-scope":
				h.Events[0].Scope.FactorySessionID = "foreign"
				wantErr = recordings.ErrInvalidProjectionScope
			case "association":
				h = namedHistory(t, "scope", "worker", "foreign-dispatch", "work", "Name")
			case "dispatch-work":
				h = namedHistory(t, "scope", "worker", "dispatch", "foreign-work", "Foreign")
			case "ambiguous-worker":
				other := namedHistory(t, "scope", "worker", "other-dispatch", "work", "Name")
				h.Events = append(h.Events, other.Events[1:]...)
			case "ambiguous-name":
				h.Events[0].SourceContext = "{}"
				h.Events[0].Payload = string(marshal(t, work.WorkRequestEventPayload{Works: []work.WorkRequestEventWork{{WorkID: "work", Name: "First"}, {WorkID: "work", Name: "Second"}}}))
			case "malformed-name":
				h.Events[0].Payload = "{"
			}
			projection, err := scopedNames(h, "scope")
			if err == nil {
				association := projection.associations["worker"]
				if association.dispatch != "dispatch" || !containsWork(association.workIDs, "work") {
					err = recordings.ErrInvalidProjectionInput
				}
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("unsafe startup projection error=%v want=%v", err, wantErr)
			}

		})
	}
}

func TestStartupNameProjectionLegacyAbsenceAndAlias(t *testing.T) {
	t.Parallel()
	const canonical = "6e98a017-c0a5-4b68-b6cf-3a77b46270fd"
	for _, scenario := range []string{"legacy", "id-name", "unassociated", "alias"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			h := namedHistory(t, "scope", "worker", "dispatch", "work", "Name")
			scope, want := "scope", "Name"
			switch scenario {
			case "legacy":
				h.Events[0].Payload = string(marshal(t, factorydefinitions.WorkRequestPayload{WorkItems: []work.FactoryWorkItem{{ID: "work", DisplayName: "Legacy"}}}))
				want = "Legacy"
			case "id-name":
				h = namedHistory(t, "scope", "worker", "dispatch", "work", "work")
				want = "work"
			case "unassociated":
				h.Events = h.Events[:2]
			case "alias":
				h = namedHistory(t, "~default", "worker", "dispatch", "work", "Name")
				scope = canonical
			}
			projection, err := scopedNames(h, scope)
			if err != nil || projection.names["work"] != want {
				t.Fatalf("startup names=%+v %v", projection, err)
			}
			if scenario == "unassociated" && len(projection.associations) != 0 {
				t.Fatalf("invented association=%+v", projection)
			}
		})
	}
}
