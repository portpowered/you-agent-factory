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

func TestWorkerWorkAttributionScopedNamesAndBatching(t *testing.T) {
	t.Parallel()
	first := capturePage(t, "worker-a", "scope-a", "recording-a", "dispatch-a", "reused")
	second := capturePage(t, "worker-b", "scope-b", "recording-b", "dispatch-b", "reused")
	captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker-a": first, "worker-b": second}}
	history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{
		"recording-a": namedHistory(t, "scope-a", "worker-a", "dispatch-a", "reused", "Alpha"),
		"recording-b": namedHistory(t, "scope-b", "worker-b", "dispatch-b", "reused", "Beta"),
	}}
	requests := []recordings.WorkerWorkAttributionRequest{
		{WorkerSessionID: "worker-a", FactorySessionID: "scope-a", WorkID: "reused"}, {WorkerSessionID: "worker-b", FactorySessionID: "scope-b", WorkID: "reused"}, {WorkerSessionID: "worker-a", FactorySessionID: "scope-a", WorkID: "reused"},
	}
	got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), requests)
	want := []recordings.WorkerWorkAttribution{
		{WorkerSessionID: "worker-a", FactorySessionID: "scope-a", WorkID: "reused", WorkName: "Alpha"}, {WorkerSessionID: "worker-b", FactorySessionID: "scope-b", WorkID: "reused", WorkName: "Beta"}, {WorkerSessionID: "worker-a", FactorySessionID: "scope-a", WorkID: "reused", WorkName: "Alpha"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped attribution = %+v, %v; want %+v", got, err, want)
	}
	if captures.calls != 2 || history.calls != 2 {
		t.Fatalf("duplicate request repeated reads: captures=%d history=%d", captures.calls, history.calls)
	}
}

func TestWorkerWorkAttributionSharesRecordingProjection(t *testing.T) {
	t.Parallel()
	h := namedHistory(t, "scope", "worker-a", "dispatch-a", "work", "Name")
	other := namedHistory(t, "scope", "worker-b", "dispatch-b", "work", "Name")
	h.Events = append(h.Events, other.Events[1:]...)
	captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{
		"worker-a": capturePage(t, "worker-a", "scope", "recording", "dispatch-a", "work"),
		"worker-b": capturePage(t, "worker-b", "scope", "recording", "dispatch-b", "work"),
	}}
	// Production generations include the Worker identity even when both
	// attempts share the same originating Factory recording.
	for id, page := range captures.pages {
		page.Catalog.RecordingGenerationID = "generation-" + id
		captures.pages[id] = page
	}
	history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": h}}
	got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{
		{WorkerSessionID: "worker-a", FactorySessionID: "scope", WorkID: "work"}, {WorkerSessionID: "worker-b", FactorySessionID: "scope", WorkID: "work"},
	})
	if err != nil || len(got) != 2 || got[0].WorkName != "Name" || got[1].WorkName != "Name" || history.calls != 1 {
		t.Fatalf("shared projection = %+v, %v, reads=%d", got, err, history.calls)
	}
}

func TestWorkerWorkAttributionMissingFactsAndLegacy(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"nameless", "unassociated", "direct", "scopeless", "uncaptured", "legacy", "explicit-id-name"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			h := namedHistory(t, "scope", "worker", "dispatch", "work", "")
			request := recordings.WorkerWorkAttributionRequest{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}
			want := ""
			switch scenario {
			case "unassociated":
				h.Events = h.Events[:2]
			case "direct":
				request.WorkID = ""
			case "scopeless":
				request.FactorySessionID = ""
			case "uncaptured":
				page = recordings.WorkerCapturedActivityPage{}
			case "legacy":
				h.Events[0].Payload = string(marshal(t, factorydefinitions.WorkRequestPayload{WorkItems: []work.FactoryWorkItem{{ID: "work", DisplayName: "Legacy"}}}))
				want = "Legacy"
			case "explicit-id-name":
				h = namedHistory(t, "scope", "worker", "dispatch", "work", "work")
				want = "work" // An explicitly authored ID-shaped name is still a name.
			}
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
			if scenario == "uncaptured" {
				delete(captures.pages, "worker")
			}
			history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": h}}
			got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{request})
			if err != nil || len(got) != 1 || got[0].WorkName != want {
				t.Fatalf("optional name = %+v, %v; want %q", got, err, want)
			}
			if history.calls != expectedHistoryReads(scenario) {
				t.Fatal("unassociated capture consulted Factory history")
			}
		})
	}
}

func TestWorkerWorkAttributionRejectsForeignAndAmbiguousFacts(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"catalog-scope", "opening-work", "opening-worker", "malformed-opening", "history-scope", "event-scope", "association", "dispatch-work", "ambiguous-worker", "ambiguous-name", "malformed-name"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			h := namedHistory(t, "scope", "worker", "dispatch", "work", "Name")
			wantErr := recordings.ErrInvalidProjectionInput
			switch scenario {
			case "catalog-scope":
				page.Catalog.FactorySessionID = "foreign"
				wantErr = recordings.ErrWorkerRecordingReplay
			case "opening-work", "opening-worker":
				page = capturePage(t, "worker", "scope", "recording", "dispatch", "foreign-work")
				if scenario == "opening-worker" {
					page = capturePage(t, "foreign-worker", "scope", "recording", "dispatch", "work")
				}
				wantErr = recordings.ErrWorkerRecordingReplay
			case "malformed-opening":
				page.Opening.Payload = json.RawMessage("{")
				wantErr = recordings.ErrWorkerRecordingReplay
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
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
			history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": h}}
			got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}})
			if !errors.Is(err, wantErr) || got != nil {
				t.Fatalf("unsafe attribution = %+v, %v; want %v", got, err, wantErr)
			}
		})
	}
}

func TestWorkerWorkAttributionErrorsAndCancellation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"capture-corrupt", "history-corrupt", "canceled", "cancel-during-history", "invalid-request", "nil-context"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
			history := &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": namedHistory(t, "scope", "worker", "dispatch", "work", "Name")}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			requests := []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}}
			var want error
			switch scenario {
			case "capture-corrupt":
				want = recordings.ErrWorkerRecordingReplay
				captures.err = want
			case "history-corrupt":
				want = &recordings.HistoricalRecordingQueryError{Kind: recordings.HistoricalRecordingQueryErrorCorruptHistory, RecordingID: "recording"}
				history.err = want
			case "canceled":
				cancel()
				want = context.Canceled
			case "cancel-during-history":
				history.cancel = cancel
				want = context.Canceled
			case "invalid-request":
				requests[0].WorkerSessionID = " "
				want = recordings.ErrInvalidWorkerRecordingRequest
			case "nil-context":
				ctx = nil
				want = recordings.ErrInvalidProjectionInput
			}
			got, err := New(captures, history).ResolveWorkerWorkAttribution(ctx, requests)
			if !errors.Is(err, want) || got != nil {
				t.Fatalf("error attribution = %+v, %v; want %v", got, err, want)
			}
		})
	}
}

func TestWorkerWorkAttributionUnavailableHistory(t *testing.T) {
	t.Parallel()
	for _, kind := range []recordings.HistoricalRecordingQueryErrorKind{recordings.HistoricalRecordingQueryErrorUnavailable, recordings.HistoricalRecordingQueryErrorMissingHistory} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": capturePage(t, "worker", "scope", "recording", "dispatch", "work")}}
			history := &historyFake{err: &recordings.HistoricalRecordingQueryError{Kind: kind, RecordingID: "recording"}}
			got, err := New(captures, history).ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}})
			want := []recordings.WorkerWorkAttribution{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work", HistoryUnavailable: true}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("unavailable optional attribution = %+v, %v; want %+v", got, err, want)
			}
		})
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

func expectedHistoryReads(scenario string) int {
	switch scenario {
	case "direct", "scopeless", "uncaptured":
		return 0
	default:
		return 1
	}
}

func TestWorkerWorkAttributionReportedDefaultRequiresExactAssociation(t *testing.T) {
	t.Parallel()
	const canonical = "6e98a017-c0a5-4b68-b6cf-3a77b46270fd"
	for _, scenario := range []string{"exact", "foreign-scope", "missing-association", "foreign-dispatch"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", canonical, "recording", "dispatch", "work")
			history := namedHistory(t, "~default", "worker", "dispatch", "work", "Recorded")
			var want error
			switch scenario {
			case "foreign-scope":
				history = namedHistory(t, "sibling", "worker", "dispatch", "work", "Foreign")
				want = recordings.ErrInvalidProjectionScope
			case "missing-association":
				history.Events = history.Events[:2]
			case "foreign-dispatch":
				history = namedHistory(t, "~default", "worker", "foreign-dispatch", "work", "Foreign")
				want = recordings.ErrInvalidProjectionInput
			}
			reader := New(&captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}, &historyFake{histories: map[string]recordings.HistoricalRecordingQueryResult{"recording": history}})
			got, err := reader.ResolveWorkerWorkAttribution(t.Context(), []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: canonical, WorkID: "work"}})
			if scenario == "missing-association" {
				if err != nil || len(got) != 1 || got[0].WorkName != "" || !got[0].HistoryUnavailable {
					t.Fatalf("unvalidated alias = %+v, %v", got, err)
				}
				return
			}
			if !errors.Is(err, want) {
				t.Fatalf("alias error = %v; want %v", err, want)
			}
			if scenario == "exact" && (len(got) != 1 || got[0].FactorySessionID != canonical || got[0].WorkName != "Recorded") {
				t.Fatalf("canonical attribution = %+v", got)
			}
		})
	}
}
