package workerworkattribution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
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
