package modelhost

import (
	"context"
	"strings"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

func normalizeLeasePolicy(idleUnloadAfter time.Duration, maxLoadedRuntimes int) (time.Duration, int) {
	if idleUnloadAfter < 0 {
		idleUnloadAfter = 0
	}
	if maxLoadedRuntimes < 0 {
		maxLoadedRuntimes = 0
	}
	return idleUnloadAfter, maxLoadedRuntimes
}

func (h *CatalogHost) leaseCapacityForModel(runtimeCfg *models.RuntimeConfig, modelName string) int {
	resource := modelScopedResource(runtimeCfg, modelName)
	if resource == nil || resource.Capacity <= 0 {
		return 0
	}
	return resource.Capacity
}

func (h *CatalogHost) leaseCapacityExhausted(modelKey string, capacity int) bool {
	if capacity <= 0 {
		return false
	}
	return len(h.byModel[modelKey]) >= capacity
}

func leaseCapacityError(modelName string) error {
	name := strings.TrimSpace(modelName)
	return &ReadinessError{
		Snapshot: ReadinessSnapshot{
			Identity:       Identity{Name: name},
			ReadinessState: managedruntime.ReadinessStateFailed,
			LifecycleState: managedruntime.LifecycleStateLoaded,
			FailureClass:   FailureClassCapacityExhausted,
		},
		Cause: ErrCapacityExhausted,
	}
}

func (h *CatalogHost) cancelIdleUnloadLocked(slotKey string) {
	if timer, ok := h.idleUnloadTimers[slotKey]; ok {
		timer.Stop()
		delete(h.idleUnloadTimers, slotKey)
	}
}

func (h *CatalogHost) scheduleIdleUnloadIfIdle(
	slotKey string,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) {
	if h.idleUnloadAfter <= 0 {
		return
	}
	modelKey := canonicalModelKey(modelName)
	if len(h.byModel[modelKey]) > 0 {
		return
	}
	h.cancelIdleUnloadLocked(slotKey)
	cfg := runtimeCfg
	h.idleUnloadTimers[slotKey] = time.AfterFunc(h.idleUnloadAfter, func() {
		h.runIdleUnload(cfg, modelName, slotKey)
	})
}

func (h *CatalogHost) runIdleUnload(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
	slotKey string,
) {
	h.mu.Lock()
	modelKey := canonicalModelKey(modelName)
	if len(h.byModel[modelKey]) > 0 {
		h.mu.Unlock()
		return
	}
	delete(h.idleUnloadTimers, slotKey)
	h.mu.Unlock()
	identity := Identity{Name: strings.TrimSpace(modelName)}
	if entry, entryErr := h.catalogEntry(runtimeCfg, modelName); entryErr == nil {
		identity = h.identityFromCatalog(runtimeCfg, entry)
	}
	h.diagnostics.logUnload(identity, "idle")
	_ = h.unloadRuntime(ctxWithoutCancel(), runtimeCfg, modelName, slotKey)
}

func ctxWithoutCancel() context.Context {
	return context.Background()
}

func (h *CatalogHost) slotHasActiveLeasesLocked(slotKey string) bool {
	parts := strings.SplitN(slotKey, "|", 2)
	if len(parts) != 2 {
		return false
	}
	modelKey := parts[1]
	return len(h.byModel[modelKey]) > 0
}

func (h *CatalogHost) evictIdleRuntimesForCapacity(
	ctx context.Context,
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) error {
	if h.maxLoadedRuntimes <= 0 {
		return nil
	}
	slotKey := h.runtimeSlotKey(runtimeCfg, modelName)
	h.mu.Lock()
	if _, exists := h.runtimeSlots[slotKey]; exists {
		h.mu.Unlock()
		return nil
	}
	if len(h.runtimeSlots) < h.maxLoadedRuntimes {
		h.mu.Unlock()
		return nil
	}
	evictKey := ""
	for key, slot := range h.runtimeSlots {
		if h.slotHasActiveLeasesLocked(key) {
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
		h.mu.Unlock()
		return leaseCapacityError(modelName)
	}
	slot := h.runtimeSlots[evictKey]
	delete(h.runtimeSlots, evictKey)
	h.cancelIdleUnloadLocked(evictKey)
	h.mu.Unlock()
	evictedModel := modelName
	if parts := strings.SplitN(evictKey, "|", 2); len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		evictedModel = parts[1]
	}
	identity := Identity{Name: strings.TrimSpace(evictedModel)}
	if entry, entryErr := h.catalogEntry(runtimeCfg, evictedModel); entryErr == nil {
		identity = h.identityFromCatalog(runtimeCfg, entry)
	}
	h.diagnostics.logUnload(identity, "pressure_eviction")
	return slot.stop(ctx)
}
