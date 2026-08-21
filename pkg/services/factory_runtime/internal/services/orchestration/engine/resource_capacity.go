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

type resourceCapacityLease struct {
	resourceID string
	token      factorytoken.Token
}

// WakeForResourceCapacity signals the engine after a resource pool changes.
// Capacity changes are a wake source even when no submission or worker result
// is buffered: a waiting transition may become enabled solely because an idle
// resource token was added.
func (e *FactoryEngine) WakeForResourceCapacity() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.signalResourceCapacityChangedLocked()
}

func (e *FactoryEngine) wakeForPendingProcessing() {
	if !e.hasBufferedInputs() {
		return
	}
	select {
	case e.submitSignal <- struct{}{}:
	default:
	}
	if hook, ok := e.dispatchHook.(factory.DispatchResultHookWakeSignaler); ok && hook.HasBufferedResults() {
		hook.SignalBufferedResults()
	}
}

func (e *FactoryEngine) hasBufferedInputs() bool {
	if e.capacityWakePending {
		return true
	}
	if e.submissionHook != nil && len(e.submissionHook.batches) > 0 {
		return true
	}
	buffer := e.runtimeState.ResultBuffer
	if buffer != nil && buffer.HasData() {
		return true
	}
	if e.dispatchHook != nil && e.dispatchHook.HasPendingResults() {
		return true
	}
	return false
}

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

// AcquireResourceCapacityLease waits for one available unit while sharing the
// same admission gate as ticks and live capacity mutation. Waiting happens
// outside the gate; a capacity increase or another lease release closes the
// current notification channel and lets all eligible waiters re-check the
// canonical marking.
func (e *FactoryEngine) AcquireResourceCapacityLease(
	ctx context.Context,
	request factory.ResourceCapacityLeaseRequest,
) (*factory.ResourceCapacityLease, error) {
	if e == nil || e.admissionGate == nil {
		return nil, fmt.Errorf("Factory Runtime resource lease admission is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	request.ResourceID = strings.TrimSpace(request.ResourceID)
	if request.ResourceID == "" {
		return nil, &factory.ResourceCapacityError{
			Code:    factory.ResourceCapacityErrorValidation,
			Message: "resource id is required",
		}
	}

	for {
		admissionRelease, err := e.AcquireResourceCapacityAdmission(ctx)
		if err != nil {
			return nil, err
		}
		leaseID, revision, changed, reserveErr := e.reserveResourceCapacityLease(request.ResourceID)
		admissionRelease()
		if reserveErr != nil {
			return nil, reserveErr
		}
		if leaseID != "" {
			return factory.NewResourceCapacityLease(request.ResourceID, revision, func() {
				e.releaseResourceCapacityLease(leaseID)
			}), nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// reserveResourceCapacityLease consumes the first available resource token in
// stable ID order. The caller must hold the engine admission gate; this helper
// takes the engine state lock and returns the current notification channel when
// no unit is available.
func (e *FactoryEngine) reserveResourceCapacityLease(
	resourceID string,
) (leaseID string, revision int, changed <-chan struct{}, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state == nil || e.runtimeState == nil || e.runtimeState.Marking == nil {
		return "", e.factoryRevision, nil, fmt.Errorf("Factory Runtime resource state is unavailable")
	}
	resource, ok := e.state.Resources[resourceID]
	if !ok || resource == nil {
		return "", e.factoryRevision, nil, &factory.ResourceCapacityError{
			Code:       factory.ResourceCapacityErrorNotFound,
			ResourceID: resourceID,
			Message:    "resource was not found",
		}
	}
	placeID := resourceAvailablePlaceID(resourceID)
	if e.state.Places[placeID] == nil {
		return "", e.factoryRevision, nil, &factory.ResourceCapacityError{
			Code:       factory.ResourceCapacityErrorNotFound,
			ResourceID: resourceID,
			Message:    "resource availability place was not found",
		}
	}
	available := e.runtimeState.Marking.TokensInPlace(placeID)
	if len(available) == 0 {
		if e.capacityChanged == nil {
			e.capacityChanged = make(chan struct{})
		}
		return "", e.factoryRevision, e.capacityChanged, nil
	}
	sort.Slice(available, func(i, j int) bool { return available[i].ID < available[j].ID })
	token := available[0]
	e.runtimeState.Marking.RemoveToken(token.ID)
	e.nextResourceLeaseID++
	leaseID = fmt.Sprintf("resource-lease-%d", e.nextResourceLeaseID)
	e.resourceLeases[leaseID] = resourceCapacityLease{resourceID: resourceID, token: token}
	e.publishRuntimeSnapshotLocked()
	return leaseID, e.factoryRevision, nil, nil
}

// releaseResourceCapacityLease returns an admitted token only after it has
// reacquired the shared admission gate. This is the same serialization point
// used by live reductions, so a release can never race a capacity decision.
func (e *FactoryEngine) releaseResourceCapacityLease(leaseID string) {
	if e == nil {
		return
	}
	releaseAdmission, err := e.AcquireResourceCapacityAdmission(context.Background())
	if err != nil {
		return
	}
	defer releaseAdmission()

	e.mu.Lock()
	defer e.mu.Unlock()
	lease, ok := e.resourceLeases[leaseID]
	if !ok {
		return
	}
	delete(e.resourceLeases, leaseID)
	if e.runtimeState == nil || e.runtimeState.Marking == nil {
		return
	}
	token := lease.token
	token.PlaceID = resourceAvailablePlaceID(lease.resourceID)
	e.runtimeState.Marking.AddToken(&token)
	e.publishRuntimeSnapshotLocked()
	e.signalResourceCapacityChangedLocked()
}

// CurrentFactoryRevision returns the session-effective revision captured by
// the runtime admission boundary.
func (e *FactoryEngine) CurrentFactoryRevision() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.factoryRevision
}

// SetFactoryRevision advances the runtime's detached revision watermark. The
// Factory Sessions coordinator calls this only after the canonical success
// event closes, so an event-append failure cannot publish a false revision.
func (e *FactoryEngine) SetFactoryRevision(revision int) {
	if e == nil || revision < 0 {
		return
	}
	e.mu.Lock()
	if revision > e.factoryRevision {
		e.factoryRevision = revision
	}
	e.mu.Unlock()
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
	result.InUseCount = e.resourceInUseCountLocked(result.ResourceID)
	result.MinimumCapacity = result.InUseCount
	e.publishRuntimeSnapshotLocked()
	e.signalResourceCapacityChangedLocked()
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
	inUse := e.resourceInUseCountLocked(request.ResourceID)
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

func (e *FactoryEngine) resourceInUseCountLocked(resourceID string) int {
	inUse := resourceInUseCountLocked(e.runtimeState, resourceID)
	for _, lease := range e.resourceLeases {
		if lease.resourceID == resourceID {
			inUse++
		}
	}
	return inUse
}

func (e *FactoryEngine) signalResourceCapacityChangedLocked() {
	if e.capacityChanged == nil {
		e.capacityChanged = make(chan struct{})
	}
	close(e.capacityChanged)
	e.capacityChanged = make(chan struct{})
	e.capacityWakePending = true
	select {
	case e.submitSignal <- struct{}{}:
	default:
	}
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
			if token.Color.DataType != factorytoken.DataTypeResource || resourceIDForToken(factorytoken.FromWorker(token)) != resourceID {
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
		if e.resourceTokenIDInUseLocked(resourceID, tokenID) {
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

func (e *FactoryEngine) resourceTokenIDInUseLocked(resourceID, tokenID string) bool {
	if _, exists := e.runtimeState.Marking.Tokens[tokenID]; exists {
		return true
	}
	for _, lease := range e.resourceLeases {
		if lease.resourceID == resourceID && lease.token.ID == tokenID {
			return true
		}
	}
	for _, dispatch := range e.runtimeState.Dispatches {
		if dispatch == nil {
			continue
		}
		for _, token := range dispatch.ConsumedTokens {
			if token.ID == tokenID {
				return true
			}
		}
	}
	return false
}
