package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func validObservationScope(scope factory.ObservationScope) bool {
	switch scope {
	case "", factory.ObservationScopeFull, factory.ObservationScopeStatus, factory.ObservationScopeProgress,
		factory.ObservationScopeDispatches, factory.ObservationScopeResults, factory.ObservationScopeResources,
		factory.ObservationScopeHealth:
		return true
	default:
		return false
	}
}

// recordRestoredWorkRequests carries the detached durable board into the new
// process recording. Runtime ledgers are intentionally process-scoped, so a
// restart recording must contain the request facts again before it can record
// later dispatch or Work-state events without losing the board on the next
// restart.
func recordRestoredWorkRequests(cfg *runtimeConfig, eventHistory recordings.RuntimeLedger) {
	if cfg == nil || cfg.restoredWorldState == nil || eventHistory == nil {
		return
	}
	restored := cfg.restoredWorldState
	if len(restored.WorkRequestsByID) == 0 {
		return
	}

	items := restoredWorkItems(restored)
	existingRequests := existingRestoredRequestIDs(eventHistory.CanonicalEvents())
	for _, requestKey := range sortedRestoredRequestKeys(restored) {
		recordRestoredWorkRequest(cfg, eventHistory, restored, items, requestKey, existingRequests)
	}
}

func sortedRestoredRequestKeys(restored *interfaces.FactoryWorldState) []string {
	requestKeys := make([]string, 0, len(restored.WorkRequestsByID))
	for requestKey := range restored.WorkRequestsByID {
		requestKeys = append(requestKeys, requestKey)
	}
	sort.Strings(requestKeys)
	return requestKeys
}

func existingRestoredRequestIDs(events []interfaces.FactoryEvent) map[string]struct{} {
	existingRequests := make(map[string]struct{})
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeWorkRequest || event.Context.RequestID == nil {
			continue
		}
		if requestID := strings.TrimSpace(*event.Context.RequestID); requestID != "" {
			existingRequests[requestID] = struct{}{}
		}
	}
	return existingRequests
}

func recordRestoredWorkRequest(
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	restored *interfaces.FactoryWorldState,
	items map[string]work.FactoryWorkItem,
	requestKey string,
	existingRequests map[string]struct{},
) {
	request := restored.WorkRequestsByID[requestKey]
	requestID := strings.TrimSpace(request.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(requestKey)
	}
	if requestID == "" {
		return
	}
	if _, exists := existingRequests[requestID]; exists {
		return
	}

	workItems := restoredWorkItemsForRequest(request, items)
	if len(workItems) == 0 {
		return
	}
	eventHistory.RecordWorkRequest(cfg.restoredWorldState.Tick, work.WorkRequestRecord{
		RequestID:     requestID,
		Type:          request.Type,
		TraceID:       request.TraceID,
		Source:        request.Source,
		ParentLineage: append([]string(nil), request.ParentLineage...),
		WorkItems:     workItems,
		Relations:     restoredRelationsForRequest(restored, requestID, workItems),
	}, cfg.clock.Now().UTC())
	existingRequests[requestID] = struct{}{}
}

func restoredWorkItemsForRequest(
	request interfaces.WorkRequestPayload,
	items map[string]work.FactoryWorkItem,
) []work.FactoryWorkItem {
	workItems := make([]work.FactoryWorkItem, 0, len(request.WorkItems))
	for _, requestedItem := range request.WorkItems {
		workID := strings.TrimSpace(requestedItem.ID)
		if workID == "" {
			continue
		}
		item, ok := items[workID]
		if !ok {
			item = requestedItem
		}
		if item.ID == "" {
			item.ID = workID
		}
		if item.TraceID == "" {
			item.TraceID = request.TraceID
		}
		workItems = append(workItems, item)
	}
	return workItems
}

func restoredRelationsForRequest(
	restored *interfaces.FactoryWorldState,
	requestID string,
	workItems []work.FactoryWorkItem,
) []work.FactoryRelation {
	if restored == nil || len(restored.RelationsByWorkID) == 0 {
		return nil
	}
	requestWorkIDs := make(map[string]struct{}, len(workItems))
	for _, item := range workItems {
		if item.ID != "" {
			requestWorkIDs[item.ID] = struct{}{}
		}
	}
	sourceIDs := make([]string, 0, len(restored.RelationsByWorkID))
	for sourceID := range restored.RelationsByWorkID {
		sourceIDs = append(sourceIDs, sourceID)
	}
	sort.Strings(sourceIDs)

	seen := make(map[string]struct{})
	result := make([]work.FactoryRelation, 0)
	for _, sourceID := range sourceIDs {
		if _, belongs := requestWorkIDs[sourceID]; !belongs {
			continue
		}
		for _, relation := range restored.RelationsByWorkID[sourceID] {
			if relation.TargetWorkID == "" ||
				(relation.RequestID != "" && relation.RequestID != requestID) {
				continue
			}
			if relation.SourceWorkID == "" {
				relation.SourceWorkID = sourceID
			}
			if relation.RequestID == "" {
				relation.RequestID = requestID
			}
			key := strings.Join([]string{
				relation.Type,
				relation.SourceWorkID,
				relation.TargetWorkID,
				relation.RequiredState,
				relation.RequestID,
			}, "\x00")
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, relation)
		}
	}
	return result
}

func restoredDispatchHasRequest(events []interfaces.FactoryEvent, dispatchID string) bool {
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeDispatchRequest &&
			stringPointerValue(event.Context.DispatchID) == dispatchID {
			return true
		}
	}
	return false
}

func restoredDispatchRequestEvent(
	cfg *runtimeConfig,
	dispatch interfaces.FactoryWorldDispatch,
) (interfaces.FactoryEvent, error) {
	dispatchID := strings.TrimSpace(dispatch.DispatchID)
	transitionID := strings.TrimSpace(dispatch.TransitionID)
	if dispatchID == "" {
		return interfaces.FactoryEvent{}, fmt.Errorf("restored active dispatch has no dispatch ID")
	}
	if transitionID == "" {
		return interfaces.FactoryEvent{}, fmt.Errorf("restored active dispatch %q has no transition ID", dispatchID)
	}
	workIDs := restoredDispatchWorkIDs(dispatch)
	traceIDs := uniqueNonEmptyStrings(dispatch.TraceIDs)
	currentTraceID := strings.TrimSpace(dispatch.CurrentChainingTraceID)
	previousTraceIDs := uniqueNonEmptyStrings(dispatch.PreviousChainingTraceIDs)
	payload := interfaces.DispatchRequestEventPayload{
		TransitionID:             transitionID,
		Inputs:                   restoredDispatchInputRefs(dispatch),
		ExpectedArtifactContext:  dispatch.ExpectedArtifactContext.Clone(),
		CurrentChainingTraceID:   stringPointerIfNonEmpty(currentTraceID),
		PreviousChainingTraceIDs: stringSlicePointer(previousTraceIDs),
		Resources:                restoredDispatchResourceRefs(dispatch.Resources),
	}
	if dispatch.RunnerID != "" || dispatch.RunnerSelectionSource != "" {
		metadata := &interfaces.DispatchRequestEventMetadata{
			RunnerID: stringPointerIfNonEmpty(dispatch.RunnerID),
		}
		if dispatch.RunnerSelectionSource != "" {
			source := dispatch.RunnerSelectionSource
			metadata.RunnerSelectionSource = &source
		}
		payload.Metadata = metadata
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("encode restored dispatch request for %q: %w", dispatchID, err)
	}
	now := cfg.clock.Now().UTC()
	startedAt := dispatch.StartedAt.UTC()
	if dispatch.StartedAt.IsZero() {
		startedAt = now
	}
	sessionID := sessionIDFromFactoryConfig(cfg)
	source := "daemon-restart"
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			CurrentChainingTraceID:   stringPointerIfNonEmpty(currentTraceID),
			DispatchID:               &dispatchID,
			EventTime:                startedAt,
			PreviousChainingTraceIDs: stringSlicePointer(previousTraceIDs),
			SessionID:                &sessionID,
			Source:                   &source,
			Tick:                     maxInt(cfg.restoredWorldState.Tick, dispatch.StartedTick),
			TraceIDs:                 stringSlicePointer(traceIDs),
			WorkIDs:                  stringSlicePointer(workIDs),
		},
		Id:            "factory-event/dispatch-request/daemon-restart/" + dispatchID,
		Payload:       encoded,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchRequest,
	}, nil
}

func restoredDispatchWorkIDs(dispatch interfaces.FactoryWorldDispatch) []string {
	workIDs := append([]string(nil), dispatch.WorkItemIDs...)
	for _, input := range dispatch.Inputs {
		workID, ok := restoredDispatchWorkID(input)
		if ok {
			workIDs = append(workIDs, workID)
		}
	}
	return uniqueNonEmptyStrings(workIDs)
}

func restoredDispatchInputRefs(dispatch interfaces.FactoryWorldDispatch) []interfaces.DispatchConsumedWorkRef {
	refs := make([]interfaces.DispatchConsumedWorkRef, 0, len(dispatch.Inputs))
	seen := make(map[string]struct{}, len(dispatch.Inputs))
	for _, input := range dispatch.Inputs {
		workID, ok := restoredDispatchWorkID(input)
		if !ok {
			continue
		}
		workID = strings.TrimSpace(workID)
		if workID == "" {
			continue
		}
		if _, exists := seen[workID]; exists {
			continue
		}
		seen[workID] = struct{}{}
		refs = append(refs, interfaces.DispatchConsumedWorkRef{WorkID: workID})
	}
	return refs
}

func restoredDispatchResourceRefs(resources []interfaces.FactoryResourceUnit) *[]interfaces.DispatchResourceRef {
	if len(resources) == 0 {
		return nil
	}
	refs := make([]interfaces.DispatchResourceRef, 0, len(resources))
	for _, resource := range resources {
		name := strings.TrimSpace(resource.ResourceID)
		if name == "" {
			continue
		}
		refs = append(refs, interfaces.DispatchResourceRef{Name: name})
	}
	if len(refs) == 0 {
		return nil
	}
	return &refs
}

func stringPointerIfNonEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

const daemonRestartDispatchInterruptionReason = "daemon restart interrupted process-bound attempt"

// reconcileRestoredDispatches closes the process-owned side of every durable
// dispatch that was active when the previous daemon stopped. The restored
// Work marking is intentionally built separately: it places each input at its
// last logical place, while this event makes the old dispatch terminal in the
// public Factory/Worker Session projections. A canonical event is used so the
// recovery is visible to historical inspection and is persisted by the normal
// Recordings recorder.
func reconcileRestoredDispatches(cfg *runtimeConfig, eventHistory recordings.RuntimeLedger) error {
	if cfg == nil || cfg.restoredWorldState == nil || eventHistory == nil {
		return nil
	}
	if len(cfg.restoredWorldState.ActiveDispatches) == 0 {
		return nil
	}

	activeDispatchIDs := make([]string, 0, len(cfg.restoredWorldState.ActiveDispatches))
	for dispatchID := range cfg.restoredWorldState.ActiveDispatches {
		if strings.TrimSpace(dispatchID) != "" {
			activeDispatchIDs = append(activeDispatchIDs, dispatchID)
		}
	}
	sort.Strings(activeDispatchIDs)

	existingEvents := eventHistory.CanonicalEvents()
	for _, dispatchID := range activeDispatchIDs {
		dispatch := cfg.restoredWorldState.ActiveDispatches[dispatchID]
		if restoredDispatchHasTerminalEvent(existingEvents, dispatchID) {
			continue
		}
		// Historical dispatch reconstruction requires a transition-bearing
		// DISPATCH_REQUEST. Older in-memory fixtures can describe an active
		// dispatch without that field; preserve their existing association /
		// interruption behavior while real recorded dispatches use the complete
		// synthetic request below.
		if strings.TrimSpace(dispatch.TransitionID) != "" &&
			!restoredDispatchHasRequest(existingEvents, dispatchID) {
			request, err := restoredDispatchRequestEvent(cfg, dispatch)
			if err != nil {
				return err
			}
			eventHistory.AppendRecordedEvent(request)
			existingEvents = append(existingEvents, request)
		}
		if !restoredDispatchHasAssociation(existingEvents, dispatchID) {
			association, err := restoredDispatchAssociationEvent(cfg, dispatch)
			if err != nil {
				return err
			}
			eventHistory.AppendRecordedEvent(association)
			existingEvents = append(existingEvents, association)
		}
		event, err := restoredDispatchInterruptionEvent(cfg, dispatch)
		if err != nil {
			return err
		}
		eventHistory.AppendRecordedEvent(event)
		existingEvents = append(existingEvents, event)
	}
	return nil
}

func restoredDispatchHasTerminalEvent(events []interfaces.FactoryEvent, dispatchID string) bool {
	for _, event := range events {
		if stringPointerValue(event.Context.DispatchID) != dispatchID {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchInterrupted,
			interfaces.FactoryEventTypeDispatchReconciled,
			interfaces.FactoryEventTypeDispatchResponse:
			return true
		}
	}
	return false
}

func restoredDispatchHasAssociation(events []interfaces.FactoryEvent, dispatchID string) bool {
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeDispatchWorkerSessionAssoc &&
			stringPointerValue(event.Context.DispatchID) == dispatchID {
			return true
		}
	}
	return false
}

func restoredDispatchAssociationEvent(
	cfg *runtimeConfig,
	dispatch interfaces.FactoryWorldDispatch,
) (interfaces.FactoryEvent, error) {
	dispatchID := strings.TrimSpace(dispatch.DispatchID)
	if dispatchID == "" {
		return interfaces.FactoryEvent{}, fmt.Errorf("restored active dispatch has no dispatch ID")
	}
	workerSessionID := dispatchID
	payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{
		WorkerSessionID: workerSessionID,
	})
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("encode Worker Session association for dispatch %q: %w", dispatchID, err)
	}
	now := cfg.clock.Now().UTC()
	startedAt := dispatch.StartedAt.UTC()
	if dispatch.StartedAt.IsZero() {
		startedAt = now
	}
	sessionID := sessionIDFromFactoryConfig(cfg)
	source := "daemon-restart"
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  startedAt,
			SessionID:  &sessionID,
			Source:     &source,
			Tick:       maxInt(cfg.restoredWorldState.Tick, dispatch.StartedTick),
		},
		Id:            "factory-event/dispatch-worker-session-association/daemon-restart/" + dispatchID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
	}, nil
}

func restoredDispatchInterruptionEvent(
	cfg *runtimeConfig,
	dispatch interfaces.FactoryWorldDispatch,
) (interfaces.FactoryEvent, error) {
	dispatchID := strings.TrimSpace(dispatch.DispatchID)
	if dispatchID == "" {
		return interfaces.FactoryEvent{}, fmt.Errorf("restored active dispatch has no dispatch ID")
	}
	workIDs := append([]string(nil), dispatch.WorkItemIDs...)
	if len(workIDs) == 0 {
		for _, input := range dispatch.Inputs {
			if input.WorkItem != nil && strings.TrimSpace(input.WorkItem.ID) != "" {
				workIDs = append(workIDs, input.WorkItem.ID)
			}
		}
	}
	workIDs = uniqueNonEmptyStrings(workIDs)
	now := cfg.clock.Now().UTC()
	payload, err := json.Marshal(interfaces.DispatchInterruptedEventPayload{
		InterruptedAt:  now,
		ObservedStatus: interfaces.FactoryDispatchStatusRunning,
		Reason:         daemonRestartDispatchInterruptionReason,
		RetryPlanned:   true,
	})
	if err != nil {
		return interfaces.FactoryEvent{}, fmt.Errorf("encode interruption for dispatch %q: %w", dispatchID, err)
	}
	sessionID := sessionIDFromFactoryConfig(cfg)
	source := "daemon-restart"
	return interfaces.FactoryEvent{
		Context: interfaces.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  now,
			SessionID:  &sessionID,
			Source:     &source,
			Tick:       maxInt(cfg.restoredWorldState.Tick, dispatch.StartedTick),
			WorkIDs:    stringSlicePointer(workIDs),
		},
		Id:            "factory-event/dispatch-interrupted/daemon-restart/" + dispatchID,
		Payload:       payload,
		SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type:          interfaces.FactoryEventTypeDispatchInterrupted,
	}, nil
}

func maxInt(first, second int) int {
	if second > first {
		return second
	}
	return first
}

func stringSlicePointer(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	return &cloned
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sessionIDFromFactoryConfig(cfg *runtimeConfig) string {
	if cfg != nil && cfg.workflowContext != nil {
		if sessionID := strings.TrimSpace(cfg.workflowContext.SessionID); sessionID != "" {
			return sessionID
		}
	}
	return factory_context.DefaultSessionID
}

func factoryConfigFromFactoryConfig(cfg *runtimeConfig) *interfaces.FactoryConfig {
	if cfg == nil {
		return nil
	}
	provider, ok := cfg.runtimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok {
		return nil
	}
	return provider.FactoryConfig()
}

func validateFactoryRuntimeDependencies(
	net *state.Net,
	eventHistory recordings.RuntimeLedger,
	clock factory.Clock,
	workRequestIDs work.RequestIDGenerator,
	newID factory.IDGenerator,
	statelessService executeCapability,
	workerSessionsService workersessions.Service,
) error {
	if net == nil {
		return fmt.Errorf("a factory specification is required")
	}
	if eventHistory == nil {
		return fmt.Errorf("a Recordings runtime ledger is required")
	}
	if clock == nil {
		return fmt.Errorf("a Factory Runtime clock is required")
	}
	if workRequestIDs == nil {
		return fmt.Errorf("a Work Request ID generator is required")
	}
	if newID == nil {
		return fmt.Errorf("a Factory Runtime ID generator is required")
	}
	if statelessService == nil {
		return fmt.Errorf("a stateless Workers service is required")
	}
	if workerSessionsService == nil {
		return fmt.Errorf("a Worker Sessions service is required")
	}
	return nil
}

var (
	_ factory.ResourceCapacityService         = (*factoryImpl)(nil)
	_ factory.AdmittedResourceCapacityService = (*factoryImpl)(nil)
	_ factory.ResourceCapacityAdmission       = (*factoryImpl)(nil)
	_ factory.ResourceCapacityLeaseAdmission  = (*factoryImpl)(nil)
	_ factory.ResourceCapacityRevisionService = (*factoryImpl)(nil)
)

func (f *factoryImpl) AcquireResourceCapacityAdmission(ctx context.Context) (func(), error) {
	if f == nil || f.engine == nil {
		return nil, fmt.Errorf("Factory Runtime resource admission is unavailable")
	}
	return f.engine.AcquireResourceCapacityAdmission(ctx)
}

func (f *factoryImpl) AcquireResourceCapacityLease(
	ctx context.Context,
	request factory.ResourceCapacityLeaseRequest,
) (*factory.ResourceCapacityLease, error) {
	if f == nil || f.engine == nil {
		return nil, fmt.Errorf("Factory Runtime resource lease admission is unavailable")
	}
	return f.engine.AcquireResourceCapacityLease(ctx, request)
}

func (f *factoryImpl) CurrentFactoryRevision() int {
	if f == nil || f.engine == nil {
		return 0
	}
	return f.engine.CurrentFactoryRevision()
}

func (f *factoryImpl) SetFactoryRevision(revision int) {
	if f == nil || f.engine == nil {
		return
	}
	f.engine.SetFactoryRevision(revision)
}

func (f *factoryImpl) PreviewResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result, false)
}

func (f *factoryImpl) PreviewResourceCapacityAdmitted(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.PreviewResourceCapacityAdmitted(ctx, request)
	if err != nil {
		return result, err
	}
	return f.attachResourceCapacitySnapshot(result, false)
}

func (f *factoryImpl) SetResourceCapacity(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.SetResourceCapacity(ctx, request)
	if err != nil {
		return result, err
	}
	result, err = f.attachResourceCapacitySnapshot(result, result.Outcome == factory.ResourceCapacityOutcomeApplied)
	if err != nil {
		return result, err
	}
	f.wakeAfterResourceCapacityChange(result)
	return result, nil
}

func (f *factoryImpl) SetResourceCapacityAdmitted(ctx context.Context, request factory.ResourceCapacityRequest) (factory.ResourceCapacityResult, error) {
	if f == nil || f.engine == nil {
		return factory.ResourceCapacityResult{}, fmt.Errorf("Factory Runtime resource capacity is unavailable")
	}
	result, err := f.engine.SetResourceCapacityAdmitted(ctx, request)
	if err != nil {
		return result, err
	}
	result, err = f.attachResourceCapacitySnapshot(result, result.Outcome == factory.ResourceCapacityOutcomeApplied)
	if err != nil {
		return result, err
	}
	f.wakeAfterResourceCapacityChange(result)
	return result, nil
}

func (f *factoryImpl) wakeAfterResourceCapacityChange(result factory.ResourceCapacityResult) {
	if f == nil || f.engine == nil || result.Outcome != factory.ResourceCapacityOutcomeApplied {
		return
	}
	f.engine.WakeForResourceCapacity()
}

func (f *factoryImpl) attachResourceCapacitySnapshot(result factory.ResourceCapacityResult, commit bool) (factory.ResourceCapacityResult, error) {
	if f == nil {
		return result, fmt.Errorf("Factory Runtime is unavailable")
	}
	f.capacitySnapshotMu.Lock()
	defer f.capacitySnapshotMu.Unlock()
	config, err := f.effectiveFactoryConfigForCapacityLocked()
	if err != nil {
		return result, err
	}
	for index := range config.Resources {
		if interfaces.CanonicalFactoryGraphResourceID(config.Resources[index]) != result.ResourceID {
			continue
		}
		config.Resources[index].Capacity = result.EffectiveCapacity
		if config.Resources[index].Name == "" {
			config.Resources[index].Name = result.ResourceName
		}
		break
	}
	if !hasResourceConfig(config, result.ResourceID) {
		config.Resources = append(config.Resources, interfaces.ResourceConfig{
			ID: result.ResourceID, Name: result.ResourceName, Capacity: result.EffectiveCapacity,
		})
	}
	snapshot, err := interfaces.NewFactorySnapshot(config)
	if err != nil {
		return result, fmt.Errorf("capture effective resource capacity Factory: %w", err)
	}
	if commit && result.Outcome == factory.ResourceCapacityOutcomeApplied {
		f.effectiveFactoryConfig = config
	}
	result.Factory = snapshot
	return result, nil
}

func (f *factoryImpl) effectiveFactoryConfigForCapacityLocked() (*interfaces.FactoryConfig, error) {
	if f.effectiveFactoryConfig != nil {
		cloned, err := interfaces.CloneFactoryConfig(f.effectiveFactoryConfig)
		if err != nil {
			return nil, fmt.Errorf("clone effective Factory: %w", err)
		}
		return cloned, nil
	}
	var source *interfaces.FactoryConfig
	if f.cfg != nil {
		source = factoryConfigFromFactoryConfig(f.cfg)
	}
	if source != nil {
		cloned, err := interfaces.CloneFactoryConfig(source)
		if err != nil {
			return nil, fmt.Errorf("clone effective Factory: %w", err)
		}
		return cloned, nil
	}
	name := ""
	if f.topology != nil {
		name = f.topology.ID
	}
	return &interfaces.FactoryConfig{Name: name}, nil
}

func hasResourceConfig(config *interfaces.FactoryConfig, resourceID string) bool {
	if config == nil {
		return false
	}
	for _, resource := range config.Resources {
		if interfaces.CanonicalFactoryGraphResourceID(resource) == resourceID {
			return true
		}
	}
	return false
}

// BindModelsRuntimeScope attaches the opened Models capability to this
// session's managed-model dispatches. The scope is a runtime-owned binding;
// Workers still owns inference selection and execution through Execute.
func (f *factoryImpl) BindModelsRuntimeScope(scope modelprovider.RuntimeScopeRef) error {
	if f == nil || f.cfg == nil {
		return fmt.Errorf("Factory Runtime is unavailable")
	}
	if scope.IsZero() {
		return modelprovider.ErrRuntimeScopeInvalid
	}
	f.cfg.modelRuntimeScope = scope
	return nil
}

// modelRuntimeInputForSelection projects the session-owned Models scope into
// the detached Workers request selected by Factory Runtime. Runtime does not
// invoke Models or choose a backend here; it only carries the opened scope and
// authored worker/resource facts to Workers, whose inference runner owns the
// local-vs-provider decision.
func modelRuntimeInputForSelection(
	cfg *runtimeConfig,
	selection runtimeExecutionSelection,
) *workers.ModelRuntimeInput {
	if cfg == nil || cfg.modelRuntimeScope.IsZero() ||
		!strings.EqualFold(
			strings.TrimSpace(selection.modelLocality),
			modelprovider.RuntimeModelLocalityLocal,
		) || strings.TrimSpace(selection.model) == "" {
		return nil
	}

	worker := modelprovider.LocalWorker{
		Name:          strings.TrimSpace(selection.workerName),
		Type:          strings.TrimSpace(selection.workerType),
		Model:         strings.TrimSpace(selection.model),
		ModelLocality: strings.TrimSpace(selection.modelLocality),
	}
	var resources []modelprovider.LocalResource
	if lookup, ok := runtimeDefinitionLookup(cfg); ok {
		if definition, found := lookup.Worker(worker.Name); found && definition != nil {
			worker.Resources = localResourcesFromFactory(definition.Resources)
		}
		if factoryLookup, found := lookup.(interfaces.RuntimeFactoryConfigLookup); found {
			if factoryConfig := factoryLookup.FactoryConfig(); factoryConfig != nil {
				resources = localResourcesFromFactory(factoryConfig.Resources)
			}
		}
	}

	return &workers.ModelRuntimeInput{
		Scope:     cfg.modelRuntimeScope,
		Worker:    worker,
		Resources: resources,
	}
}

func localResourcesFromFactory(
	resources []interfaces.ResourceConfig,
) []modelprovider.LocalResource {
	if len(resources) == 0 {
		return nil
	}
	projected := make([]modelprovider.LocalResource, len(resources))
	for index, resource := range resources {
		projected[index] = modelprovider.LocalResource{
			ID:         resource.ID,
			Name:       resource.Name,
			Type:       resource.Type,
			Capacity:   resource.Capacity,
			Model:      resource.Model,
			Backend:    resource.Backend,
			LoadPolicy: resource.LoadPolicy,
			Provider:   resource.Provider,
		}
	}
	return projected
}

// seedRestoredWork copies only recorded Work into a fresh marking. Resource
// tokens have already been generated from the current topology and clock by
// buildRuntimeMarking, so recorded resource occupancy is intentionally not an
// input to this conversion.
func seedRestoredWork(
	marking *petri.Marking,
	net *state.Net,
	restored *interfaces.FactoryWorldState,
	now time.Time,
	resourcePlaceIDs map[string]struct{},
	excludedWorkIDs map[string]struct{},
	toleratedWorkIDs map[string]struct{},
) (map[string]struct{}, error) {
	seededWorkIDs := make(map[string]struct{})
	if marking == nil || net == nil || restored == nil {
		return seededWorkIDs, nil
	}
	items := restoredWorkItems(restored)
	placements := restoredWorkPlacements(restored, items)
	if err := validateRestoredWorkState(restored, net, items, placements, resourcePlaceIDs, toleratedWorkIDs); err != nil {
		return nil, err
	}
	requestIDs := restoredWorkRequestIDs(restored)
	parentIDs := make(map[string]struct{})

	workIDs := make([]string, 0, len(items))
	for workID := range items {
		workIDs = append(workIDs, workID)
	}
	sort.Strings(workIDs)
	for _, workID := range workIDs {
		if _, excluded := excludedWorkIDs[workID]; excluded {
			// Deterministic replay re-materializes Work with recorded dispatch
			// facts from its replay submission at the recorded logical tick. Do
			// not let the detached seed dispatch one tick before that hook.
			continue
		}
		placeID, hasPlacement := placements[workID]
		if !hasPlacement {
			// WorkItemsByID is the durable historical index. Only current
			// occupancy becomes a live runtime token.
			continue
		}
		token, ok := restoredWorkTokenForPlacement(
			marking,
			net,
			restored,
			items[workID],
			placeID,
			requestIDs[workID],
			restored.RelationsByWorkID[workID],
			now,
			resourcePlaceIDs,
		)
		if !ok {
			return nil, fmt.Errorf(
				"restore Work %q at %q: placement cannot be represented by the current Factory topology",
				workID,
				placements[workID],
			)
		}
		marking.AddToken(token)
		seededWorkIDs[token.Color.WorkID] = struct{}{}
		registerRestoredWorkParent(marking, token, parentIDs)
	}

	for _, parentID := range sortedStringKeys(parentIDs) {
		marking.CompleteParentChildRegistration(parentID)
	}
	return seededWorkIDs, nil
}

// restoredWorkIDsWithRecordedDispatch returns the Work identities whose
// restored replay facts include a dispatch. Replay must re-materialize those
// requests so the recorded side effect can run, replacing their seeded token
// at the materialization boundary; Work without a recorded dispatch remains
// seeded in place.
func restoredWorkIDsWithRecordedDispatch(restored *interfaces.FactoryWorldState) map[string]struct{} {
	workIDs := make(map[string]struct{})
	if restored == nil {
		return workIDs
	}
	for _, dispatch := range restored.ActiveDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, dispatch := range restored.CompletedDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, dispatch := range restored.FailedDispatches {
		addRestoredDispatchWorkIDs(workIDs, dispatch.WorkItemIDs)
	}
	for _, approval := range restored.PendingHumanApprovalsByID {
		addRestoredDispatchWorkIDs(workIDs, approval.WorkItemIDs)
	}
	return workIDs
}

func addRestoredDispatchWorkIDs(destination map[string]struct{}, workIDs []string) {
	for _, workID := range workIDs {
		if workID != "" {
			destination[workID] = struct{}{}
		}
	}
}
