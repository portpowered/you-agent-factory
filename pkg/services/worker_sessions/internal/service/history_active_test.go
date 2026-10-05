package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func addActiveHistoryFixture(r *registry, id string, direct bool) {
	metadata := observationMetadata()
	metadata.direct = direct
	metadata.runtimeID = "runtime-1"
	if !direct {
		metadata.factorySessionID = "factory-1"
	}
	r.observations[id] = metadata
	r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateRunning}
	if direct {
		r.supervisions[id] = &supervision{accepted: true}
	} else {
		if r.runtimeAttemptControls == nil {
			r.runtimeAttemptControls = make(map[string]*runtimeAttempt)
		}
		r.runtimeAttemptControls[id] = &runtimeAttempt{completed: make(chan struct{})}
	}
}

func TestActiveHistoryRequiresAdmittedOwnersAndFiltersBeforePaging(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "direct-a", true)
	addActiveHistoryFixture(r, "factory-b", false)
	addActiveHistoryFixture(r, "orphan", true)
	delete(r.supervisions, "orphan")
	addActiveHistoryFixture(r, "unadmitted", true)
	r.supervisions["unadmitted"].accepted = false
	addActiveHistoryFixture(r, "terminal", true)
	r.sessions["terminal"] = workersessions.Session{ID: "terminal", State: workersessions.StateCompleted}
	addActiveHistoryFixture(r, "closed-owner", false)
	close(r.runtimeAttemptControls["closed-owner"].completed)
	for _, tc := range []struct {
		name    string
		request workersessions.ListWorkerSessionObservationsRequest
		want    []string
	}{
		{"both origins", workersessions.ListWorkerSessionObservationsRequest{}, []string{"direct-a", "factory-b"}},
		{"filtered before limit", workersessions.ListWorkerSessionObservationsRequest{Scope: workersessions.ObservationScopeFactory, MaxResults: 1}, []string{"factory-b"}},
		{"terminal filter", workersessions.ListWorkerSessionObservationsRequest{States: []workersessions.State{workersessions.StateCompleted}}, []string{}},
		{"other runtime", workersessions.ListWorkerSessionObservationsRequest{RuntimeID: "other"}, []string{}},
		{"factory scope", workersessions.ListWorkerSessionObservationsRequest{FactorySessionID: "factory-1"}, []string{"factory-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.request.History = workersessions.ObservationHistoryActive
			page, err := r.ListWorkerSessionObservations(t.Context(), tc.request)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(page.Observations))
			for _, observation := range page.Observations {
				ids = append(ids, observation.WorkerSessionID)
			}
			if !reflect.DeepEqual(ids, tc.want) || page.NextToken != "" {
				t.Fatalf("page = %v, token = %q; want %v without cursor", ids, page.NextToken, tc.want)
			}
		})
	}
}

func TestActiveHistoryFreezesMembershipFactsAndKeepsCompatibility(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "a", true)
	addActiveHistoryFixture(r, "b", false)
	request := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	first, err := r.ListWorkerSessionObservations(t.Context(), request)
	if err != nil || len(first.Observations) != 1 || first.NextToken == "" {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	first.Observations[0].WorkIDs[0] = "changed caller"
	addActiveHistoryFixture(r, "c", true)
	r.sessions["b"] = workersessions.Session{ID: "b", State: workersessions.StateCompleted}
	r.observations["b"].workIDs[0] = "changed live"
	request.NextToken = first.NextToken
	second, err := r.ListWorkerSessionObservations(t.Context(), request)
	if err != nil || len(second.Observations) != 1 || second.NextToken != "" {
		t.Fatalf("second page = %#v, %v", second, err)
	}
	if got := second.Observations[0]; got.WorkerSessionID != "b" || got.State != workersessions.StateRunning || got.WorkIDs[0] != "work-1" {
		t.Fatalf("frozen observation = %#v", got)
	}
	compatibility, err := r.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil || len(compatibility.Observations) != 3 || compatibility.Observations[1].State != workersessions.StateCompleted {
		t.Fatalf("omitted history = %#v, %v", compatibility, err)
	}
}

func TestActiveHistoryRejectsMismatchedProfileFiltersAndCanceledReads(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "a", true)
	addActiveHistoryFixture(r, "b", true)
	request := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	first, err := r.ListWorkerSessionObservations(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.NextToken = first.NextToken
	if _, err := r.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{NextToken: first.NextToken}); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("snapshot cursor on omitted history error = %v", err)
	}
	foreign := newObservationRegistry(nil, nil)
	if _, err := foreign.ListWorkerSessionObservations(t.Context(), request); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("foreign cursor error = %v", err)
	}
	request.Scope = workersessions.ObservationScopeDirect
	if _, err := r.ListWorkerSessionObservations(t.Context(), request); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("mismatched filter error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.ListWorkerSessionObservations(ctx, request); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestActiveHistoryCursorNormalizesEquivalentStateFilters(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil, nil)
	addActiveHistoryFixture(r, "a", true)
	addActiveHistoryFixture(r, "b", true)
	request := workersessions.ListWorkerSessionObservationsRequest{
		History: workersessions.ObservationHistoryActive, MaxResults: 1,
		States: []workersessions.State{workersessions.StatePaused, workersessions.StateRunning, workersessions.StateRunning},
	}
	first, err := r.ListWorkerSessionObservations(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.NextToken = first.NextToken
	request.Scope = workersessions.ObservationScopeAll
	request.States = []workersessions.State{workersessions.StateRunning, workersessions.StatePaused}
	request.MaxResults = 2
	second, err := r.ListWorkerSessionObservations(t.Context(), request)
	if err != nil || len(second.Observations) != 1 || second.Observations[0].WorkerSessionID != "b" {
		t.Fatalf("equivalent filters = %#v, %v", second, err)
	}
}
