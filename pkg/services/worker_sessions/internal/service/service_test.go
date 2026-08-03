package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newRegistry() workersessions.Service {
	return newRegistryWithExecution(succeedingExecution())
}

func newRegistryWithExecution(execution workers.WorkstationExecutionService) workersessions.Service {
	registry, err := service.New(execution, logging.NoopLogger{})
	if err != nil {
		panic(fmt.Sprintf("service.New() error = %v, want nil", err))
	}
	return registry
}

func TestReserve_ValidIdentity_StoresSessionInReservedState(t *testing.T) {
	registry := newRegistry()

	session, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if session.ID != "worker-1" || session.State != workersessions.StateReserved {
		t.Fatalf("Reserve() = %+v, want ID=worker-1 State=RESERVED", session)
	}
}

func TestReserve_InvalidIdentity_ReturnsTypedValidationError(t *testing.T) {
	registry := newRegistry()

	_, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "   "})
	if !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Reserve() error = %v, want ErrInvalidSessionID", err)
	}
}

func TestReserve_DuplicateIdentity_ReturnsTypedErrorAndLeavesExistingSessionUnchanged(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	first, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("first Reserve() error = %v, want nil", err)
	}

	_, err = registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"})
	if !errors.Is(err, workersessions.ErrSessionAlreadyExists) {
		t.Fatalf("duplicate Reserve() error = %v, want ErrSessionAlreadyExists", err)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != first {
		t.Fatalf("Get() after duplicate Reserve() = %+v, want unchanged %+v", got, first)
	}
}

func TestGet_UnknownIdentity_ReturnsTypedNotFoundDistinctFromValidationFailure(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	_, notFoundErr := registry.Get(ctx, workersessions.GetRequest{ID: "missing"})
	if !errors.Is(notFoundErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() error = %v, want ErrSessionNotFound", notFoundErr)
	}

	_, invalidErr := registry.Get(ctx, workersessions.GetRequest{ID: ""})
	if !errors.Is(invalidErr, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Get() error = %v, want ErrInvalidSessionID", invalidErr)
	}

	if errors.Is(notFoundErr, workersessions.ErrInvalidSessionID) || errors.Is(invalidErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("not-found and validation errors must be distinguishable")
	}
}

func TestGet_RepeatedCalls_ReturnSameIdentityAndCurrentState(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	first, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("first Get() error = %v, want nil", err)
	}
	second, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("second Get() error = %v, want nil", err)
	}
	if first != second {
		t.Fatalf("repeated Get() = %+v then %+v, want identical snapshots", first, second)
	}
}

func TestGet_MutatingReturnedSnapshot_DoesNotAffectLaterGet(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	snapshot, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	snapshot.State = workersessions.StateTerminated
	snapshot.ID = "mutated"
	t.Logf("mutated local snapshot: %+v", snapshot)

	after, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if after.State != workersessions.StateReserved || after.ID != "worker-1" {
		t.Fatalf("mutating a returned snapshot leaked into the registry: got %+v", after)
	}
}

func TestList_EmptyRegistry_ReturnsSuccessfulEmptyResult(t *testing.T) {
	registry := newRegistry()

	result, err := registry.List(context.Background(), workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() sessions = %v, want empty", result.Sessions)
	}
}

func TestList_MultipleInsertionOrders_ReturnSameDeterministicOrdering(t *testing.T) {
	ctx := context.Background()
	ids := []string{"worker-c", "worker-a", "worker-b"}
	reverseIDs := []string{"worker-b", "worker-a", "worker-c"}

	forward := newRegistry()
	for _, id := range ids {
		if _, err := forward.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
			t.Fatalf("Reserve(%q) error = %v, want nil", id, err)
		}
	}
	reversed := newRegistry()
	for _, id := range reverseIDs {
		if _, err := reversed.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
			t.Fatalf("Reserve(%q) error = %v, want nil", id, err)
		}
	}

	forwardResult, err := forward.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	reversedResult, err := reversed.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	wantOrder := []string{"worker-a", "worker-b", "worker-c"}
	if got := idsOf(forwardResult.Sessions); !equalStrings(got, wantOrder) {
		t.Fatalf("List() order = %v, want %v", got, wantOrder)
	}
	if got := idsOf(reversedResult.Sessions); !equalStrings(got, wantOrder) {
		t.Fatalf("List() order = %v, want %v", got, wantOrder)
	}
}

func TestList_FilterByState_ReturnsExactlyMatchingSessions(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-2"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateRunning}}})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() filtered by RUNNING = %v, want empty (all sessions are RESERVED)", result.Sessions)
	}

	result, err = registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateReserved}}})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if got := idsOf(result.Sessions); !equalStrings(got, []string{"worker-1", "worker-2"}) {
		t.Fatalf("List() filtered by RESERVED = %v, want [worker-1 worker-2]", got)
	}
}

func TestList_InvalidFilter_ReturnsTypedValidationErrorAndNoPartialResult(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}})
	if !errors.Is(err, workersessions.ErrInvalidState) {
		t.Fatalf("List() error = %v, want ErrInvalidState", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() with invalid filter returned a partial result: %v", result.Sessions)
	}
}

func TestList_MutatingReturnedResult_DoesNotAffectLaterListOrGet(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	result.Sessions[0].State = workersessions.StateTerminated
	result.Sessions = append(result.Sessions, workersessions.Session{ID: "injected", State: workersessions.StateRunning})

	after, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(after.Sessions) != 1 || after.Sessions[0].State != workersessions.StateReserved {
		t.Fatalf("mutating a returned List() result leaked into the registry: got %+v", after.Sessions)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateReserved {
		t.Fatalf("Get() after mutating a List() result = %+v, want State=RESERVED", got)
	}
}

func TestConcurrentReserve_DistinctIdentities_RetainsEveryUniqueSession(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const count = 200

	var wg sync.WaitGroup
	wg.Add(count)
	for i := range count {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
				t.Errorf("Reserve(%q) error = %v, want nil", id, err)
			}
		}(i)
	}
	wg.Wait()

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != count {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), count)
	}
	for _, session := range result.Sessions {
		if session.State != workersessions.StateReserved {
			t.Errorf("session %q state = %q, want RESERVED", session.ID, session.State)
		}
	}
}

func TestConcurrentReserve_SameIdentity_ExactlyOneSucceeds(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const attempts = 50

	var wg sync.WaitGroup
	results := make([]error, attempts)
	wg.Add(attempts)
	for i := range attempts {
		go func(i int) {
			defer wg.Done()
			_, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "shared"})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes, duplicates := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, workersessions.ErrSessionAlreadyExists):
			duplicates++
		default:
			t.Fatalf("Reserve() error = %v, want nil or ErrSessionAlreadyExists", err)
		}
	}
	if successes != 1 || duplicates != attempts-1 {
		t.Fatalf("got %d successes and %d duplicates, want exactly 1 success and %d duplicates", successes, duplicates, attempts-1)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "shared"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ID != "shared" || got.State != workersessions.StateReserved {
		t.Fatalf("Get() after concurrent duplicate Reserve() = %+v, want ID=shared State=RESERVED", got)
	}
}

func TestConcurrentGetAndList_DuringReservation_ReturnInternallyConsistentSnapshots(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const writers = 50
	const readers = 50

	var wg sync.WaitGroup
	wg.Add(writers + readers)
	for i := range writers {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
				t.Errorf("Reserve(%q) error = %v, want nil", id, err)
			}
		}(i)
	}
	for i := range readers {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i%writers)
			if session, err := registry.Get(ctx, workersessions.GetRequest{ID: id}); err == nil {
				if err := session.Validate(); err != nil {
					t.Errorf("Get(%q) returned an invalid snapshot: %v", id, err)
				}
			} else if !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Errorf("Get(%q) error = %v, want nil or ErrSessionNotFound", id, err)
			}
			if result, err := registry.List(ctx, workersessions.ListRequest{}); err != nil {
				t.Errorf("List() error = %v, want nil", err)
			} else {
				for _, session := range result.Sessions {
					if err := session.Validate(); err != nil {
						t.Errorf("List() returned an invalid snapshot: %v", err)
					}
				}
			}
		}(i)
	}
	wg.Wait()

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != writers {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), writers)
	}
}

func idsOf(sessions []workersessions.Session) []string {
	ids := make([]string, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
