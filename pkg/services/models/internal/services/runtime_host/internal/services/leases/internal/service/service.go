package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
)

type service struct {
	hostClock       models.HostClock
	slotFacts       hostleases.SlotFactsProvider
	coordinator     hostleases.SlotCapacityCoordinator
	mu              sync.Mutex
	leases          map[string]leaseRecord
	capacityHolders map[string]int
	nextLease       int
}

type leaseRecord struct {
	lease models.ModelLease
}

var _ hostleases.Service = (*service)(nil)
var _ hostleases.CoordinatorBindable = (*service)(nil)

// New constructs an inert leases owner that retains injected effects and
// allocates lease/capacity state without launching subprocesses or timers.
func New(
	hostClock models.HostClock,
	slotFacts hostleases.SlotFactsProvider,
) hostleases.Service {
	return &service{
		hostClock:       hostClock,
		slotFacts:       slotFacts,
		leases:          make(map[string]leaseRecord),
		capacityHolders: make(map[string]int),
	}
}

func (s *service) BindSlotCapacityCoordinator(
	coordinator hostleases.SlotCapacityCoordinator,
) {
	s.coordinator = coordinator
}

func (s *service) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	if err := request.Validate(); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	facts, err := s.slotFacts.SlotFacts(ctx, request.Scope, request.Name)
	if err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	if facts.Readiness != models.ReadinessStateReady {
		return models.AcquireModelLeaseResult{}, models.ErrHostRuntimeNotReady
	}

	slotKey := leaseSlotKey(request.Scope, request.Name)
	s.mu.Lock()
	if leaseCapacityExhausted(s.capacityHolders[slotKey], facts.Capacity) {
		s.mu.Unlock()
		return models.AcquireModelLeaseResult{}, models.ErrHostCapacityExhausted
	}

	s.nextLease++
	ref, err := (models.ModelLeaseRef{}).Parse(fmt.Sprintf("model-lease-%d", s.nextLease))
	if err != nil {
		s.mu.Unlock()
		return models.AcquireModelLeaseResult{}, err
	}
	now := s.hostClock.Now()
	lease := models.ModelLease{
		Lease:         ref,
		Scope:         request.Scope,
		ModelName:     request.Name,
		Holder:        strings.TrimSpace(request.Holder),
		ExpiresAt:     now.Add(hostleases.DefaultLeaseTTL),
		Status:        models.ModelLeaseStatusActive,
		HostReadiness: facts.Readiness,
	}
	s.leases[ref.String()] = leaseRecord{lease: lease}
	s.capacityHolders[slotKey]++
	s.mu.Unlock()

	if s.coordinator != nil {
		s.coordinator.OnLeaseCapacityAcquired(request.Scope, request.Name)
	}

	return models.AcquireModelLeaseResult{Lease: lease}, nil
}

func (s *service) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (s *service) ReleaseModelLease(
	ctx context.Context,
	request models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	if err := request.Validate(); err != nil {
		return models.ReleaseModelLeaseResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.ReleaseModelLeaseResult{}, err
	}

	leaseKey := request.Lease.String()
	s.mu.Lock()
	record, ok := s.leases[leaseKey]
	if !ok || record.lease.Scope != request.Scope {
		s.mu.Unlock()
		return models.ReleaseModelLeaseResult{}, models.ErrHostLeaseNotFound
	}
	if record.lease.Status != models.ModelLeaseStatusActive {
		s.mu.Unlock()
		return models.ReleaseModelLeaseResult{}, models.ErrHostLeaseNotFound
	}

	record.lease.Status = models.ModelLeaseStatusReleased
	s.leases[leaseKey] = record
	slotKey := leaseSlotKey(request.Scope, record.lease.ModelName)
	modelName := record.lease.ModelName
	s.releaseCapacityCountLocked(slotKey)
	s.mu.Unlock()

	if s.coordinator != nil {
		s.coordinator.OnLeaseCapacityReleased(request.Scope, modelName)
	}

	return models.ReleaseModelLeaseResult{
		Lease:   record.lease,
		Outcome: models.ModelLeaseReleased,
	}, nil
}

func (s *service) releaseCapacityCountLocked(slotKey string) {
	count := s.capacityHolders[slotKey]
	if count <= 1 {
		delete(s.capacityHolders, slotKey)
		return
	}
	s.capacityHolders[slotKey] = count - 1
}

func leaseSlotKey(scope models.RuntimeScopeRef, modelName string) string {
	return scope.String() + "|" + strings.ToUpper(strings.TrimSpace(modelName))
}

func leaseCapacityExhausted(activeCount int, capacity int) bool {
	if capacity <= 0 {
		return false
	}
	return activeCount >= capacity
}

func hostContextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return fmt.Errorf(
		"%w: %w",
		models.ErrHostCancelled,
		errors.Join(ctx.Err(), context.Cause(ctx)),
	)
}
