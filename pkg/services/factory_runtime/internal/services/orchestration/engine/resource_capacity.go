package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

// AcquireResourceCapacityAdmission reserves the same boundary used by engine
// ticks. The caller owns the release function until the complete live-change
// admission transaction has finished.
func (e *FactoryEngine) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	if e == nil || e.admissionGate == nil {
		return nil, fmt.Errorf("Factory Runtime admission gate is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.admissionGate:
	}
	var once sync.Once
	return func() {
		once.Do(func() { e.admissionGate <- struct{}{} })
	}, nil
}

// PreviewResourceCapacity evaluates a capacity request under the shared
// admission boundary without mutating the marking.
func (e *FactoryEngine) PreviewResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	release, err := e.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		return factory.ResourceCapacityResult{}, err
	}
	defer release()
	return e.PreviewResourceCapacityAdmitted(ctx, request)
}

// PreviewResourceCapacityAdmitted evaluates a request while the caller owns
// the admission lease.
func (e *FactoryEngine) PreviewResourceCapacityAdmitted(_ context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if e == nil || e.state == nil || e.runtimeState == nil || e.runtimeState.Marking == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource state is unavailable")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.resourceCapacityDecisionLocked(request)
}

// SetResourceCapacity applies a capacity request under the shared admission
// boundary. Idle tokens are removed in stable order and new token identities
// are allocated monotonically so replay and inspection remain deterministic.
func (e *FactoryEngine) SetResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	release, err := e.AcquireResourceCapacityAdmission(ctx)
	if err != nil {
		return factory.ResourceCapacityResult{}, err
	}
	defer release()
	return e.SetResourceCapacityAdmitted(ctx, request)
}

// SetResourceCapacityAdmitted applies a request while the caller owns the
// admission lease.
func (e *FactoryEngine) SetResourceCapacityAdmitted(_ context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if e == nil || e.state == nil || e.runtimeState == nil || e.runtimeState.Marking == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource state is unavailable")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	result, err := e.resourceCapacityDecisionLocked(request)
	if err != nil || result.Outcome == factory.ResourceCapacityOutcomeNoOp {
		return result, err
	}
	resource := e.state.Resources[result.ResourceID]
	placeID := resourceAvailablePlaceID(result.ResourceID)
	available := e.runtimeState.Marking.TokensInPlace(placeID)
	delta := result.RequestedCapacity - result.PreviousCapacity
	if delta < 0 {
		removeCount := -delta
		if len(available) < removeCount {
			return factory.ResourceCapacityResult{}, fmt.Errorf("resource %q available marking is inconsistent", result.ResourceID)
		}
		sort.Slice(available, func(i, j int) bool { return available[i].ID > available[j].ID })
		for _, token := range available[:removeCount] {
			e.runtimeState.Marking.RemoveToken(token.ID)
		}
	}
	if delta > 0 {
		now := e.clock.Now()
		for range delta {
			token := e.nextResourceTokenLocked(result.ResourceID, placeID, now)
			e.runtimeState.Marking.AddToken(token)
		}
	}
	resource.Capacity = result.RequestedCapacity
	result.AvailableCount = len(e.runtimeState.Marking.TokensInPlace(placeID))
	result.InUseCount = resourceInUseCountLocked(e.runtimeState, result.ResourceID)
	result.MinimumCapacity = result.InUseCount
	return result, nil
}

func (e *FactoryEngine) resourceCapacityDecisionLocked(request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	request.ResourceID = strings.TrimSpace(request.ResourceID)
	if request.ResourceID == "" {
		return factory.ResourceCapacityResult{}, &factory.ResourceCapacityError{Code: factory.ResourceCapacityErrorValidation, Message: "resource id is required"}
	}
	resource, ok := e.state.Resources[request.ResourceID]
	if !ok || resource == nil {
		return factory.ResourceCapacityResult{}, &factory.ResourceCapacityError{Code: factory.ResourceCapacityErrorNotFound, ResourceID: request.ResourceID, Message: "resource was not found"}
	}
	availablePlace := resourceAvailablePlaceID(request.ResourceID)
	if e.state.Places[availablePlace] == nil {
		return factory.ResourceCapacityResult{}, &factory.ResourceCapacityError{Code: factory.ResourceCapacityErrorNotFound, ResourceID: request.ResourceID, Message: "resource availability place was not found"}
	}
	available := len(e.runtimeState.Marking.TokensInPlace(availablePlace))
	inUse := resourceInUseCountLocked(e.runtimeState, request.ResourceID)
	result := factory.ResourceCapacityResult{
		ResourceID:        request.ResourceID,
		ResourceName:      resource.Name,
		PreviousCapacity:  resource.Capacity,
		RequestedCapacity: request.RequestedCapacity,
		EffectiveCapacity: resource.Capacity,
		InUseCount:        inUse,
		AvailableCount:    available,
		MinimumCapacity:   inUse,
	}
	if request.RequestedCapacity < 0 {
		return result, &factory.ResourceCapacityError{
			Code: factory.ResourceCapacityErrorValidation, ResourceID: request.ResourceID,
			CurrentCapacity: resource.Capacity, RequestedCapacity: request.RequestedCapacity,
			InUseCount: inUse, AvailableCount: available, MinimumCapacity: inUse,
			Message: "requested capacity must not be negative",
		}
	}
	if request.RequestedCapacity < inUse {
		return result, &factory.ResourceCapacityError{
			Code: factory.ResourceCapacityErrorCapacityInUse, ResourceID: request.ResourceID,
			CurrentCapacity: resource.Capacity, RequestedCapacity: request.RequestedCapacity,
			InUseCount: inUse, AvailableCount: available, MinimumCapacity: inUse,
		}
	}
	result.EffectiveCapacity = request.RequestedCapacity
	if request.RequestedCapacity == resource.Capacity {
		result.Outcome = factory.ResourceCapacityOutcomeNoOp
		return result, nil
	}
	result.Outcome = factory.ResourceCapacityOutcomeApplied
	return result, nil
}

func resourceAvailablePlaceID(resourceID string) string {
	return resourceID + ":" + interfaces.ResourceStateAvailable
}

func resourceInUseCountLocked(runtimeState *RuntimeState, resourceID string) int {
	seen := make(map[string]struct{})
	inUse := 0
	for tokenID, token := range runtimeState.Marking.Tokens {
		if token == nil || token.Color.DataType != factorytoken.DataTypeResource || resourceIDForToken(*token) != resourceID {
			continue
		}
		if token.PlaceID == resourceAvailablePlaceID(resourceID) {
			continue
		}
		seen[tokenID] = struct{}{}
		inUse++
	}
	for _, dispatch := range runtimeState.Dispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.DataType != factorytoken.DataTypeResource || resourceIDForToken(token) != resourceID {
				continue
			}
			if _, exists := seen[token.ID]; exists {
				continue
			}
			seen[token.ID] = struct{}{}
			inUse++
		}
	}
	return inUse
}

func resourceIDForToken(token factorytoken.Token) string {
	if strings.TrimSpace(token.Color.WorkTypeID) != "" {
		return strings.TrimSpace(token.Color.WorkTypeID)
	}
	return strings.TrimSuffix(token.PlaceID, ":"+interfaces.ResourceStateAvailable)
}

func (e *FactoryEngine) nextResourceTokenLocked(resourceID, placeID string, now time.Time) *factorytoken.Token {
	for index := 0; ; index++ {
		tokenID := fmt.Sprintf("%s:resource:%d", resourceID, index)
		if _, exists := e.runtimeState.Marking.Tokens[tokenID]; exists {
			continue
		}
		return &factorytoken.Token{
			ID:      tokenID,
			PlaceID: placeID,
			Color: factorytoken.Color{
				WorkID:     fmt.Sprintf("%s:%d", resourceID, index),
				WorkTypeID: resourceID,
				DataType:   factorytoken.DataTypeResource,
			},
			CreatedAt: now,
			EnteredAt: now,
			History:   factorytoken.History{},
		}
	}
}
