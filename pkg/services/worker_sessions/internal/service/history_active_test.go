package service

import (
	"context"
	"errors"
	"reflect"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
	r := newObservationRegistry(nil)
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
	r := newObservationRegistry(nil)
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
	r := newObservationRegistry(nil)
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
	foreign := newObservationRegistry(nil)
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
	r := newObservationRegistry(nil)
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

func TestFleetHistorySamplesAllOwnersBeforeSharedArchive(t *testing.T) {
	t.Parallel()
	direct := fleetHistoryOwnerFixture(fleetObservation("a-live", true, workersessions.StateRunning))
	factory := fleetHistoryOwnerFixture(fleetObservation("b-live", false, workersessions.StateRunning))
	logs := fleetHistoryLogsFake{read: func(_ context.Context, req workersessions.ListWorkerSessionObservationsRequest, owners map[historyIdentity]struct{}) ([]workersessions.Observation, error) {
		if len(owners) != 2 {
			t.Fatalf("archive exclusions = %+v", owners)
		}
		archived := []workersessions.Observation{
			{WorkerSessionID: "c-ended", FactorySessionID: "factory-old", AttemptID: "attempt-ended", State: workersessions.StateCompleted},
			{WorkerSessionID: "d-lost", AttemptID: "attempt-lost", State: workersessions.StateFailed, Failure: &workersessions.FailureCause{Kind: workersessions.FailureCauseProcessGone}, RecordingHealth: recordings.WorkerRecordingStatusIncomplete},
		}
		rows := make([]workersessions.Observation, 0)
		for _, row := range archived {
			if observationStateMatches(row.State, req.States) {
				rows = append(rows, row)
			}
		}
		return rows, nil
	}}
	clock := platformclock.NewDeterministic(time.Now(), time.Millisecond)
	catalogCalls := 0
	query := NewFleetHistory(func(context.Context) ([]workersessions.Service, error) {
		catalogCalls++
		return []workersessions.Service{factory, direct, factory}, nil
	}, logs, clock, logging.NoopLogger{}, newTestHistoryBudget(), nil)
	for _, tc := range []struct {
		history workersessions.ObservationHistory
		states  []workersessions.State
		ids     []string
	}{
		{workersessions.ObservationHistoryActive, nil, []string{"a-live", "b-live"}},
		{workersessions.ObservationHistoryAll, nil, []string{"a-live", "b-live", "c-ended", "d-lost"}},
		{workersessions.ObservationHistoryArchived, nil, []string{"c-ended", "d-lost"}},
		{workersessions.ObservationHistoryArchived, []workersessions.State{workersessions.StateRunning}, []string{}},
		{workersessions.ObservationHistoryActive, []workersessions.State{workersessions.StateCompleted}, []string{}},
	} {
		req := workersessions.ListWorkerSessionObservationsRequest{History: tc.history, States: tc.states, MaxResults: 1}
		callsBefore := catalogCalls
		ids := make([]string, 0)
		for {
			page, err := query.ListWorkerSessionObservations(t.Context(), req)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range page.Observations {
				ids = append(ids, row.WorkerSessionID)
				if row.WorkerSessionID == "d-lost" && (row.State != workersessions.StateFailed || row.Failure.Kind != workersessions.FailureCauseProcessGone || row.RecordingHealth != recordings.WorkerRecordingStatusIncomplete) {
					t.Fatalf("lost row = %+v", row)
				}
			}
			if page.NextToken == "" {
				break
			}
			req.NextToken = page.NextToken
		}
		if !reflect.DeepEqual(ids, tc.ids) || catalogCalls != callsBefore+1 {
			t.Fatalf("%s: ids=%v calls=%d; want %v and one catalog sample", tc.history, ids, catalogCalls-callsBefore, tc.ids)
		}
	}
}

func TestFleetHistoryFreezesMembershipAndRejectsForeignCursors(t *testing.T) {
	t.Parallel()
	live := []workersessions.Observation{fleetObservation("a", true, workersessions.StateRunning), fleetObservation("b", false, workersessions.StateRunning)}
	live[1].WorkIDs = []string{"work-1"}
	source := fleetHistoryPageFake{read: func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
		return workersessions.ListWorkerSessionObservationsResult{Observations: live}, nil
	}}
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	calls := 0
	catalog := func(context.Context) ([]workersessions.Service, error) {
		calls++
		return []workersessions.Service{source}, nil
	}
	query := NewFleetHistory(catalog, nil, clock, logging.NoopLogger{}, newTestHistoryBudget(), nil)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	first, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || first.NextToken == "" {
		t.Fatalf("first=%+v, %v", first, err)
	}
	live = append(live, fleetObservation("c", true, workersessions.StateRunning))
	live[1].State = workersessions.StateCompleted
	live[1].WorkIDs[0] = "mutated"
	req.NextToken = first.NextToken
	second, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(second.Observations) != 1 || second.Observations[0].WorkerSessionID != "b" || second.Observations[0].State != workersessions.StateRunning || second.Observations[0].WorkIDs[0] != "work-1" || second.NextToken != "" || calls != 1 {
		t.Fatalf("frozen=%+v, %v calls=%d", second, err, calls)
	}
	foreign := NewFleetHistory(catalog, nil, clock, logging.NoopLogger{}, newTestHistoryBudget(), nil)
	if _, err := foreign.ListWorkerSessionObservations(t.Context(), req); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("foreign token: %v", err)
	}
	req.History = workersessions.ObservationHistoryArchived
	if _, err := query.ListWorkerSessionObservations(t.Context(), req); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("cross-history token: %v", err)
	}
}

func TestFleetHistoryKeepsScopedIdentityAndPropagatesUnavailable(t *testing.T) {
	t.Parallel()
	firstRow := fleetObservation("shared-id", false, workersessions.StateRunning)
	secondRow := firstRow.Clone()
	secondRow.FactorySessionID = "factory-2"
	first, second := fleetHistoryOwnerFixture(firstRow), fleetHistoryOwnerFixture(secondRow)
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	query := NewFleetHistory(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{first, second}, nil
	}, nil, clock, logging.NoopLogger{}, newTestHistoryBudget(), nil)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	one, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(one.Observations) != 1 || one.NextToken == "" {
		t.Fatalf("first scoped page=%+v, %v", one, err)
	}
	req.NextToken = one.NextToken
	two, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(two.Observations) != 1 || two.Observations[0].FactorySessionID == one.Observations[0].FactorySessionID {
		t.Fatalf("second scoped page=%+v, %v", two, err)
	}
	req.NextToken, req.History = "", workersessions.ObservationHistoryArchived
	if _, err := query.ListWorkerSessionObservations(t.Context(), req); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("missing capture reader: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := query.ListWorkerSessionObservations(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

// Only the list port is exercised by the fleet projector; all other capabilities
// remain owned by the source registry in production.
type fleetHistoryPageFake struct {
	workersessions.Service
	read func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error)
}

func TestFleetHistoryOwnerSamplingPreservesRegistryCursor(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil)
	addActiveHistoryFixture(r, "a", true)
	addActiveHistoryFixture(r, "b", true)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	first, err := r.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || first.NextToken == "" {
		t.Fatalf("first registry page=%+v, %v", first, err)
	}
	cursor, err := decodeHistoryCursor(first.NextToken)
	if err != nil {
		t.Fatal(err)
	}
	// Charge near-capacity retained storage without a load-sized unit fixture.
	// Another intermediate snapshot would evict this customer cursor.
	r.historySnapshots.entries[cursor.ID].bytes = historySnapshotBytes
	r.historySnapshots.bytes = historySnapshotBytes
	owners, err := readFleetHistoryOwners(t.Context(), r, req)
	if err != nil || len(owners) != 2 {
		t.Fatalf("sampled owners=%+v, %v", owners, err)
	}
	owners[1].WorkIDs[0] = "caller mutation"
	fresh, err := readFleetHistoryOwners(t.Context(), r, req)
	if err != nil || len(fresh) != 2 || fresh[1].WorkIDs[0] != "work-1" {
		t.Fatalf("detached sampling=%+v, %v", fresh, err)
	}
	req.NextToken = first.NextToken
	replay, err := r.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(replay.Observations) != 1 || replay.Observations[0].WorkerSessionID != "b" || replay.Observations[0].WorkIDs[0] != "work-1" {
		t.Fatalf("registry cursor after sampling=%+v, %v", replay, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readFleetHistoryOwners(ctx, r, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled sampling=%v", err)
	}
}

func (s fleetHistoryPageFake) ListWorkerSessionObservations(_ context.Context, req workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return s.read(req)
}

func TestFleetHistoryUsesScopedSnapshotInsteadOfCompatibilityCursor(t *testing.T) {
	t.Parallel()
	rows := []workersessions.Observation{
		{WorkerSessionID: "same-id", FactorySessionID: "factory-a", AttemptID: "attempt-a", State: workersessions.StateRunning, DurationBasis: workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable},
		{WorkerSessionID: "same-id", FactorySessionID: "factory-b", AttemptID: "attempt-b", State: workersessions.StateRunning, DurationBasis: workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable},
	}
	sources := make([]workersessions.Service, 0, len(rows))
	for _, row := range rows {
		sources = append(sources, fleetHistoryPageFake{read: func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
			return workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{row}}, nil
		}})
	}
	clock := platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond)
	query := NewFleetHistory(func(context.Context) ([]workersessions.Service, error) {
		return sources, nil
	}, nil, clock, logging.NoopLogger{}, newTestHistoryBudget(), nil)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 1}
	one, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(one.Observations) != 1 || one.NextToken == "" {
		t.Fatalf("first=%+v err=%v", one, err)
	}
	if !reflect.DeepEqual(one.Observations[0], rows[0]) {
		t.Fatalf("first scoped observation=%+v; want %+v", one.Observations[0], rows[0])
	}
	req.NextToken = one.NextToken
	two, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(two.Observations) != 1 || two.Observations[0].FactorySessionID == one.Observations[0].FactorySessionID || two.NextToken != "" {
		t.Fatalf("second=%+v err=%v", two, err)
	}
	if !reflect.DeepEqual(two.Observations[0], rows[1]) {
		t.Fatalf("second scoped observation=%+v; want %+v", two.Observations[0], rows[1])
	}
	req.History = ""
	if _, err := query.ListWorkerSessionObservations(t.Context(), req); err == nil {
		t.Fatal("history snapshot accepted as compatibility cursor")
	}
}

func TestFleetHistoryReadsBoundedOwnerPagesAndRejectsMalformedSources(t *testing.T) {
	t.Parallel()
	r := newObservationRegistry(nil)
	addActiveHistoryFixture(r, "a", true)
	addActiveHistoryFixture(r, "b", true)
	addActiveHistoryFixture(r, "c", true)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 2}
	rows, err := r.activeHistoryObservations(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	paged := fleetHistoryPageFake{read: func(request workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
		calls++
		if request.NextToken == "" {
			return workersessions.ListWorkerSessionObservationsResult{Observations: rows[:2], NextToken: "next"}, nil
		}
		if request.NextToken != "next" || request.MaxResults != 2 {
			t.Fatalf("unexpected source request: %+v", request)
		}
		return workersessions.ListWorkerSessionObservationsResult{Observations: rows[2:]}, nil
	}}
	got, err := readFleetHistoryOwners(t.Context(), paged, req)
	if err != nil || !reflect.DeepEqual(got, rows) || calls != 2 {
		t.Fatalf("bounded pages: count=%d err=%v", len(rows), err)
	}
	for _, tc := range []struct {
		name string
		page workersessions.ListWorkerSessionObservationsResult
		err  error
		want error
	}{
		{"short continuation", workersessions.ListWorkerSessionObservationsResult{Observations: rows[:1], NextToken: "next"}, nil, workersessions.ErrInvalidObservationPagination},
		{"duplicate identity", workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{rows[0], rows[0]}}, nil, workersessions.ErrObservationProjectionUnavailable},
		{"unsorted", workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{rows[1], rows[0]}}, nil, workersessions.ErrObservationProjectionUnavailable},
		{"unavailable source", workersessions.ListWorkerSessionObservationsResult{}, workersessions.ErrObservationProjectionUnavailable, workersessions.ErrObservationProjectionUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := fleetHistoryPageFake{read: func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
				return tc.page, tc.err
			}}
			if _, err := readFleetHistoryOwners(t.Context(), source, req); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
}

type fleetHistoryLogsFake struct {
	read func(context.Context, workersessions.ListWorkerSessionObservationsRequest, map[historyIdentity]struct{}) ([]workersessions.Observation, error)
}

func (f fleetHistoryLogsFake) archivedHistory(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest, owners map[historyIdentity]struct{}) ([]workersessions.Observation, error) {
	return f.read(ctx, req, owners)
}

func TestFleetHistoryInjectedRolesFreezePagesAndLogOutcomes(t *testing.T) {
	t.Parallel()
	row := func(id, factory string, state workersessions.State) workersessions.Observation {
		return workersessions.Observation{WorkerSessionID: id, FactorySessionID: factory, AttemptID: "attempt-" + id, State: state, DurationBasis: workersessions.DurationBasisUnavailable, Transcript: workersessions.TranscriptAvailabilityUnavailable}
	}
	live := []workersessions.Observation{row("a", "factory-a", workersessions.StateRunning), row("b", "factory-b", workersessions.StateRunning)}
	catalogCalls, archiveCalls := 0, 0
	catalog := func(context.Context) ([]workersessions.Service, error) {
		catalogCalls++
		return []workersessions.Service{fleetHistoryPageFake{read: func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
			return workersessions.ListWorkerSessionObservationsResult{Observations: live}, nil
		}}}, nil
	}
	archived := row("c", "factory-old", workersessions.StateCompleted)
	var archiveErr error
	logs := fleetHistoryLogsFake{read: func(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest, owners map[historyIdentity]struct{}) ([]workersessions.Observation, error) {
		archiveCalls++
		if req.History != workersessions.ObservationHistoryAll || len(owners) != len(live) {
			t.Fatalf("archive selection: %+v owners=%+v", req, owners)
		}
		if archiveErr != nil {
			return nil, archiveErr
		}
		return []workersessions.Observation{archived}, ctx.Err()
	}}
	logger := &recordingLogger{}
	query := NewFleetHistory(catalog, logs, platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond), logger, newTestHistoryBudget(), nil)
	req := workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryAll, MaxResults: 1}
	first, err := query.ListWorkerSessionObservations(t.Context(), req)
	if err != nil || len(first.Observations) != 1 || first.Observations[0].WorkerSessionID != "a" || first.NextToken == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	live = append(live, row("d", "factory-new", workersessions.StateRunning))
	req.NextToken = first.NextToken
	for _, id := range []string{"b", "c"} {
		page, err := query.ListWorkerSessionObservations(t.Context(), req)
		if err != nil || len(page.Observations) != 1 || page.Observations[0].WorkerSessionID != id {
			t.Fatalf("frozen=%+v err=%v", page, err)
		}
		req.NextToken = page.NextToken
	}
	if req.NextToken != "" || catalogCalls != 1 || archiveCalls != 1 {
		t.Fatal("continuation resampled completed roles")
	}
	fresh, err := query.ListWorkerSessionObservations(t.Context(), workersessions.ListWorkerSessionObservationsRequest{History: workersessions.ObservationHistoryActive, MaxResults: 10})
	if err != nil || len(fresh.Observations) != 3 || fresh.Observations[2].FactorySessionID != "factory-new" || archiveCalls != 1 {
		t.Fatalf("new owner=%+v err=%v", fresh, err)
	}
	archiveErr = workersessions.ErrObservationProjectionUnavailable
	if _, err := query.ListWorkerSessionObservations(t.Context(), req); !errors.Is(err, archiveErr) {
		t.Fatalf("archive failure=%v", err)
	}
	entries := logger.entriesFor("worker session fleet history list")
	if len(entries) != 5 || entries[4].fields["failed"] != true || entries[0].fields["has_next"] != true {
		t.Fatalf("selected logger outcomes=%+v", entries)
	}
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer cancel()
	if _, err := query.ListWorkerSessionObservations(ctx, req); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline=%v", err)
	}
	if catalogCalls != 3 {
		t.Fatalf("expired read sampled catalog: calls=%d", catalogCalls)
	}
}

func fleetHistoryOwnerFixture(rows ...workersessions.Observation) workersessions.Service {
	return fleetHistoryPageFake{read: func(workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
		return workersessions.ListWorkerSessionObservationsResult{Observations: rows}, nil
	}}
}
