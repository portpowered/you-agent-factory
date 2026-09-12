package wire

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestFleetObservationServiceCharacterizesPagedOrderAndCursor(t *testing.T) {
	t.Parallel()

	firstSources := newInterleavedFleetSources()
	firstService := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{firstSources[2], firstSources[0], firstSources[1]}, nil
	})
	firstPage, firstTokens := listFleetObservationPages(t, firstService, workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		MaxResults: 3,
	})
	assertFleetObservationIDs(t, firstPage, expectedInterleavedFleetIDs())
	if len(firstTokens) != 6 || firstTokens[len(firstTokens)-1] != "" {
		t.Fatalf("fleet page tokens = %#v, want five continuation tokens and an exhausted page", firstTokens)
	}
	for _, source := range firstSources {
		t.Logf("baseline fixture=fleet-characterization-v1 source=%s requests=%+v", source.name, source.requestsSnapshot())
	}

	secondSources := newInterleavedFleetSources()
	secondService := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{secondSources[1], secondSources[2], secondSources[0]}, nil
	})
	secondPage, _ := listFleetObservationPages(t, secondService, workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		MaxResults: 3,
	})
	assertFleetObservationIDs(t, secondPage, expectedInterleavedFleetIDs())

	for _, source := range firstSources {
		requests := source.requestsSnapshot()
		if len(requests) != 6 {
			t.Fatalf("%s bounded call count = %d, want one call per fleet page", source.name, len(requests))
		}
		for _, request := range requests {
			if request.Scope != workersessions.ObservationScopeAll || request.MaxResults != 4 || len(request.States) != 0 {
				t.Fatalf("%s bounded request = %#v, want all scope, lookahead limit 4, and no states", source.name, request)
			}
		}
	}
}

func TestFleetObservationServiceUsesOneLookaheadReadPerFleetPage(t *testing.T) {
	t.Parallel()

	inventory := make([]workersessions.Observation, 0, 45)
	wantIDs := make([]string, 0, 45)
	for id := 1; id <= 45; id++ {
		workerSessionID := fmt.Sprintf("worker-%02d", id)
		inventory = append(inventory, fleetObservation(workerSessionID, true, workersessions.StateCompleted))
		wantIDs = append(wantIDs, workerSessionID)
	}
	source := newFleetObservationSource("bounded", inventory...)
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{source}, nil
	})

	request := workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		MaxResults: 20,
	}
	var got []workersessions.Observation
	wantPageLengths := []int{20, 20, 5}
	wantCursors := []string{"", "worker-20", "worker-40"}
	for index, wantPageLength := range wantPageLengths {
		result, err := service.ListWorkerSessionObservations(context.Background(), request)
		if err != nil {
			t.Fatalf("bounded fleet page %d: %v", index+1, err)
		}
		if len(result.Observations) != wantPageLength {
			t.Fatalf("bounded fleet page %d rows = %d, want %d", index+1, len(result.Observations), wantPageLength)
		}
		wantNextToken := ""
		if index < len(wantPageLengths)-1 {
			wantNextToken = encodeFleetObservationCursor(wantCursors[index+1])
		}
		if result.NextToken != wantNextToken {
			t.Fatalf("bounded fleet page %d token = %q, want %q", index+1, result.NextToken, wantNextToken)
		}
		got = append(got, result.Observations...)
		request.NextToken = result.NextToken
	}
	assertFleetObservationIDs(t, got, wantIDs)

	requests := source.requestsSnapshot()
	if len(requests) != len(wantPageLengths) {
		t.Fatalf("bounded source calls = %d, want %d", len(requests), len(wantPageLengths))
	}
	for index, request := range requests {
		if request.Scope != workersessions.ObservationScopeAll || request.MaxResults != 21 {
			t.Fatalf("bounded source request[%d] = %#v, want all scope and lookahead limit 21", index, request)
		}
		if request.NextToken != encodeFleetObservationCursor(wantCursors[index]) {
			t.Fatalf("bounded source request[%d] cursor = %q, want %q", index, request.NextToken, encodeFleetObservationCursor(wantCursors[index]))
		}
	}
}

func TestFleetLookaheadLimitSaturatesAtIntegerMaximum(t *testing.T) {
	if got := fleetLookaheadLimit(20); got != 21 {
		t.Fatalf("fleetLookaheadLimit(20) = %d, want 21", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := fleetLookaheadLimit(maxInt); got != maxInt {
		t.Fatalf("fleetLookaheadLimit(maxInt) = %d, want saturation at maxInt", got)
	}
}

func TestFleetObservationServiceMergesComplementaryFactsWithoutOverwriting(t *testing.T) {
	t.Parallel()

	primary := fleetObservation("merge-01", true, workersessions.StateFailed)
	authoritativeModel := "authoritative-model"
	primary.Model = &authoritativeModel
	authoritativeAttempt := primary.AttemptID
	primary.FactorySessionID = ""
	primary.ProviderSession = providers.SessionRef{}
	primary.ProviderSessionAvailable = false
	primary.WorkIDs = nil
	primary.TurnID = ""
	primary.ReasoningEffort = nil
	primary.StartedAt = nil
	primary.EndedAt = nil
	primary.Duration = nil
	primary.DurationBasis = ""
	primary.TokenUsage = nil
	primary.TurnUsage = nil
	primary.Transcript = ""
	primary.RecordingHealth = ""
	primary.RecordingHealthReason = ""
	primary.ConfirmationState = ""
	primary.Failure = nil
	primary.Parse = workersessions.ParseDiagnostics{}

	complementary := fleetObservation("merge-01", false, workersessions.StateFailed)
	conflictingModel := "conflicting-model"
	complementary.Model = &conflictingModel
	complementary.AttemptID = "secondary-attempt"
	complementary.RecordingHealthReason = "secondary-recording"

	primarySource := newFleetObservationSource("primary", primary)
	secondarySource := newFleetObservationSource("secondary", complementary)
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{primarySource, secondarySource}, nil
	})

	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		MaxResults: 20,
	})
	if err != nil {
		t.Fatalf("complementary fleet list: %v", err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("complementary fleet rows = %d, want one unique identity", len(result.Observations))
	}
	got := result.Observations[0]
	if got.Model == nil || *got.Model != authoritativeModel {
		t.Fatalf("duplicate model = %#v, want authoritative primary model", got.Model)
	}
	if got.AttemptID != authoritativeAttempt || got.Direct != primary.Direct || got.State != primary.State {
		t.Fatalf("authoritative identity facts changed: %#v", got)
	}
	if got.FactorySessionID != complementary.FactorySessionID ||
		got.ProviderSession != complementary.ProviderSession ||
		!got.ProviderSessionAvailable ||
		!reflect.DeepEqual(got.WorkIDs, complementary.WorkIDs) ||
		got.TurnID != complementary.TurnID ||
		got.ReasoningEffort == nil ||
		got.StartedAt == nil || got.EndedAt == nil || got.Duration == nil ||
		got.DurationBasis != complementary.DurationBasis ||
		got.Transcript != complementary.Transcript ||
		got.RecordingHealth != complementary.RecordingHealth ||
		got.RecordingHealthReason != complementary.RecordingHealthReason ||
		got.ConfirmationState != complementary.ConfirmationState ||
		got.Failure == nil || got.Failure.Detail != complementary.Failure.Detail ||
		!reflect.DeepEqual(got.Parse, complementary.Parse) ||
		got.TokenUsage == nil || got.TurnUsage == nil {
		t.Fatalf("complementary facts were not preserved: %#v", got)
	}
}

func TestFleetObservationServiceRejectsMalformedSourcePages(t *testing.T) {
	t.Parallel()

	pageObservation := func(id string, direct bool, state workersessions.State) workersessions.Observation {
		return fleetObservation(id, direct, state)
	}
	cases := []struct {
		name    string
		request workersessions.ListWorkerSessionObservationsRequest
		result  workersessions.ListWorkerSessionObservationsResult
	}{
		{
			name: "overfull",
			result: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
				pageObservation("a", true, workersessions.StateCompleted),
				pageObservation("b", true, workersessions.StateCompleted),
				pageObservation("c", true, workersessions.StateCompleted),
				pageObservation("d", true, workersessions.StateCompleted),
			}},
		},
		{
			name: "duplicate identity",
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("b", true, workersessions.StateCompleted),
					pageObservation("b", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("b"),
			},
		},
		{
			name: "regressing identities",
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("b", true, workersessions.StateCompleted),
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("c", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("c"),
			},
		},
		{
			name: "row before cursor",
			request: workersessions.ListWorkerSessionObservationsRequest{
				Scope: workersessions.ObservationScopeAll, MaxResults: 2,
				NextToken: encodeFleetObservationCursor("b"),
			},
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("c", true, workersessions.StateCompleted),
					pageObservation("d", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("d"),
			},
		},
		{
			name: "scope mismatch",
			request: workersessions.ListWorkerSessionObservationsRequest{
				Scope: workersessions.ObservationScopeDirect, MaxResults: 2,
			},
			result: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
				pageObservation("a", false, workersessions.StateCompleted),
			}},
		},
		{
			name: "state mismatch",
			request: workersessions.ListWorkerSessionObservationsRequest{
				Scope: workersessions.ObservationScopeAll, States: []workersessions.State{workersessions.StateCompleted}, MaxResults: 2,
			},
			result: workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
				pageObservation("a", true, workersessions.StateFailed),
			}},
		},
		{
			name: "malformed continuation",
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("b", true, workersessions.StateCompleted),
					pageObservation("c", true, workersessions.StateCompleted),
				},
				NextToken: "not-base64",
			},
		},
		{
			name: "repeated continuation",
			request: workersessions.ListWorkerSessionObservationsRequest{
				Scope: workersessions.ObservationScopeAll, MaxResults: 2,
				NextToken: encodeFleetObservationCursor("cursor"),
			},
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("cursor-a", true, workersessions.StateCompleted),
					pageObservation("cursor-b", true, workersessions.StateCompleted),
					pageObservation("cursor-c", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("cursor"),
			},
		},
		{
			name: "underfilled continuation",
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("b", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("b"),
			},
		},
		{
			name: "regressing continuation",
			result: workersessions.ListWorkerSessionObservationsResult{
				Observations: []workersessions.Observation{
					pageObservation("a", true, workersessions.StateCompleted),
					pageObservation("b", true, workersessions.StateCompleted),
					pageObservation("c", true, workersessions.StateCompleted),
				},
				NextToken: encodeFleetObservationCursor("b"),
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := test.request
			if request.MaxResults == 0 {
				request.MaxResults = 2
			}
			bad := newFleetObservationSource("bad")
			bad.resultFn = func(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
				return test.result, nil
			}
			later := newFleetObservationSource("later", fleetObservation("z", true, workersessions.StateCompleted))
			service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
				return []workersessions.Service{bad, later}, nil
			})

			result, err := service.ListWorkerSessionObservations(context.Background(), request)
			if !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
				t.Fatalf("malformed %s error = %v, want invalid pagination", test.name, err)
			}
			if len(result.Observations) != 0 || result.NextToken != "" {
				t.Fatalf("malformed %s returned partial result: %#v", test.name, result)
			}
			if len(bad.requestsSnapshot()) != 1 || len(later.requestsSnapshot()) != 0 {
				t.Fatalf("malformed %s calls = bad:%d later:%d, want one bounded call and no later source", test.name, len(bad.requestsSnapshot()), len(later.requestsSnapshot()))
			}
		})
	}
}

func TestFleetObservationServiceStopsOnUnavailableProjectionAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("complete projection unavailable", func(t *testing.T) {
		t.Parallel()
		available := newFleetObservationSource("available", fleetObservation("a", true, workersessions.StateCompleted))
		unavailable := newFleetObservationSource("unavailable")
		unavailable.err = workersessions.ErrObservationProjectionUnavailable
		later := newFleetObservationSource("later", fleetObservation("z", true, workersessions.StateCompleted))
		service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
			return []workersessions.Service{available, unavailable, later}, nil
		})

		result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 2})
		if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
			t.Fatalf("complete projection error = %v, want projection unavailable", err)
		}
		if len(result.Observations) != 0 || len(later.requestsSnapshot()) != 0 {
			t.Fatalf("complete projection returned partial/later work: %#v, later calls=%d", result, len(later.requestsSnapshot()))
		}
	})

	t.Run("cancellation after bounded source call", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		first := newFleetObservationSource("first")
		first.resultFn = func(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
			cancel()
			return workersessions.ListWorkerSessionObservationsResult{Observations: []workersessions.Observation{
				fleetObservation("a", true, workersessions.StateCompleted),
			}}, nil
		}
		later := newFleetObservationSource("later", fleetObservation("z", true, workersessions.StateCompleted))
		service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
			return []workersessions.Service{first, later}, nil
		})

		result, err := service.ListWorkerSessionObservations(ctx, workersessions.ListWorkerSessionObservationsRequest{MaxResults: 2})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled fleet error = %v, want context canceled", err)
		}
		if len(result.Observations) != 0 || len(later.requestsSnapshot()) != 0 {
			t.Fatalf("canceled fleet returned partial/later work: %#v, later calls=%d", result, len(later.requestsSnapshot()))
		}
	})

	t.Run("catalog failure", func(t *testing.T) {
		t.Parallel()
		catalogErr := errors.New("catalog unavailable")
		service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
			return nil, catalogErr
		})

		result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 2})
		if !errors.Is(err, catalogErr) {
			t.Fatalf("catalog failure = %v, want catalog error", err)
		}
		if len(result.Observations) != 0 || result.NextToken != "" {
			t.Fatalf("catalog failure returned partial result: %#v", result)
		}
	})
}

func TestFleetObservationServiceCharacterizesFiltersFactsAndDetachment(t *testing.T) {
	t.Parallel()

	sources, primary, primaryBefore := newFilteredFleetSources()
	initialInventories := make([][]workersessions.Observation, len(sources))
	for index, source := range sources {
		initialInventories[index] = source.inventorySnapshot()
	}
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{sources[0], sources[1], sources[2], sources[3]}, nil
	})
	states := []workersessions.State{
		workersessions.StateCompleted,
		workersessions.StateFailed,
		workersessions.StateCompleted,
	}
	statesBefore := append([]workersessions.State(nil), states...)
	assertFilteredFleetScopes(t, service, sources, states)
	if !reflect.DeepEqual(states, statesBefore) {
		t.Fatalf("caller state filter changed: got %#v, want %#v", states, statesBefore)
	}

	allResult, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		States:     states,
		MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("all-scope fact list: %v", err)
	}
	returnedPrimary := fleetObservationByID(t, allResult.Observations, primary.WorkerSessionID)
	if !reflect.DeepEqual(returnedPrimary, primaryBefore) {
		t.Fatalf("duplicate returned facts changed: got %#v, want %#v", returnedPrimary, primaryBefore)
	}
	mutateFleetObservation(returnedPrimary)
	if !reflect.DeepEqual(primary, primaryBefore) {
		t.Fatalf("caller-owned duplicate facts were mutated: got %#v, want %#v", primary, primaryBefore)
	}

	for index, source := range sources {
		after := source.inventorySnapshot()
		if !reflect.DeepEqual(after, initialInventories[index]) {
			t.Fatalf("%s inventory was mutated: got %#v, want %#v", source.name, after, initialInventories[index])
		}
	}

	freshResult, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		Scope:      workersessions.ObservationScopeAll,
		States:     states,
		MaxResults: 50,
	})
	if err != nil {
		t.Fatalf("fresh all-scope fact list: %v", err)
	}
	freshPrimary := fleetObservationByID(t, freshResult.Observations, primary.WorkerSessionID)
	if !reflect.DeepEqual(freshPrimary, primaryBefore) {
		t.Fatalf("source returned shared mutable facts after caller mutation: got %#v, want %#v", freshPrimary, primaryBefore)
	}
	for _, source := range sources {
		assertFleetSourceRequestHistory(t, source, []workersessions.ObservationScope{
			workersessions.ObservationScopeDirect,
			workersessions.ObservationScopeFactory,
			workersessions.ObservationScopeAll,
		}, states)
		t.Logf("baseline filtered fixture source=%s requests=%+v", source.name, source.requestsSnapshot())
	}
}

func assertFilteredFleetScopes(
	t *testing.T,
	service *FleetObservationService,
	sources []*fleetObservationSource,
	states []workersessions.State,
) {
	t.Helper()
	cases := []struct {
		name  string
		scope workersessions.ObservationScope
		want  []string
	}{
		{name: "direct", scope: workersessions.ObservationScopeDirect, want: []string{"filter-01", "filter-05", "filter-07", "filter-09"}},
		{name: "factory", scope: workersessions.ObservationScopeFactory, want: []string{"filter-02", "filter-04", "filter-08", "filter-10"}},
		{name: "all", scope: workersessions.ObservationScopeAll, want: []string{"filter-01", "filter-02", "filter-04", "filter-05", "filter-07", "filter-08", "filter-09", "filter-10"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := workersessions.ListWorkerSessionObservationsRequest{
				Scope:      test.scope,
				States:     states,
				MaxResults: 50,
			}
			result, err := service.ListWorkerSessionObservations(context.Background(), request)
			if err != nil {
				t.Fatalf("%s fleet list: %v", test.name, err)
			}
			assertFleetObservationIDs(t, result.Observations, test.want)
			if result.NextToken != "" {
				t.Fatalf("%s next token = %q, want exhausted", test.name, result.NextToken)
			}
			for _, source := range sources {
				assertFleetSourceRequestFilters(t, source, test.scope, states)
			}
		})
	}
}

func TestFleetObservationServicePreservesOptionalProjectionFacts(t *testing.T) {
	source := newFleetObservationSource("projection", fleetObservation("worker-base", true, workersessions.StateFailed))
	source.err = workersessions.ErrObservationProjectionUnavailable
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{source}, nil
	})

	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil {
		t.Fatalf("fleet list with unavailable optional projection: %v", err)
	}
	assertFleetObservationIDs(t, result.Observations, []string{"worker-base"})
	if len(source.requestsSnapshot()) != 1 {
		t.Fatalf("optional projection source calls = %d, want one", len(source.requestsSnapshot()))
	}
}

func TestFleetObservationServiceReturnsNonNilEmptyAndRejectsInvalidInput(t *testing.T) {
	empty := newFleetObservationSource("empty")
	service := NewFleetObservationService(func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{empty}, nil
	})
	result, err := service.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{})
	if err != nil || result.Observations == nil || len(result.Observations) != 0 {
		t.Fatalf("empty fleet list = %#v, %v, want non-nil empty result", result, err)
	}
	if result.MaxResults != workersessions.DefaultWorkerSessionObservationListMaxResults {
		t.Fatalf("empty fleet max results = %d, want default %d", result.MaxResults, workersessions.DefaultWorkerSessionObservationListMaxResults)
	}

	for name, request := range map[string]workersessions.ListWorkerSessionObservationsRequest{
		"invalid scope":  {Scope: workersessions.ObservationScope("unknown")},
		"negative limit": {MaxResults: -1},
		"invalid cursor": {NextToken: "%%%"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.ListWorkerSessionObservations(context.Background(), request); err == nil {
				t.Fatal("invalid request unexpectedly succeeded")
			}
		})
	}

	if NewFleetObservationService(nil) != nil {
		t.Fatal("NewFleetObservationService(nil) returned a service")
	}
	var unavailable *FleetObservationService
	if _, err := unavailable.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("nil fleet service error = %v, want projection unavailable", err)
	}
	got, err := decodeFleetObservationCursor(base64.StdEncoding.EncodeToString([]byte("worker-1")))
	if err != nil || got != "worker-1" {
		t.Fatalf("decoded cursor = %q, %v, want worker-1", got, err)
	}
}

func listFleetObservationPages(
	t *testing.T,
	service *FleetObservationService,
	request workersessions.ListWorkerSessionObservationsRequest,
) ([]workersessions.Observation, []string) {
	t.Helper()
	var observations []workersessions.Observation
	var tokens []string
	seenTokens := make(map[string]struct{})
	for {
		result, err := service.ListWorkerSessionObservations(context.Background(), request)
		if err != nil {
			t.Fatalf("fleet page with token %q: %v", request.NextToken, err)
		}
		observations = append(observations, result.Observations...)
		tokens = append(tokens, result.NextToken)
		if result.NextToken == "" {
			return observations, tokens
		}
		if _, exists := seenTokens[result.NextToken]; exists {
			t.Fatalf("fleet cursor repeated: %q", result.NextToken)
		}
		seenTokens[result.NextToken] = struct{}{}
		request.NextToken = result.NextToken
	}
}

func newInterleavedFleetSources() []*fleetObservationSource {
	sourceA := make([]workersessions.Observation, 0, 6)
	sourceB := make([]workersessions.Observation, 0, 6)
	sourceC := make([]workersessions.Observation, 0, 7)
	for _, id := range []int{1, 4, 7, 10, 13, 16} {
		sourceA = append(sourceA, fleetObservation(interleavedFleetID(id), id%2 == 1, interleavedFleetState(id)))
	}
	for _, id := range []int{2, 5, 8, 11, 14, 17} {
		sourceB = append(sourceB, fleetObservation(interleavedFleetID(id), id%2 == 1, interleavedFleetState(id)))
	}
	for _, id := range []int{3, 6, 8, 9, 12, 15, 18} {
		sourceC = append(sourceC, fleetObservation(interleavedFleetID(id), id%2 == 1, interleavedFleetState(id)))
	}
	return []*fleetObservationSource{
		newFleetObservationSource("source-a", sourceA...),
		newFleetObservationSource("source-b", sourceB...),
		newFleetObservationSource("source-c", sourceC...),
	}
}

func expectedInterleavedFleetIDs() []string {
	ids := make([]string, 0, 18)
	for id := 1; id <= 18; id++ {
		ids = append(ids, interleavedFleetID(id))
	}
	return ids
}

func interleavedFleetID(id int) string { return fmt.Sprintf("session-%02d", id) }

func interleavedFleetState(id int) workersessions.State {
	if id%2 == 0 {
		return workersessions.StateFailed
	}
	return workersessions.StateCompleted
}

func newFilteredFleetSources() ([]*fleetObservationSource, workersessions.Observation, workersessions.Observation) {
	primary := fleetObservation("filter-07", true, workersessions.StateCompleted)
	primaryBefore := primary.Clone()
	complementary := primary.Clone()
	complementary.WorkIDs = []string{"complementary-work"}
	complementaryModel := "complementary-model"
	complementary.Model = &complementaryModel
	complementary.Parse.Errors = []workersessions.ParseDiagnostic{{Code: "COMPLEMENTARY", LineNumber: 7, Message: "secondary fact"}}
	return []*fleetObservationSource{
		newFleetObservationSource("direct-primary",
			fleetObservation("filter-01", true, workersessions.StateCompleted),
			fleetObservation("filter-03", true, workersessions.StateRunning),
			fleetObservation("filter-05", true, workersessions.StateFailed),
			primary,
			fleetObservation("filter-09", true, workersessions.StateFailed),
		),
		newFleetObservationSource("direct-duplicate", complementary),
		newFleetObservationSource("factory",
			fleetObservation("filter-02", false, workersessions.StateFailed),
			fleetObservation("filter-04", false, workersessions.StateCompleted),
			fleetObservation("filter-06", false, workersessions.StateRunning),
			fleetObservation("filter-08", false, workersessions.StateFailed),
			fleetObservation("filter-10", false, workersessions.StateCompleted),
		),
		newFleetObservationSource("empty"),
	}, primary, primaryBefore
}

func fleetObservation(id string, direct bool, state workersessions.State) workersessions.Observation {
	started := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	model := "fixture-model-" + id
	reasoningEffort := "medium"
	inputTokens := 10
	outputTokens := 4
	totalTokens := 14
	observation := workersessions.Observation{
		WorkerSessionID:          id,
		Direct:                   direct,
		FactorySessionID:         "factory-session-" + id,
		ProviderSession:          providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-" + id},
		ProviderSessionAvailable: true,
		Model:                    &model,
		ReasoningEffort:          &reasoningEffort,
		WorkIDs:                  []string{"work-" + id},
		TurnID:                   "turn-" + id,
		AttemptID:                "attempt-" + id,
		State:                    state,
		ConfirmationState:        workersessions.ConfirmationStateConfirmed,
		StartedAt:                &started,
		DurationBasis:            workersessions.DurationBasisActiveClock,
		TokenUsage:               &workersessions.TokenUsage{InputTokens: &inputTokens, OutputTokens: &outputTokens, TotalTokens: &totalTokens},
		TurnUsage:                &workersessions.TurnUsage{TurnCount: 1, FinalContextTokens: 10, PeakContextTokens: 10},
		Transcript:               workersessions.TranscriptAvailabilityAvailable,
		RecordingHealth:          recordings.WorkerRecordingStatusComplete,
		Parse:                    workersessions.ParseDiagnostics{EventCount: 2, MalformedLineCount: 1, UnknownEventCount: 1, Errors: []workersessions.ParseDiagnostic{{Code: "FIXTURE_PARSE", LineNumber: 2, Message: "bounded fixture diagnostic"}}},
	}
	if state.Terminal() {
		ended := started.Add(2 * time.Second)
		duration := 2 * time.Second
		observation.EndedAt = &ended
		observation.Duration = &duration
		observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
	}
	if state == workersessions.StateFailed {
		observation.Failure = &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseWorkersExecutionFailure,
			Detail: "bounded fixture failure",
		}
	}
	return observation
}

func assertFleetSourceRequestFilters(
	t *testing.T,
	source *fleetObservationSource,
	wantScope workersessions.ObservationScope,
	wantStates []workersessions.State,
) {
	t.Helper()
	requests := source.requestsSnapshot()
	if len(requests) == 0 {
		t.Fatalf("%s has no recorded request", source.name)
	}
	request := requests[len(requests)-1]
	if request.Scope != wantScope || request.MaxResults != 51 || !reflect.DeepEqual(request.States, wantStates) {
		t.Fatalf("%s request = %#v, want scope=%q states=%#v lookahead limit=51", source.name, request, wantScope, wantStates)
	}
}

func assertFleetSourceRequestHistory(
	t *testing.T,
	source *fleetObservationSource,
	wantScopes []workersessions.ObservationScope,
	wantStates []workersessions.State,
) {
	t.Helper()
	requests := source.requestsSnapshot()
	wantScopes = append(append([]workersessions.ObservationScope(nil), wantScopes...), workersessions.ObservationScopeAll, workersessions.ObservationScopeAll)
	if len(requests) != len(wantScopes) {
		t.Fatalf("%s request count = %d, want %d", source.name, len(requests), len(wantScopes))
	}
	for index, wantScope := range wantScopes {
		request := requests[index]
		if request.Scope != wantScope || request.MaxResults != 51 || !reflect.DeepEqual(request.States, wantStates) {
			t.Fatalf("%s request[%d] = %#v, want scope=%q states=%#v lookahead limit=51", source.name, index, request, wantScope, wantStates)
		}
	}
}

func fleetObservationByID(t *testing.T, observations []workersessions.Observation, id string) workersessions.Observation {
	t.Helper()
	for _, observation := range observations {
		if observation.WorkerSessionID == id {
			return observation
		}
	}
	t.Fatalf("fleet observations did not contain %q: %#v", id, fleetObservationIDsForTest(observations))
	return workersessions.Observation{}
}

func mutateFleetObservation(observation workersessions.Observation) {
	if len(observation.WorkIDs) > 0 {
		observation.WorkIDs[0] = "mutated-work"
	}
	if observation.Model != nil {
		*observation.Model = "mutated-model"
	}
	if observation.Duration != nil {
		*observation.Duration = time.Hour
	}
	if observation.Failure != nil {
		observation.Failure.Detail = "mutated-failure"
	}
	if len(observation.Parse.Errors) > 0 {
		observation.Parse.Errors[0].Message = "mutated-parse"
	}
}

func assertFleetObservationIDs(t *testing.T, observations []workersessions.Observation, want []string) {
	t.Helper()
	got := fleetObservationIDsForTest(observations)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fleet observation IDs = %#v, want %#v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Fatalf("fleet observation IDs are not ascending: %#v", got)
	}
	for index := 1; index < len(got); index++ {
		if got[index] == got[index-1] {
			t.Fatalf("fleet observation identity repeated at index %d: %#v", index, got)
		}
	}
}

func fleetObservationIDsForTest(observations []workersessions.Observation) []string {
	ids := make([]string, 0, len(observations))
	for _, observation := range observations {
		ids = append(ids, observation.WorkerSessionID)
	}
	return ids
}

type fleetObservationSource struct {
	workersessions.Service
	name      string
	inventory []workersessions.Observation
	err       error
	resultFn  func(context.Context, workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error)

	mu       sync.Mutex
	requests []workersessions.ListWorkerSessionObservationsRequest
}

func newFleetObservationSource(name string, inventory ...workersessions.Observation) *fleetObservationSource {
	detached := make([]workersessions.Observation, 0, len(inventory))
	for _, observation := range inventory {
		detached = append(detached, observation.Clone())
	}
	return &fleetObservationSource{name: name, inventory: detached}
}

func (source *fleetObservationSource) ListWorkerSessionObservations(
	ctx context.Context,
	request workersessions.ListWorkerSessionObservationsRequest,
) (workersessions.ListWorkerSessionObservationsResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	if err := request.Validate(); err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	source.recordRequest(request)
	if source.resultFn != nil {
		return source.resultFn(ctx, request)
	}
	cursor, err := decodeFleetSourceCursor(request.NextToken)
	if err != nil {
		return workersessions.ListWorkerSessionObservationsResult{}, err
	}
	candidates := source.matchingObservations(cursor, request)
	limit := request.MaxResults
	if limit == 0 {
		limit = workersessions.DefaultWorkerSessionObservationListMaxResults
	}
	pageSize := limit
	if pageSize > len(candidates) {
		pageSize = len(candidates)
	}
	page := make([]workersessions.Observation, 0, pageSize)
	for _, observation := range candidates[:pageSize] {
		page = append(page, observation.Clone())
	}
	result := workersessions.ListWorkerSessionObservationsResult{
		Observations: page,
		MaxResults:   limit,
	}
	if pageSize < len(candidates) {
		result.NextToken = base64.StdEncoding.EncodeToString([]byte(candidates[pageSize-1].WorkerSessionID))
	}
	return result, source.err
}

func (source *fleetObservationSource) matchingObservations(
	cursor string,
	request workersessions.ListWorkerSessionObservationsRequest,
) []workersessions.Observation {
	candidates := make([]workersessions.Observation, 0, len(source.inventory))
	for _, observation := range source.inventory {
		if observation.WorkerSessionID <= cursor || !fleetSourceObservationMatches(observation, request.Scope, request.States) {
			continue
		}
		candidates = append(candidates, observation)
	}
	sort.Slice(candidates, func(left, right int) bool {
		return candidates[left].WorkerSessionID < candidates[right].WorkerSessionID
	})
	return candidates
}

func decodeFleetSourceCursor(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || strings.TrimSpace(string(decoded)) == "" {
		return "", workersessions.ErrInvalidObservationPagination
	}
	return string(decoded), nil
}

func fleetSourceObservationMatches(
	observation workersessions.Observation,
	scope workersessions.ObservationScope,
	states []workersessions.State,
) bool {
	switch scope {
	case "":
		scope = workersessions.ObservationScopeAll
	case workersessions.ObservationScopeDirect:
		if !observation.Direct {
			return false
		}
	case workersessions.ObservationScopeFactory:
		if observation.Direct {
			return false
		}
	case workersessions.ObservationScopeAll:
	default:
		return false
	}
	if scope == workersessions.ObservationScopeAll && len(states) == 0 {
		return true
	}
	if len(states) == 0 {
		return true
	}
	for _, state := range states {
		if observation.State == state {
			return true
		}
	}
	return false
}

func (source *fleetObservationSource) recordRequest(request workersessions.ListWorkerSessionObservationsRequest) {
	request.States = append([]workersessions.State(nil), request.States...)
	source.mu.Lock()
	defer source.mu.Unlock()
	source.requests = append(source.requests, request)
}

func (source *fleetObservationSource) requestsSnapshot() []workersessions.ListWorkerSessionObservationsRequest {
	source.mu.Lock()
	defer source.mu.Unlock()
	requests := make([]workersessions.ListWorkerSessionObservationsRequest, 0, len(source.requests))
	for _, request := range source.requests {
		request.States = append([]workersessions.State(nil), request.States...)
		requests = append(requests, request)
	}
	return requests
}

func (source *fleetObservationSource) inventorySnapshot() []workersessions.Observation {
	result := make([]workersessions.Observation, 0, len(source.inventory))
	for _, observation := range source.inventory {
		result = append(result, observation.Clone())
	}
	return result
}
