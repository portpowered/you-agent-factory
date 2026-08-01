package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
)

type service struct {
	hostClock       modelseffects.HostClock
	slotFacts       modelseffects.SlotFactsProvider
	coordinator     modelseffects.SlotCapacityCoordinator
	mu              sync.Mutex
	leases          map[string]leaseRecord
	capacityHolders map[string]int
	nextLease       int
}

type leaseRecord struct {
	lease models.ModelLease
}

var _ hostleases.Service = (*service)(nil)
var _ modelseffects.CoordinatorBindable = (*service)(nil)

// New constructs an inert leases owner that retains injected effects and
// allocates lease/capacity state without launching subprocesses or timers.
func New(
	hostClock modelseffects.HostClock,
	slotFacts modelseffects.SlotFactsProvider,
) hostleases.Service {
	return &service{
		hostClock:       hostClock,
		slotFacts:       slotFacts,
		leases:          make(map[string]leaseRecord),
		capacityHolders: make(map[string]int),
	}
}

func (s *service) BindSlotCapacityCoordinator(
	coordinator modelseffects.SlotCapacityCoordinator,
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
	now := s.hostClock.Now()
	expiredReleases := s.expireStaleLeasesLocked(slotKey, now)
	if leaseCapacityExhausted(s.capacityHolders[slotKey], facts.Capacity) {
		s.mu.Unlock()
		s.notifyCapacityReleased(expiredReleases)
		return models.AcquireModelLeaseResult{}, models.ErrHostCapacityExhausted
	}
	requestHolder := strings.TrimSpace(request.Holder)
	if contended := strings.TrimSpace(facts.ContendedHolder); contended != "" &&
		!strings.EqualFold(contended, requestHolder) {
		s.mu.Unlock()
		s.notifyCapacityReleased(expiredReleases)
		return models.AcquireModelLeaseResult{}, models.ErrHostCapacityContended
	}

	s.nextLease++
	ref, err := (models.ModelLeaseRef{}).Parse(fmt.Sprintf("model-lease-%d", s.nextLease))
	if err != nil {
		s.mu.Unlock()
		return models.AcquireModelLeaseResult{}, err
	}
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

	s.notifyCapacityReleased(expiredReleases)
	if s.coordinator != nil {
		s.coordinator.OnLeaseCapacityAcquired(request.Scope, request.Name)
	}

	return models.AcquireModelLeaseResult{Lease: lease}, nil
}

func (s *service) GetModelLease(
	ctx context.Context,
	request models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelLeaseResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.GetModelLeaseResult{}, err
	}

	leaseKey := request.Lease.String()
	s.mu.Lock()
	record, ok := s.leases[leaseKey]
	if !ok || record.lease.Scope != request.Scope {
		s.mu.Unlock()
		return models.GetModelLeaseResult{}, models.ErrHostLeaseNotFound
	}
	slotKey := leaseSlotKey(request.Scope, record.lease.ModelName)
	now := s.hostClock.Now()
	expiredReleases := s.expireStaleLeasesLocked(slotKey, now)
	record = s.leases[leaseKey]
	if record.lease.Status == models.ModelLeaseStatusActive && leaseExpired(record.lease, now) {
		record = s.expireLeaseRecordLocked(leaseKey, record, slotKey)
		s.mu.Unlock()
		s.notifyCapacityReleased(append(expiredReleases, capacityRelease{
			scope:     request.Scope,
			modelName: record.lease.ModelName,
		}))
		return models.GetModelLeaseResult{Lease: record.lease}, models.ErrHostLeaseExpired
	}
	if record.lease.Status == models.ModelLeaseStatusExpired {
		s.mu.Unlock()
		s.notifyCapacityReleased(expiredReleases)
		return models.GetModelLeaseResult{Lease: record.lease}, models.ErrHostLeaseExpired
	}
	s.mu.Unlock()
	s.notifyCapacityReleased(expiredReleases)
	return models.GetModelLeaseResult{Lease: record.lease}, nil
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
	slotKey := leaseSlotKey(request.Scope, record.lease.ModelName)
	now := s.hostClock.Now()
	expiredReleases := s.expireStaleLeasesLocked(slotKey, now)
	record = s.leases[leaseKey]
	if record.lease.Status == models.ModelLeaseStatusActive && leaseExpired(record.lease, now) {
		record = s.expireLeaseRecordLocked(leaseKey, record, slotKey)
		s.mu.Unlock()
		s.notifyCapacityReleased(append(expiredReleases, capacityRelease{
			scope:     request.Scope,
			modelName: record.lease.ModelName,
		}))
		return models.ReleaseModelLeaseResult{Lease: record.lease}, models.ErrHostLeaseExpired
	}
	if record.lease.Status == models.ModelLeaseStatusExpired {
		s.mu.Unlock()
		s.notifyCapacityReleased(expiredReleases)
		return models.ReleaseModelLeaseResult{Lease: record.lease}, models.ErrHostLeaseExpired
	}
	if record.lease.Status != models.ModelLeaseStatusActive {
		s.mu.Unlock()
		s.notifyCapacityReleased(expiredReleases)
		return models.ReleaseModelLeaseResult{}, models.ErrHostLeaseNotFound
	}

	record.lease.Status = models.ModelLeaseStatusReleased
	s.leases[leaseKey] = record
	modelName := record.lease.ModelName
	s.releaseCapacityCountLocked(slotKey)
	s.mu.Unlock()

	s.notifyCapacityReleased(expiredReleases)
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

type capacityRelease struct {
	scope     models.RuntimeScopeRef
	modelName string
}

func (s *service) notifyCapacityReleased(releases []capacityRelease) {
	if s.coordinator == nil {
		return
	}
	for _, release := range releases {
		s.coordinator.OnLeaseCapacityReleased(release.scope, release.modelName)
	}
}

func leaseExpired(lease models.ModelLease, now time.Time) bool {
	return lease.Status == models.ModelLeaseStatusActive && !now.Before(lease.ExpiresAt)
}

func (s *service) expireStaleLeasesLocked(
	slotKey string,
	now time.Time,
) []capacityRelease {
	releases := make([]capacityRelease, 0)
	for leaseKey, record := range s.leases {
		if leaseSlotKey(record.lease.Scope, record.lease.ModelName) != slotKey {
			continue
		}
		if record.lease.Status != models.ModelLeaseStatusActive || !leaseExpired(record.lease, now) {
			continue
		}
		record = s.expireLeaseRecordLocked(leaseKey, record, slotKey)
		releases = append(releases, capacityRelease{
			scope:     record.lease.Scope,
			modelName: record.lease.ModelName,
		})
	}
	return releases
}

func (s *service) expireLeaseRecordLocked(
	leaseKey string,
	record leaseRecord,
	slotKey string,
) leaseRecord {
	record.lease.Status = models.ModelLeaseStatusExpired
	s.leases[leaseKey] = record
	s.releaseCapacityCountLocked(slotKey)
	return record
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
