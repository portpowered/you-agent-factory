package workerworkattribution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

type canonicalQueryFake struct {
	mu       sync.Mutex
	requests []recordings.HistoricalRecordingQueryRequest
	result   recordings.HistoricalRecordingQueryResult
	err      error
	cancel   context.CancelFunc
}

func (f *canonicalQueryFake) ReadHistoricalEvents(request recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
			}, nil)
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
			reader := NewArtifactHistoryReader(query, func(context.Context, string) (recordings.RecordingArtifactReference, error) { return "", fallbackErr }, nil)
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

func (f *canonicalQueryFake) DecodeHistoricalEvents(request recordings.HistoricalRecordingQueryRequest, _ []byte) (recordings.HistoricalRecordingQueryResult, error) {
	return f.ReadHistoricalEvents(request)
}

func TestRetainedNamesFreshnessAndRetry(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	captures := &captureFake{pages: map[string]recordings.WorkerCapturedActivityPage{"worker": page}}
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	payload := []byte("first")
	var readErr error
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) { return payload, readErr })
	service := New(captures, reader)
	request := []recordings.WorkerWorkAttributionRequest{{WorkerSessionID: "worker", FactorySessionID: "scope", WorkID: "work"}}
	check := func(name string, unavailable bool) {
		t.Helper()
		got, err := service.ResolveWorkerWorkAttribution(t.Context(), request)
		if err != nil || len(got) != 1 || got[0].WorkName != name || got[0].HistoryUnavailable != unavailable {
			t.Fatalf("names = %+v, %v", got, err)
		}
	}
	check("Alpha", false)
	check("Alpha", false)
	if len(query.requests) != 1 {
		t.Fatal("unchanged artifact decoded again")
	}
	// Equal byte length represents replacement that metadata-only caching misses.
	payload = []byte("other")
	query.result = namedHistory(t, "scope", "worker", "dispatch", "work", "Beta")
	check("Beta", false)
	if len(query.requests) != 2 {
		t.Fatal("replacement was not decoded")
	}
	readErr = os.ErrNotExist
	check("", true)
	readErr = nil
	check("Beta", false)
	if len(query.requests) != 2 {
		t.Fatal("temporary unavailability poisoned the projection")
	}
	payload = []byte("broken")
	query.err = recordings.ErrInvalidProjectionInput
	if _, err := service.ResolveWorkerWorkAttribution(t.Context(), request); !errors.Is(err, query.err) {
		t.Fatalf("corrupt replacement = %v", err)
	}
	query.err = nil
	query.result = namedHistory(t, "scope", "worker", "dispatch", "work", "Gamma")
	check("Gamma", false)
	if len(query.requests) != 4 {
		t.Fatal("failed projection was cached")
	}
}

func TestRetainedNamesBatchSharesArtifactAcrossAttemptGenerations(t *testing.T) {
	t.Parallel()
	captures := &captureFake{pages: make(map[string]recordings.WorkerCapturedActivityPage)}
	var requests []recordings.WorkerWorkAttributionRequest
	for _, id := range []string{"worker-a", "worker-b"} {
		page := capturePage(t, id, "scope", "recording", "dispatch-"+id, "work")
		page.Catalog.RecordingGenerationID = "generation-" + id
		page.Catalog.OriginatingArtifact = "exact.json"
		captures.pages[id] = page
		requests = append(requests, recordings.WorkerWorkAttributionRequest{WorkerSessionID: id, FactorySessionID: "scope", WorkID: "work"})
	}
	history := namedHistory(t, "scope", "worker-a", "dispatch-worker-a", "work", "Alpha")
	other := namedHistory(t, "scope", "worker-b", "dispatch-worker-b", "work", "Alpha")
	history.Events = append(history.Events, other.Events[1:]...)
	query := &canonicalQueryFake{result: history}
	reads := 0
	payload := []byte("first")
	var readErr error
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		reads++
		return payload, readErr
	})
	service := New(captures, reader)
	check := func(name string, unavailable bool) {
		t.Helper()
		got, err := service.ResolveWorkerWorkAttribution(t.Context(), requests)
		assertAttributionBatch(t, got, err, requests, name, unavailable)
	}
	check("Alpha", false)
	if reads != 1 || len(query.requests) != 1 {
		t.Fatalf("shared artifact read/decode counts = %d/%d", reads, len(query.requests))
	}
	payload = []byte("other") // Equal-length replacement must be seen on a fresh query.
	query.result.Events[0] = namedHistory(t, "scope", "worker-a", "dispatch-worker-a", "work", "Beta").Events[0]
	check("Beta", false)
	if reads != 2 || len(query.requests) != 2 {
		t.Fatalf("fresh artifact read/decode counts = %d/%d", reads, len(query.requests))
	}
	readErr = os.ErrNotExist
	check("", true)
	if reads != 3 {
		t.Fatalf("missing source read %d times; want one read per batch", reads)
	}
	readErr = nil
	check("Beta", false)
	if reads != 4 {
		t.Fatalf("restored source read count = %d; want fresh retry", reads)
	}
	// Sharing the projection must never skip validation of the second opening.
	page := captures.pages["worker-b"]
	page.Opening.Payload = []byte("broken")
	captures.pages["worker-b"] = page
	if _, err := service.ResolveWorkerWorkAttribution(t.Context(), requests); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("invalid sibling capture = %v", err)
	}
	page = capturePage(t, "worker-b", "scope", "recording", "conflicting-dispatch", "work")
	page.Catalog.RecordingGenerationID = "generation-worker-b"
	page.Catalog.OriginatingArtifact = "exact.json"
	captures.pages["worker-b"] = page
	if _, err := service.ResolveWorkerWorkAttribution(t.Context(), requests); !errors.Is(err, recordings.ErrInvalidProjectionInput) {
		t.Fatalf("conflicting sibling association = %v", err)
	}
}

func TestRetainedNamesLegacyBatchSharesOnlyResolvedSource(t *testing.T) {
	t.Parallel()
	captures := &captureFake{pages: make(map[string]recordings.WorkerCapturedActivityPage)}
	var requests []recordings.WorkerWorkAttributionRequest
	var history recordings.HistoricalRecordingQueryResult
	for _, id := range []string{"worker-a", "worker-b", "worker-c"} {
		page := capturePage(t, id, "scope", "recording", "dispatch-"+id, "work")
		page.Catalog.RecordingGenerationID = "generation-" + id
		page.Catalog.OriginatingArtifact = ""
		captures.pages[id] = page
		requests = append(requests, recordings.WorkerWorkAttributionRequest{WorkerSessionID: id, FactorySessionID: "scope", WorkID: "work"})
		other := namedHistory(t, "scope", id, "dispatch-"+id, "work", "Alpha")
		if len(history.Events) == 0 {
			history = other
		} else {
			history.Events = append(history.Events, other.Events[1:]...)
		}
	}
	query := &canonicalQueryFake{result: history}
	selected := 0
	var reads []string
	reader := NewArtifactHistoryReader(query, func(context.Context, string) (recordings.RecordingArtifactReference, error) {
		selected++
		if selected%3 == 0 {
			return "second.json", nil
		}
		return "first.json", nil
	}, func(path string) ([]byte, error) { reads = append(reads, path); return []byte(path), nil })
	service := New(captures, reader)
	for batch := range 2 {
		got, err := service.ResolveWorkerWorkAttribution(t.Context(), requests)
		assertAttributionBatch(t, got, err, requests, "Alpha", false)
		if len(reads) != (batch+1)*2 {
			t.Fatalf("batch %d source reads = %v", batch, reads)
		}
	}
	if selected != 6 || !reflect.DeepEqual(reads, []string{"first.json", "second.json", "first.json", "second.json"}) || len(query.requests) != 2 {
		t.Fatalf("legacy source selection/read/decode = %d/%v/%d", selected, reads, len(query.requests))
	}
	// Resolving a legacy candidate must not turn it into explicit provenance:
	// absence of its association remains optional/unavailable, never borrowed.
	query.result = namedHistory(t, "scope", "unrelated", "dispatch-unrelated", "work", "Shadow")
	reader.readFile = func(string) ([]byte, error) { return []byte("replacement"), nil }
	got, err := service.ResolveWorkerWorkAttribution(t.Context(), requests)
	assertAttributionBatch(t, got, err, requests, "", true)
}

func assertAttributionBatch(t *testing.T, got []recordings.WorkerWorkAttribution, err error, requests []recordings.WorkerWorkAttributionRequest, name string, unavailable bool) {
	t.Helper()
	if err != nil || len(got) != len(requests) {
		t.Fatalf("batch = %+v, %v", got, err)
	}
	for i, row := range got {
		if row.WorkerSessionID != requests[i].WorkerSessionID || row.WorkName != name || row.HistoryUnavailable != unavailable {
			t.Fatalf("row %d = %+v", i, row)
		}
	}
}

func TestRetainedNamesLegacySelectionAndScope(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = ""
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	artifact := recordings.RecordingArtifactReference("first.json")
	var selected []string
	reader := NewArtifactHistoryReader(query, func(context.Context, string) (recordings.RecordingArtifactReference, error) { return artifact, nil }, func(path string) ([]byte, error) { selected = append(selected, path); return []byte("same"), nil })
	if _, err := reader.readWorkerFactoryNames(t.Context(), page); err != nil {
		t.Fatal(err)
	}
	artifact = "second.json"
	query.result = namedHistory(t, "scope", "worker", "dispatch", "work", "Beta")
	got, err := reader.readWorkerFactoryNames(t.Context(), page)
	if err != nil || got.names["work"] != "Beta" || len(query.requests) != 2 || !reflect.DeepEqual(selected, []string{"first.json", "second.json"}) {
		t.Fatalf("legacy selection = %+v, %v, %v", got, err, selected)
	}
	page.Catalog.FactorySessionID = "foreign"
	if _, err := reader.readWorkerFactoryNames(t.Context(), page); !errors.Is(err, recordings.ErrInvalidProjectionScope) {
		t.Fatalf("foreign scope = %v", err)
	}
}

func TestRetainedNamesFreshGenerationsReuseValidatedSource(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	payload := []byte("first")
	var reads int
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		reads++
		return payload, nil
	})
	for _, generation := range []string{"first", "second", "third"} {
		page.Catalog.RecordingGenerationID = generation
		got, err := reader.readWorkerFactoryNames(t.Context(), page)
		if err != nil || got.names["work"] != "Alpha" {
			t.Fatalf("generation %s = %+v, %v", generation, got, err)
		}
	}
	if reads != 3 || len(query.requests) != 1 {
		t.Fatalf("fresh generations: reads=%d decodes=%d", reads, len(query.requests))
	}
	// Equal-length replacements still invalidate the shared compact projection.
	payload = []byte("other")
	query.result = namedHistory(t, "scope", "worker", "dispatch", "work", "Beta")
	got, err := reader.readWorkerFactoryNames(t.Context(), page)
	if err != nil || got.names["work"] != "Beta" || len(query.requests) != 2 {
		t.Fatalf("replacement = %+v, %v; decodes=%d", got, err, len(query.requests))
	}
	// Identical bytes in another recording still require independent validation.
	page.Catalog.RecordingID = "another-recording"
	query.err = recordings.ErrInvalidProjectionInput
	if _, err := reader.readWorkerFactoryNames(t.Context(), page); !errors.Is(err, query.err) || len(query.requests) != 3 {
		t.Fatalf("foreign recording = %v; decodes=%d", err, len(query.requests))
	}
}

func TestRetainedNamesEvictionPreservesRecentlyUsedSource(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "frequent.json"
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) { return []byte("same"), nil })
	read := func(candidate recordings.WorkerCapturedActivityPage) {
		t.Helper()
		got, err := reader.readWorkerFactoryNames(t.Context(), candidate)
		if err != nil || got.names["work"] != "Alpha" {
			t.Fatalf("names = %+v, %v", got, err)
		}
	}
	read(page)
	for index := range maxNameArtifacts + 1 {
		candidate := page
		candidate.Catalog.OriginatingArtifact = fmt.Sprintf("infrequent-%d.json", index)
		read(candidate)
		// Distinct attempts at the hot source do not consume cache capacity.
		page.Catalog.RecordingGenerationID = fmt.Sprintf("generation-%d", index)
		read(page)
	}
	if len(query.requests) != maxNameArtifacts+2 {
		t.Fatalf("hot source repeatedly decoded: %d calls", len(query.requests))
	}
	cold := page
	cold.Catalog.OriginatingArtifact = "infrequent-0.json"
	read(cold)
	if len(query.requests) != maxNameArtifacts+3 {
		t.Fatal("oldest cold source was not evicted")
	}
}

func TestRetainedNamesCanceledDecodeIsRetryable(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha"), cancel: cancel}
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) { return []byte("same"), nil })
	if _, err := reader.readWorkerFactoryNames(ctx, page); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	query.cancel = nil
	got, err := reader.readWorkerFactoryNames(t.Context(), page)
	if err != nil || got.names["work"] != "Alpha" || len(query.requests) != 2 {
		t.Fatalf("retry = %+v, %v", got, err)
	}
}

func TestRetainedNamesConcurrentReadsAndEviction(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) { return []byte("same"), nil })
	// Parallel consumers share only immutable projections. No lock spans artifact IO.
	var readers sync.WaitGroup
	for range 32 {
		readers.Go(func() {
			got, err := reader.readWorkerFactoryNames(t.Context(), page)
			if err != nil || got.names["work"] != "Alpha" {
				t.Errorf("concurrent names = %+v, %v", got, err)
			}
		})
	}
	readers.Wait()
	if len(query.requests) != 1 {
		t.Fatalf("unchanged concurrent artifact decoded %d times", len(query.requests))
	}
	for index := range maxNameArtifacts + 1 {
		candidate := page
		candidate.Catalog.OriginatingArtifact = fmt.Sprintf("artifact-%d.json", index)
		got, err := reader.readWorkerFactoryNames(t.Context(), candidate)
		if err != nil || got.names["work"] != "Alpha" {
			t.Fatalf("eviction list = %+v, %v", got, err)
		}
	}
	calls := len(query.requests)
	got, err := reader.readWorkerFactoryNames(t.Context(), page)
	if err != nil || got.names["work"] != "Alpha" || len(query.requests) != calls+1 {
		t.Fatalf("evicted artifact retry = %+v, %v", got, err)
	}
}

type gatedNamesQuery struct {
	canonicalQueryFake
	entered chan struct{}
	release chan struct{}
}

type observedWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (ctx *observedWaitContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func (f *gatedNamesQuery) DecodeHistoricalEvents(request recordings.HistoricalRecordingQueryRequest, payload []byte) (recordings.HistoricalRecordingQueryResult, error) {
	f.entered <- struct{}{}
	<-f.release
	return f.canonicalQueryFake.DecodeHistoricalEvents(request, payload)
}

func TestRetainedNamesWaiterCancellationDoesNotInterruptDecode(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &gatedNamesQuery{
		canonicalQueryFake: canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")},
		entered:            make(chan struct{}, 3), release: make(chan struct{}),
	}
	var release sync.Once
	unblock := func() { release.Do(func() { close(query.release) }) }
	t.Cleanup(unblock)
	read := make(chan struct{}, 3)
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		read <- struct{}{}
		return []byte("same"), nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waiter := &observedWaitContext{Context: ctx, waiting: make(chan struct{})}
	result := make(chan error, 3)
	invoke := func(ctx context.Context) {
		got, err := reader.readWorkerFactoryNames(ctx, page)
		if err == nil && got.names["work"] != "Alpha" {
			err = fmt.Errorf("name = %q", got.names["work"])
		}
		result <- err
	}
	go invoke(t.Context())
	<-query.entered
	<-read
	go invoke(waiter)
	<-read
	select {
	case <-waiter.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("waiter did not join the shared decode")
	}
	cancel()
	// Failure ceiling only: the leader remains gated until cancellation returns.
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled waiter waited for the unrelated decode")
	}
	go invoke(t.Context())
	<-read
	unblock()
	for range 2 {
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	}
	if len(query.requests) != 1 {
		t.Fatalf("shared decode count = %d", len(query.requests))
	}
}

func TestRetainedNamesSurvivesCanceledLeader(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &gatedNamesQuery{
		canonicalQueryFake: canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")},
		entered:            make(chan struct{}, 3), release: make(chan struct{}),
	}
	var release sync.Once
	unblock := func() { release.Do(func() { close(query.release) }) }
	t.Cleanup(unblock)
	read := make(chan struct{}, 3)
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		read <- struct{}{}
		return []byte("same"), nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	leader := make(chan error, 1)
	go func() {
		_, err := reader.readWorkerFactoryNames(ctx, page)
		leader <- err
	}()
	<-query.entered
	<-read
	survivor := make(chan error, 1)
	waiter := &observedWaitContext{Context: t.Context(), waiting: make(chan struct{})}
	go func() {
		got, err := reader.readWorkerFactoryNames(waiter, page)
		if err == nil && got.names["work"] != "Alpha" {
			err = fmt.Errorf("surviving name = %q", got.names["work"])
		}
		survivor <- err
	}()
	<-read
	select {
	case <-waiter.waiting:
	case <-time.After(5 * time.Second):
		t.Fatal("survivor did not join the shared decode")
	}
	cancel()
	unblock()
	if err := <-leader; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader cancellation = %v", err)
	}
	if err := <-survivor; err != nil {
		t.Fatal(err)
	}
	got, err := reader.readWorkerFactoryNames(t.Context(), page)
	if err != nil || got.names["work"] != "Alpha" || len(query.requests) != 2 {
		t.Fatalf("fresh result = %+v, %v; decode count = %d", got, err, len(query.requests))
	}
}

type namesResult struct {
	projection nameProjection
	err        error
}

func startNamesRead(ctx context.Context, reader *ArtifactHistoryReader, page recordings.WorkerCapturedActivityPage) <-chan namesResult {
	result := make(chan namesResult, 1)
	go func() {
		projection, err := reader.readWorkerFactoryNames(ctx, page)
		result <- namesResult{projection: projection, err: err}
	}()
	return result
}

func awaitNamesResult(t *testing.T, result <-chan namesResult, name string, want error) {
	t.Helper()
	select {
	case got := <-result:
		if !errors.Is(got.err, want) || (want == nil && got.projection.names["work"] != name) {
			t.Fatalf("names = %+v, %v; want %q, %v", got.projection, got.err, name, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("name read did not complete")
	}
}

func awaitNamesSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("name read did not reach the controlled edge")
	}
}

func TestRetainedNamesSharesArtifactReadWithIndependentCancellation(t *testing.T) {
	t.Parallel()
	for _, canceled := range []string{"waiter", "leader"} {
		t.Run(canceled, func(t *testing.T) {
			t.Parallel()
			page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
			page.Catalog.OriginatingArtifact = "exact.json"
			query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			t.Cleanup(unblock)
			var reads atomic.Int32
			reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
				if reads.Add(1) == 1 {
					close(entered)
					<-release
				}
				return []byte("same"), nil
			})
			leaderCtx, cancelLeader := context.WithCancel(t.Context())
			defer cancelLeader()
			waiterCtx, cancelWaiter := context.WithCancel(t.Context())
			defer cancelWaiter()
			leader := startNamesRead(leaderCtx, reader, page)
			awaitNamesSignal(t, entered)
			waitCtx := &observedWaitContext{Context: waiterCtx, waiting: make(chan struct{})}
			waiter := startNamesRead(waitCtx, reader, page)
			awaitNamesSignal(t, waitCtx.waiting)
			if reads.Load() != 1 {
				t.Fatal("concurrent waiter read the artifact again")
			}
			if canceled == "waiter" {
				cancelWaiter()
				awaitNamesResult(t, waiter, "", context.Canceled)
				unblock()
				awaitNamesResult(t, leader, "Alpha", nil)
			} else {
				cancelLeader()
				unblock()
				awaitNamesResult(t, leader, "", context.Canceled)
				awaitNamesResult(t, waiter, "Alpha", nil)
			}
			before := reads.Load()
			awaitNamesResult(t, startNamesRead(t.Context(), reader, page), "Alpha", nil)
			if reads.Load() != before+1 || len(query.requests) != 1 {
				t.Fatalf("fresh read/decode counts = %d/%d", reads.Load(), len(query.requests))
			}
		})
	}
}

func TestRetainedNamesSharedUnavailableReadRetries(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var reads atomic.Int32
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		if reads.Add(1) == 1 {
			close(entered)
			<-release
			return nil, os.ErrNotExist
		}
		return []byte("restored"), nil
	})
	leader := startNamesRead(t.Context(), reader, page)
	awaitNamesSignal(t, entered)
	waitCtx := &observedWaitContext{Context: t.Context(), waiting: make(chan struct{})}
	waiter := startNamesRead(waitCtx, reader, page)
	awaitNamesSignal(t, waitCtx.waiting)
	unblock()
	for _, result := range []<-chan namesResult{leader, waiter} {
		select {
		case got := <-result:
			if !isUnavailableHistory(got.err) || !errors.Is(got.err, os.ErrNotExist) {
				t.Fatalf("unavailable source = %v", got.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("shared unavailable read did not complete")
		}
	}
	if reads.Load() != 1 || len(query.requests) != 0 {
		t.Fatalf("unavailable read/decode counts = %d/%d", reads.Load(), len(query.requests))
	}
	awaitNamesResult(t, startNamesRead(t.Context(), reader, page), "Alpha", nil)
	if reads.Load() != 2 || len(query.requests) != 1 {
		t.Fatalf("restored read/decode counts = %d/%d", reads.Load(), len(query.requests))
	}
}

func TestRetainedNamesInFlightReadKeepsProvenanceSeparate(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &canonicalQueryFake{result: namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha")}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var reads atomic.Int32
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		if reads.Add(1) == 1 {
			close(entered)
			<-release
		}
		return []byte("same"), nil
	})
	leader := startNamesRead(t.Context(), reader, page)
	awaitNamesSignal(t, entered)
	for _, dimension := range []string{"factory", "recording", "generation", "artifact"} {
		candidate := page
		var want error
		switch dimension {
		case "factory":
			candidate.Catalog.FactorySessionID = "foreign"
			want = recordings.ErrInvalidProjectionScope
		case "recording":
			candidate.Catalog.RecordingID = "another-recording"
		case "generation":
			candidate.Catalog.RecordingGenerationID = "another-generation"
		case "artifact":
			candidate.Catalog.OriginatingArtifact = "another.json"
		}
		// Each different provenance completes while the original source is gated.
		awaitNamesResult(t, startNamesRead(t.Context(), reader, candidate), "Alpha", want)
	}
	if reads.Load() != 5 {
		t.Fatalf("distinct provenance reads = %d", reads.Load())
	}
	unblock()
	awaitNamesResult(t, leader, "Alpha", nil)
}

type replacedNamesQuery struct {
	canonicalQueryFake
	first, replacement recordings.HistoricalRecordingQueryResult
	entered, release   chan struct{}
}

func (query *replacedNamesQuery) DecodeHistoricalEvents(_ recordings.HistoricalRecordingQueryRequest, payload []byte) (recordings.HistoricalRecordingQueryResult, error) {
	if string(payload) == "first" {
		close(query.entered)
		<-query.release
		return query.first, nil
	}
	return query.replacement, nil
}

func TestRetainedNamesFreshReadDuringOlderDecodeObservesReplacement(t *testing.T) {
	t.Parallel()
	page := capturePage(t, "worker", "scope", "recording", "dispatch", "work")
	page.Catalog.OriginatingArtifact = "exact.json"
	query := &replacedNamesQuery{
		first:       namedHistory(t, "scope", "worker", "dispatch", "work", "Alpha"),
		replacement: namedHistory(t, "scope", "worker", "dispatch", "work", "Beta"),
		entered:     make(chan struct{}), release: make(chan struct{}),
	}
	var once sync.Once
	unblock := func() { once.Do(func() { close(query.release) }) }
	t.Cleanup(unblock)
	var reads atomic.Int32
	reader := NewArtifactHistoryReader(query, nil, func(string) ([]byte, error) {
		if reads.Add(1) == 1 {
			return []byte("first"), nil
		}
		return []byte("other"), nil
	})
	older := startNamesRead(t.Context(), reader, page)
	awaitNamesSignal(t, query.entered)
	// The old decode stays gated; a new call must read the replacement instead
	// of joining a completed read whose bytes no longer represent the source.
	awaitNamesResult(t, startNamesRead(t.Context(), reader, page), "Beta", nil)
	unblock()
	awaitNamesResult(t, older, "Alpha", nil)
	awaitNamesResult(t, startNamesRead(t.Context(), reader, page), "Beta", nil)
	if reads.Load() != 3 {
		t.Fatalf("fresh source reads = %d", reads.Load())
	}
}
