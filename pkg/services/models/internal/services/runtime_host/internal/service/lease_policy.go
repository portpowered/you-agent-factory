package service

import (
	"context"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type slotCoordinator struct {
	*SlotState
	scopes          runtimescopes.Service
	hostClock       modelseffects.HostClock
	diagnostics     hostDiagnostics
	idleUnloadAfter time.Duration
}

var _ modelseffects.SlotCapacityCoordinator = (*slotCoordinator)(nil)

func NewSlotCoordinator(state *SlotState, scopes runtimescopes.Service, clock modelseffects.HostClock, logger modelseffects.HostDiagnosticLogger, metrics modelseffects.HostMetricsRecorder, idleUnloadAfter time.Duration) modelseffects.SlotCapacityCoordinator {
	idleUnloadAfter, _ = normalizeHostPolicy(idleUnloadAfter, 0)
	return &slotCoordinator{
		SlotState: state, scopes: scopes, hostClock: clock,
		diagnostics:     hostDiagnostics{logger: logger, metrics: metrics},
		idleUnloadAfter: idleUnloadAfter,
	}
}

// OnLeaseCapacityAcquired records one active holder for idle-unload policy.
func (s *slotCoordinator) OnLeaseCapacityAcquired(
	scope models.RuntimeScopeRef,
	modelName string,
) {
	slotKey := runtimeSlotKey(scope, modelName)
	s.mu.Lock()
	s.acquireSlotCapacityLocked(slotKey)
	s.mu.Unlock()
}

// OnLeaseCapacityReleased frees one holder and may schedule idle unload.
func (s *slotCoordinator) OnLeaseCapacityReleased(
	scope models.RuntimeScopeRef,
	modelName string,
) {
	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return
	}
	s.releaseSlotCapacityWithOverlays(scope, modelName, binding.RuntimeConfig(), binding.OperatorModels)
}

func (s *slotCoordinator) releaseSlotCapacityWithOverlays(
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
	overlays map[string]models.ModelOverlay,
) {
	slotKey := runtimeSlotKey(scope, modelName)
	identity := supervisedIdentityForModel(runtimeCfg, overlays, modelName)

	s.mu.Lock()
	s.releaseSlotCapacityLocked(slotKey)
	s.scheduleIdleUnloadIfIdle(slotKey, identity)
	s.mu.Unlock()
}

func normalizeHostPolicy(idleUnloadAfter time.Duration, maxLoadedRuntimes int) (time.Duration, int) {
	if idleUnloadAfter < 0 {
		idleUnloadAfter = 0
	}
	if maxLoadedRuntimes < 0 {
		maxLoadedRuntimes = 0
	}
	return idleUnloadAfter, maxLoadedRuntimes
}

func (s *SlotState) slotHasActiveHoldersLocked(slotKey string) bool {
	return s.capacityHolders[slotKey] > 0
}

func (s *SlotState) acquireSlotCapacityLocked(slotKey string) {
	s.capacityHolders[slotKey]++
	s.cancelIdleUnloadLocked(slotKey)
}

func (s *SlotState) releaseSlotCapacityLocked(slotKey string) {
	count := s.capacityHolders[slotKey]
	if count <= 1 {
		delete(s.capacityHolders, slotKey)
		return
	}
	s.capacityHolders[slotKey] = count - 1
}

func (s *SlotState) cancelIdleUnloadLocked(slotKey string) {
	if timer, ok := s.idleUnloadTimers[slotKey]; ok {
		if timer != nil && timer.timer != nil {
			timer.timer.Stop()
		}
		if timer != nil && timer.cancel != nil {
			close(timer.cancel)
		}
		delete(s.idleUnloadTimers, slotKey)
	}
}

func (s *slotCoordinator) scheduleIdleUnloadIfIdle(
	slotKey string,
	identity supervisedIdentity,
) {
	if s.idleUnloadAfter <= 0 {
		return
	}
	if identity.LoadPolicy == string(models.LoadPolicyKeepWarm) {
		return
	}
	if s.slotHasActiveHoldersLocked(slotKey) {
		return
	}
	s.cancelIdleUnloadLocked(slotKey)
	identityCopy := identity
	timer := s.hostClock.NewTimer(s.idleUnloadAfter)
	if timer == nil {
		return
	}
	entry := &idleUnload{timer: timer, cancel: make(chan struct{})}
	s.idleUnloadTimers[slotKey] = entry
	go s.awaitIdleUnload(identityCopy, slotKey, entry)
}

func (s *slotCoordinator) awaitIdleUnload(
	identity supervisedIdentity,
	slotKey string,
	entry *idleUnload,
) {
	select {
	case <-entry.timer.C():
		s.runIdleUnload(identity, slotKey, entry)
	case <-entry.cancel:
	}
}

func (s *slotCoordinator) runIdleUnload(
	identity supervisedIdentity,
	slotKey string,
	entry *idleUnload,
) {
	s.mu.Lock()
	if current := s.idleUnloadTimers[slotKey]; current != entry {
		s.mu.Unlock()
		return
	}
	if s.slotHasActiveHoldersLocked(slotKey) {
		delete(s.idleUnloadTimers, slotKey)
		s.mu.Unlock()
		return
	}
	delete(s.idleUnloadTimers, slotKey)
	slot := s.runtimeSlots[slotKey]
	delete(s.runtimeSlots, slotKey)
	s.mu.Unlock()

	s.diagnostics.logUnload(identity, modelseffects.RuntimeCorrelation(ctxWithoutCancel()), "idle")
	if slot != nil {
		_ = slot.stop(ctxWithoutCancel())
	}
}

func ctxWithoutCancel() context.Context {
	return context.Background()
}

func (s *service) evictIdleRuntimesForCapacity(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	modelName string,
	identity supervisedIdentity,
) error {
	if s.maxLoadedRuntimes <= 0 {
		return nil
	}
	slotKey := runtimeSlotKey(scope, modelName)
	s.mu.Lock()
	if _, exists := s.runtimeSlots[slotKey]; exists {
		s.mu.Unlock()
		return nil
	}
	if len(s.runtimeSlots) < s.maxLoadedRuntimes {
		s.mu.Unlock()
		return nil
	}
	evictKey := ""
	for key, slot := range s.runtimeSlots {
		if s.slotHasActiveHoldersLocked(key) {
			continue
		}
		if slot.isLoading() {
			continue
		}
		if !slot.isResident() {
			continue
		}
		evictKey = key
		break
	}
	if evictKey == "" {
		s.mu.Unlock()
		return capacityExhaustedError(modelName)
	}
	slot := s.runtimeSlots[evictKey]
	delete(s.runtimeSlots, evictKey)
	s.cancelIdleUnloadLocked(evictKey)
	s.mu.Unlock()

	evictedModel := modelName
	if parts := strings.SplitN(evictKey, "|", 2); len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		evictedModel = parts[1]
	}
	evictedIdentity := supervisedIdentity{
		Name:    strings.TrimSpace(evictedModel),
		Backend: identity.Backend,
	}
	diagnostics := s.supervisor.Diagnostics
	diagnostics.logUnload(
		evictedIdentity, modelseffects.RuntimeCorrelation(ctx), "pressure_eviction",
	)
	return slot.stop(ctx)
}

func capacityExhaustedError(modelName string) error {
	name := strings.TrimSpace(modelName)
	return &models.HostReadinessError{
		Snapshot: models.HostReadinessSnapshot{
			Identity:       models.HostIdentity{Name: name},
			ReadinessState: models.ReadinessStateFailed,
			LifecycleState: models.LifecycleStateLoaded,
			FailureClass:   models.HostFailureClassCapacityExhausted,
		},
		Cause: models.ErrHostCapacityExhausted,
	}
}

func loadingCapacityExhaustedError(modelName string) error {
	name := strings.TrimSpace(modelName)
	return &models.HostReadinessError{
		Snapshot: models.HostReadinessSnapshot{
			Identity:       models.HostIdentity{Name: name},
			ReadinessState: models.ReadinessStateLoading,
			LifecycleState: models.LifecycleStateLoading,
			FailureClass:   models.HostFailureClassCapacityExhausted,
		},
		Cause: models.ErrHostCapacityExhausted,
	}
}
