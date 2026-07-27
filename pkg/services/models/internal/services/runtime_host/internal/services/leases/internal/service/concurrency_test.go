package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
)

func TestConcurrentAcquireBarrierRespectsConfiguredCapacity(t *testing.T) {
	t.Parallel()

	const capacity = 4

	scope := mustRuntimeScopeRef(t, "leases-race-barrier")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	const contenders = capacity + 4
	start := make(chan struct{})
	results := make(chan error, contenders)
	var wg sync.WaitGroup

	for worker := 0; worker < contenders; worker++ {
		wg.Add(1)
		holder := fmt.Sprintf("barrier-%d", worker)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
				Scope:  request.Scope,
				Name:   request.Name,
				Holder: holder,
			})
			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, models.ErrHostCapacityExhausted):
		default:
			t.Fatalf("unexpected acquire error: %v", err)
		}
	}
	if successes != capacity {
		t.Fatalf("successful barrier acquires = %d, want %d", successes, capacity)
	}
	if observed := internalservice.ActiveCapacityForTest(service, scope, request.Name); observed != capacity {
		t.Fatalf("service active capacity = %d, want %d", observed, capacity)
	}
}

func TestConcurrentAcquireReleasePreservesCapacityAndUniqueIdentities(t *testing.T) {
	t.Parallel()

	const (
		capacity   = 4
		workers    = 16
		iterations = 50
	)

	scope := mustRuntimeScopeRef(t, "leases-race-acquire-release")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	var (
		seenIDs sync.Map
		errCh   = make(chan error, workers*iterations)
		wg      sync.WaitGroup
	)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		holder := fmt.Sprintf("worker-%d", worker)
		go func() {
			defer wg.Done()
			for range iterations {
				acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
					Scope:  request.Scope,
					Name:   request.Name,
					Holder: holder,
				})
				if err != nil {
					if errors.Is(err, models.ErrHostCapacityExhausted) {
						continue
					}
					errCh <- fmt.Errorf("AcquireModelLease: %w", err)
					return
				}

				leaseID := acquired.Lease.Lease.String()
				if leaseID == "" {
					errCh <- errors.New("successful acquire returned empty lease identity")
					return
				}
				if _, loaded := seenIDs.LoadOrStore(leaseID, struct{}{}); loaded {
					errCh <- fmt.Errorf("duplicate active lease identity %q", leaseID)
					return
				}
				if observed := internalservice.ActiveCapacityForTest(service, scope, request.Name); observed > capacity {
					errCh <- fmt.Errorf(
						"service active capacity %d exceeded limit %d at acquire boundary",
						observed,
						capacity,
					)
					return
				}

				_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
					Scope: scope,
					Lease: acquired.Lease.Lease,
				})
				if err != nil {
					errCh <- fmt.Errorf("ReleaseModelLease: %w", err)
					return
				}

				seenIDs.Delete(leaseID)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	fillCapacity(t, service, request, capacity)
}

func TestConcurrentAcquireReleaseDoesNotStrandCapacity(t *testing.T) {
	t.Parallel()

	const (
		capacity   = 3
		workers    = 12
		iterations = 40
	)

	scope := mustRuntimeScopeRef(t, "leases-race-no-strand")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	var (
		issuedMu sync.Mutex
		issued   []models.ModelLeaseRef
		errCh    = make(chan error, workers*iterations)
		wg       sync.WaitGroup
	)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		holder := fmt.Sprintf("holder-%d", worker)
		go func() {
			defer wg.Done()
			for range iterations {
				acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
					Scope:  request.Scope,
					Name:   request.Name,
					Holder: holder,
				})
				if err != nil {
					if errors.Is(err, models.ErrHostCapacityExhausted) {
						continue
					}
					errCh <- fmt.Errorf("AcquireModelLease: %w", err)
					return
				}

				issuedMu.Lock()
				issued = append(issued, acquired.Lease.Lease)
				issuedMu.Unlock()

				if _, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
					Scope: scope,
					Lease: acquired.Lease.Lease,
				}); err != nil {
					errCh <- fmt.Errorf("ReleaseModelLease: %w", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	fillCapacity(t, service, request, capacity)
}

func TestConcurrentCancelDuringAcquireDoesNotConsumeCapacity(t *testing.T) {
	t.Parallel()

	const (
		capacity   = 2
		workers    = 24
		iterations = 30
	)

	scope := mustRuntimeScopeRef(t, "leases-race-cancel")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	var (
		cancelledAttempts atomic.Int64
		successfulLeases  sync.Map
		errCh             = make(chan error, workers*iterations)
		wg                sync.WaitGroup
	)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		holder := fmt.Sprintf("cancel-worker-%d", worker)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				if i%2 == 0 {
					cancel()
				}

				acquired, err := service.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
					Scope:  request.Scope,
					Name:   request.Name,
					Holder: holder,
				})
				cancel()

				if err != nil {
					if errors.Is(err, models.ErrHostCancelled) {
						cancelledAttempts.Add(1)
						continue
					}
					if errors.Is(err, models.ErrHostCapacityExhausted) {
						continue
					}
					errCh <- fmt.Errorf("AcquireModelLease: %w", err)
					return
				}

				leaseID := acquired.Lease.Lease.String()
				if leaseID == "" {
					errCh <- errors.New("cancelled acquire path returned lease without identity")
					return
				}
				if _, loaded := successfulLeases.LoadOrStore(leaseID, struct{}{}); loaded {
					errCh <- fmt.Errorf("duplicate lease identity %q from concurrent acquire", leaseID)
					return
				}

				_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
					Scope: scope,
					Lease: acquired.Lease.Lease,
				})
				if err != nil {
					errCh <- fmt.Errorf("ReleaseModelLease: %w", err)
					return
				}
				successfulLeases.Delete(leaseID)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
	if cancelledAttempts.Load() == 0 {
		t.Fatal("expected cancelled acquire attempts during concurrent stress")
	}

	fillCapacity(t, service, request, capacity)
}

func TestConcurrentExpiryGetAndAcquireReclaimsCapacity(t *testing.T) {
	t.Parallel()

	const capacity = 3

	start := time.Date(2026, time.July, 27, 14, 0, 0, 0, time.UTC)
	clock := &mutexAdvanceableHostClock{now: start}
	scope := mustRuntimeScopeRef(t, "leases-race-expiry")
	service := internalservice.New(clock, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	held := make([]models.ModelLeaseRef, 0, capacity)
	for holder := 0; holder < capacity; holder++ {
		acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
			Scope:  request.Scope,
			Name:   request.Name,
			Holder: fmt.Sprintf("seed-%d", holder),
		})
		if err != nil {
			t.Fatalf("seed acquire %d: %v", holder, err)
		}
		held = append(held, acquired.Lease.Lease)
	}

	clock.advance(hostleases.DefaultLeaseTTL)

	var wg sync.WaitGroup
	errCh := make(chan error, capacity*2)
	for _, lease := range held {
		wg.Add(1)
		go func(lease models.ModelLeaseRef) {
			defer wg.Done()
			_, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
				Scope: scope,
				Lease: lease,
			})
			if !errors.Is(err, models.ErrHostLeaseExpired) {
				errCh <- fmt.Errorf("GetModelLease %s = %v, want ErrHostLeaseExpired", lease, err)
			}
		}(lease)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		fillCapacity(t, service, request, capacity)
	}()

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}
}

func TestConcurrentAcquireDuringLazyExpiryHonoursCapacity(t *testing.T) {
	t.Parallel()

	const (
		capacity   = 2
		workers    = 10
		iterations = 30
	)

	start := time.Date(2026, time.July, 27, 15, 0, 0, 0, time.UTC)
	clock := &mutexAdvanceableHostClock{now: start}
	scope := mustRuntimeScopeRef(t, "leases-race-lazy-expiry")
	service := internalservice.New(clock, readySlotFacts{capacity: capacity})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	var (
		seenIDs sync.Map
		errCh   = make(chan error, workers*iterations)
		wg      sync.WaitGroup
	)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		holder := fmt.Sprintf("lazy-expiry-%d", worker)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if i%5 == 0 {
					clock.advance(hostleases.DefaultLeaseTTL / 2)
				}

				acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
					Scope:  request.Scope,
					Name:   request.Name,
					Holder: holder,
				})
				if err != nil {
					if errors.Is(err, models.ErrHostCapacityExhausted) {
						continue
					}
					errCh <- fmt.Errorf("AcquireModelLease: %w", err)
					return
				}

				leaseID := acquired.Lease.Lease.String()
				if _, loaded := seenIDs.LoadOrStore(leaseID, struct{}{}); loaded {
					errCh <- fmt.Errorf("duplicate lease identity %q", leaseID)
					return
				}

				if i%3 == 0 {
					clock.advance(hostleases.DefaultLeaseTTL)
				}

				_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
					Scope: scope,
					Lease: acquired.Lease.Lease,
				})
				if err != nil &&
					!errors.Is(err, models.ErrHostLeaseExpired) {
					errCh <- fmt.Errorf("ReleaseModelLease: %w", err)
					return
				}

				seenIDs.Delete(leaseID)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	fillCapacity(t, service, request, capacity)
}

func fillCapacity(
	t *testing.T,
	service hostleases.Service,
	request models.AcquireModelLeaseRequest,
	capacity int,
) {
	t.Helper()

	acquired := make([]models.ModelLeaseRef, 0, capacity)
	for holder := 0; holder < capacity; holder++ {
		result, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
			Scope:  request.Scope,
			Name:   request.Name,
			Holder: fmt.Sprintf("fill-%d", holder),
		})
		if err != nil {
			t.Fatalf("fill capacity slot %d/%d: %v", holder+1, capacity, err)
		}
		acquired = append(acquired, result.Lease.Lease)
	}

	_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "fill-overflow",
	})
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("overflow acquire = %v, want ErrHostCapacityExhausted", err)
	}

	for _, lease := range acquired {
		if _, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
			Scope: request.Scope,
			Lease: lease,
		}); err != nil {
			t.Fatalf("release fill lease: %v", err)
		}
	}
}

type mutexAdvanceableHostClock struct {
	mu  sync.Mutex
	now time.Time
}

func (clock *mutexAdvanceableHostClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *mutexAdvanceableHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during leases concurrency tests")
}

func (clock *mutexAdvanceableHostClock) advance(delta time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(delta)
}
