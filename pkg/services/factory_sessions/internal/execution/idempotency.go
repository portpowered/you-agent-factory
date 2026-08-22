package factorysessionexecution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livechange"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
)

// IdempotencyTupleHash returns a stable digest for the normalized execution tuple
// used to compare replay safety for one requestId.
func IdempotencyTupleHash(req StartRequest) (string, error) {
	normalized, err := normalizeIdempotencyDocument(req)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal idempotency tuple: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// CheckRequestIDReplay reports ErrExecutionRequestIDConflict when requestId was
// previously recorded with a different normalized tuple.
func CheckRequestIDReplay(requestID, recordedHash, incomingHash string) error {
	if strings.TrimSpace(requestID) == "" {
		return NewValidationError("requestId", "requestId is required")
	}
	if recordedHash == "" || recordedHash == incomingHash {
		return nil
	}
	return ErrExecutionRequestIDConflict
}

// CheckAsyncStartReplayMode reports ErrExecutionRequestIDConflict when requestId
// was previously used for a sync start rather than an exact async replay.
func CheckAsyncStartReplayMode(asyncStart *AsyncStartResult) error {
	if asyncStart != nil {
		return nil
	}
	return ErrExecutionRequestIDConflict
}

// CheckSyncStartReplayMode reports ErrExecutionRequestIDConflict when requestId
// was previously used for an async start rather than an exact sync replay.
func CheckSyncStartReplayMode(asyncStart *AsyncStartResult, syncStart *SyncStartResult, syncStartInFlight bool) error {
	if syncStart != nil || syncStartInFlight {
		return nil
	}
	if asyncStart != nil {
		return ErrExecutionRequestIDConflict
	}
	return nil
}

func normalizeIdempotencyDocument(req StartRequest) (map[string]any, error) {
	source, err := normalizeSourceForIdempotency(req.Source)
	if err != nil {
		return nil, err
	}
	document := map[string]any{
		"source": source,
	}
	if len(req.Args) > 0 {
		args, err := canonicalizeMap(req.Args)
		if err != nil {
			return nil, err
		}
		document["args"] = args
	}
	if req.Orchestrator != nil {
		orchestrator, err := canonicalizeRawJSON(req.Orchestrator.Raw)
		if err != nil {
			return nil, err
		}
		document["orchestrator"] = orchestrator
	}
	if policy := normalizeRequestedPolicyForIdempotency(req.RequestedPolicy); policy != nil {
		document["requestedPolicy"] = policy
	}
	if req.Runtime != nil {
		document["runtime"] = map[string]any{
			"childExecutorMode": normalizeChildExecutorMode(req.Runtime.ChildExecutorMode),
		}
	}
	return document, nil
}

func normalizeSourceForIdempotency(source Source) (map[string]any, error) {
	document := map[string]any{
		"kind": string(source.Kind),
	}
	switch source.Kind {
	case workflowsource.WorkflowSourceKindFactoryID:
		document["factoryId"] = strings.TrimSpace(source.FactoryID)
	case workflowsource.WorkflowSourceKindFactoryInline:
		inline, err := canonicalizeRawJSON(source.FactoryInline)
		if err != nil {
			return nil, err
		}
		document["factoryInline"] = inline
	case workflowsource.WorkflowSourceKindWorkflowFile:
		document["workflowFile"] = strings.TrimSpace(source.WorkflowFile)
	case workflowsource.WorkflowSourceKindWorkflowName:
		document["workflowName"] = strings.TrimSpace(source.WorkflowName)
	case workflowsource.WorkflowSourceKindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return nil, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inline := map[string]any{
			"inlineSource": strings.TrimSpace(source.InlineWorkflow.InlineSource),
		}
		if dialect := strings.TrimSpace(source.InlineWorkflow.Dialect); dialect != "" {
			inline["dialect"] = dialect
		}
		if entrypoint := strings.TrimSpace(source.InlineWorkflow.Entrypoint); entrypoint != "" {
			inline["entrypoint"] = entrypoint
		}
		if len(source.InlineWorkflow.Metadata) > 0 {
			inline["metadata"] = canonicalizeStringMap(source.InlineWorkflow.Metadata)
		}
		document["inlineWorkflow"] = inline
	default:
		return nil, NewValidationError("source.kind", "source.kind is invalid")
	}
	return document, nil
}

func normalizeRequestedPolicyForIdempotency(policy map[string]any) any {
	if len(policy) == 0 {
		return nil
	}
	if hash, ok := policy["policyHash"].(string); ok && strings.TrimSpace(hash) != "" {
		return map[string]string{"policyHash": strings.TrimSpace(hash)}
	}
	canonical, err := canonicalizeMap(policy)
	if err != nil {
		return policy
	}
	return canonical
}

func canonicalizeRawJSON(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("parse json document: %w", err)
	}
	return canonicalizeValue(value)
}

func canonicalizeMap(document map[string]any) (map[string]any, error) {
	if len(document) == 0 {
		return nil, nil
	}
	value, err := canonicalizeValue(document)
	if err != nil {
		return nil, err
	}
	canonical, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", value)
	}
	return canonical, nil
}

func canonicalizeStringMap(document map[string]string) map[string]string {
	if len(document) == 0 {
		return nil
	}
	keys := make([]string, 0, len(document))
	for key := range document {
		keys = append(keys, key)
	}
	sortStrings(keys)
	out := make(map[string]string, len(document))
	for _, key := range keys {
		out[key] = document[key]
	}
	return out
}

func canonicalizeValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sortStrings(keys)
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			canonical, err := canonicalizeValue(typed[key])
			if err != nil {
				return nil, err
			}
			out[key] = canonical
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			canonical, err := canonicalizeValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, canonical)
		}
		return out, nil
	default:
		return typed, nil
	}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func (s *JavaScriptRuntimeService) tryReplayAsyncStart(
	ctx context.Context,
	requestID string,
	tupleHash string,
	waitIfInflight bool,
) (AsyncStartResult, bool, error) {
	for {
		s.mu.Lock()
		replay, ok := s.startReplay[requestID]
		if !ok {
			s.mu.Unlock()
			return AsyncStartResult{}, false, nil
		}
		if err := CheckRequestIDReplay(requestID, replay.tupleHash, tupleHash); err != nil {
			s.mu.Unlock()
			return AsyncStartResult{}, false, err
		}
		if replay.asyncStart != nil {
			cloned := cloneAsyncStartResult(*replay.asyncStart)
			s.mu.Unlock()
			return cloned, true, nil
		}
		if waitIfInflight {
			if flight, ok := s.startInflight[requestID]; ok {
				done := flight.done
				s.mu.Unlock()
				select {
				case <-ctx.Done():
					return AsyncStartResult{}, false, ctx.Err()
				case <-done:
					continue
				}
			}
		}
		if err := CheckAsyncStartReplayMode(replay.asyncStart); err != nil {
			s.mu.Unlock()
			return AsyncStartResult{}, false, err
		}
		s.mu.Unlock()
		return AsyncStartResult{}, false, nil
	}
}

func (s *JavaScriptRuntimeService) tryReplaySyncStart(
	ctx context.Context,
	requestID string,
	tupleHash string,
	waitIfInflight bool,
) (SyncStartResult, bool, error) {
	for {
		s.mu.Lock()
		replay, ok := s.startReplay[requestID]
		if !ok {
			s.mu.Unlock()
			return SyncStartResult{}, false, nil
		}
		if err := CheckRequestIDReplay(requestID, replay.tupleHash, tupleHash); err != nil {
			s.mu.Unlock()
			return SyncStartResult{}, false, err
		}
		if replay.syncStart != nil {
			cloned := cloneSyncStartResult(*replay.syncStart)
			s.mu.Unlock()
			return cloned, true, nil
		}
		syncStartInFlight := false
		if waitIfInflight {
			if flight, ok := s.startInflight[requestID]; ok {
				syncStartInFlight = true
				done := flight.done
				s.mu.Unlock()
				select {
				case <-ctx.Done():
					return SyncStartResult{}, false, ctx.Err()
				case <-done:
					continue
				}
			}
		}
		if err := CheckSyncStartReplayMode(replay.asyncStart, replay.syncStart, syncStartInFlight); err != nil {
			s.mu.Unlock()
			return SyncStartResult{}, false, err
		}
		s.mu.Unlock()
		return SyncStartResult{}, false, nil
	}
}

func (s *JavaScriptRuntimeService) recordAsyncStartReplay(requestID string, result AsyncStartResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	replay := s.startReplay[requestID]
	cloned := cloneAsyncStartResult(result)
	replay.asyncStart = &cloned
	s.startReplay[requestID] = replay
}

func (s *JavaScriptRuntimeService) recordSyncStartReplay(requestID string, result SyncStartResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	replay := s.startReplay[requestID]
	cloned := cloneSyncStartResult(result)
	replay.syncStart = &cloned
	s.startReplay[requestID] = replay
}

type reservedStartSession struct {
	state   *runtimeSessionState
	isNew   bool
	release func()
}

func (s *JavaScriptRuntimeService) reserveStartSession(
	ctx context.Context,
	normalized StartRequest,
	tupleHash string,
	waitIfInflight bool,
) (*reservedStartSession, error) {
	for {
		s.mu.Lock()
		if replay, ok := s.startReplay[normalized.RequestID]; ok {
			if err := CheckRequestIDReplay(normalized.RequestID, replay.tupleHash, tupleHash); err != nil {
				s.mu.Unlock()
				return nil, err
			}
			state, ok := s.sessions[replay.sessionID]
			if !ok {
				s.mu.Unlock()
				return nil, ErrSessionNotFound
			}
			if waitIfInflight {
				if flight, ok := s.startInflight[normalized.RequestID]; ok {
					done := flight.done
					s.mu.Unlock()
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-done:
						continue
					}
				}
			}
			s.mu.Unlock()
			return &reservedStartSession{state: state, isNew: false, release: func() {}}, nil
		}

		flight := &startInflightFlight{done: make(chan struct{})}
		sessionID, err := NewDurableSessionID(s.generateSessionID)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		placeholder := &runtimeSessionState{
			session: SessionReadResult{SessionID: sessionID},
		}
		s.sessions[sessionID] = placeholder
		s.startReplay[normalized.RequestID] = startReplayRecord{
			sessionID: sessionID,
			tupleHash: tupleHash,
		}
		s.startInflight[normalized.RequestID] = flight
		s.mu.Unlock()

		release := func() {
			s.mu.Lock()
			delete(s.startInflight, normalized.RequestID)
			close(flight.done)
			s.mu.Unlock()
		}
		return &reservedStartSession{state: placeholder, isNew: true, release: release}, nil
	}
}

func syncWaitTimeout(normalized StartRequest) (time.Duration, bool) {
	if normalized.Wait == nil || normalized.Wait.TimeoutMillis == nil || *normalized.Wait.TimeoutMillis <= 0 {
		return 0, false
	}
	return time.Duration(*normalized.Wait.TimeoutMillis) * time.Millisecond, true
}

func (s *JavaScriptRuntimeService) startWaitingSyncSession(
	ctx context.Context,
	reserved *reservedStartSession,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowsource.JavaScriptPolicyResolution,
	waitTimeout time.Duration,
) (SyncStartResult, error) {
	startedAt := s.now()
	running := projectRuntimeRunningSessionState(
		reserved.state.session.SessionID,
		normalized,
		resolved,
		policyResolution,
		startedAt,
	)
	runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
	runDone := make(chan struct{})
	s.mu.Lock()
	reserved.state.session = running.session
	reserved.state.result = running.result
	reserved.state.events = running.events
	reserved.state.runCancel = runCancel
	reserved.state.runDone = runDone
	reserved.state.startRequest = cloneStartRequest(normalized)
	reserved.state.resolvedSource = resolved
	reserved.state.sourceContent = sourceContent
	s.mu.Unlock()

	if err := s.launchAsyncRun(func() {
		s.runAsyncSession(
			runCtx,
			reserved.state.session.SessionID,
			normalized,
			resolved,
			sourceContent,
			policyResolution,
			startedAt,
			runDone,
		)
	}); err != nil {
		runCancel()
		s.mu.Lock()
		reserved.state.runCancel = nil
		close(runDone)
		s.mu.Unlock()
		return SyncStartResult{}, err
	}

	result, err := s.waitSyncCompletion(
		ctx,
		reserved.state.session.SessionID,
		waitTimeout,
		normalized.Wait.CancelOnTimeout,
	)
	if err != nil {
		return SyncStartResult{}, err
	}
	s.recordSyncStartReplay(normalized.RequestID, result)
	return result, nil
}

func (s *JavaScriptRuntimeService) completeImmediateSyncStart(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowsource.JavaScriptPolicyResolution,
	reserved *reservedStartSession,
) (SyncStartResult, error) {
	terminal, err := s.executeImmediateSyncSession(
		ctx, normalized, resolved, sourceContent, policyResolution, reserved.state.session.SessionID,
	)
	if err != nil {
		return SyncStartResult{}, err
	}
	s.mu.Lock()
	candidate := cloneRuntimeSessionState(reserved.state)
	applyRuntimeSessionFields(&candidate, terminal)
	candidate.runCancel = nil
	if err := s.recordCanonicalTerminalState(reserved.state, candidate); err != nil {
		s.mu.Unlock()
		return SyncStartResult{}, err
	}
	s.mu.Unlock()

	snapshot, err := s.snapshotSessionState(reserved.state.session.SessionID)
	if err != nil {
		return SyncStartResult{}, err
	}
	result := s.syncStartFromState(snapshot)
	s.recordSyncStartReplay(normalized.RequestID, result)
	return result, nil
}

func (s *JavaScriptRuntimeService) waitSyncCompletion(
	ctx context.Context,
	sessionID string,
	waitTimeout time.Duration,
	cancelOnTimeout bool,
) (SyncStartResult, error) {
	deadline := s.syncWaits.Now().Add(waitTimeout)

	for {
		select {
		case <-ctx.Done():
			return SyncStartResult{}, ctx.Err()
		case <-s.syncWaits.After(10 * time.Millisecond):
			if !s.syncWaits.Now().Before(deadline) {
				return s.projectSyncWaitTimeout(ctx, sessionID, cancelOnTimeout)
			}

			snapshot, err := s.snapshotSessionState(sessionID)
			if err != nil {
				return SyncStartResult{}, err
			}
			if IsTerminalLifecycleStatus(snapshot.session.Status) {
				return s.syncStartFromState(snapshot), nil
			}
		}
	}
}

func (s *JavaScriptRuntimeService) projectSyncWaitTimeout(
	ctx context.Context,
	sessionID string,
	cancelOnTimeout bool,
) (SyncStartResult, error) {
	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return SyncStartResult{}, ErrSessionNotFound
	}

	if cancelOnTimeout && state.runCancel != nil {
		state.runCancel()
	}
	s.mu.Unlock()

	if cancelOnTimeout {
		deadline := s.syncWaits.Now().Add(5 * time.Second)
		for s.syncWaits.Now().Before(deadline) {
			snapshot, err := s.snapshotSessionState(sessionID)
			if err != nil {
				return SyncStartResult{}, err
			}
			if IsTerminalLifecycleStatus(snapshot.session.Status) {
				result := s.syncStartFromState(snapshot)
				result.SyncOutcome = SyncOutcomeTimedOut
				result.TimedOut = true
				result.SessionCanceledByTimeout = true
				return result, nil
			}
			select {
			case <-ctx.Done():
				return SyncStartResult{}, ctx.Err()
			case <-s.syncWaits.After(10 * time.Millisecond):
			}
		}
	}

	s.mu.Lock()
	state, ok = s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return SyncStartResult{}, ErrSessionNotFound
	}
	state.result = ResultReadResult{
		SessionID:     sessionID,
		Mode:          ResultModeFinal,
		ResultStatus:  ResultStatusNotReady,
		SessionStatus: LifecycleStatusRunning,
		Availability: &ResultAvailabilityDetail{
			Reason:    "SYNC_WAIT_TIMED_OUT",
			Message:   "Sync wait ended before a terminal result was available.",
			Retryable: true,
		},
	}
	if state.session.ResultSummary == nil {
		state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusNotReady)}
	} else {
		state.session.ResultSummary.ResultStatus = string(ResultStatusNotReady)
	}
	state.events = rebuildRuntimeSessionCanonicalEvents(state)
	snapshot := cloneRuntimeSessionState(state)
	s.mu.Unlock()

	result := s.syncStartFromState(snapshot)
	result.SyncOutcome = SyncOutcomeTimedOut
	result.TimedOut = true
	return result, nil
}

// ApplyLiveChange lets a durable JavaScript Factory Session mutate the active
// Factory Runtime that owns its Worker children. The transport-facing session
// gateway remains the public owner; this optional capability keeps durable
// execution-specific state behind its existing boundary.
func (s *JavaScriptRuntimeService) ApplyLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
) (factorysessions.LiveChangeResult, error) {
	return s.runDurableLiveChange(ctx, sessionID, request, "")
}

// RecoverLiveChange closes an admitted durable live change after a restart or
// an interrupted application, using the request event retained in the session
// history as the source of truth.
func (s *JavaScriptRuntimeService) RecoverLiveChange(
	ctx context.Context,
	sessionID string,
	requestID string,
) (factorysessions.LiveChangeResult, error) {
	return s.runDurableLiveChange(ctx, sessionID, factorysessions.LiveChangeRequest{}, requestID)
}

func (s *JavaScriptRuntimeService) runDurableLiveChange(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	recoverRequestID string,
) (factorysessions.LiveChangeResult, error) {
	if s == nil {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	if err := ctx.Err(); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	runtime := s.workerInvoker()
	if runtime == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "durable Factory Session runtime is unavailable",
		}
	}

	s.liveChangeMu.Lock()
	defer s.liveChangeMu.Unlock()

	events := durableLiveChangeEventLog{service: s, sessionID: id}
	application := runtimebinding.NewLiveChangeApplication(runtime)
	if application == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "resource capacity application is unavailable",
		}
	}
	admission := runtimebinding.NewLiveChangeAdmission(runtime)
	if admission == nil {
		if _, requiresAdmission := runtime.(workflowsource.AdmittedResourceCapacityService); requiresAdmission {
			return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
				Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
				Message: "live change coordination is unavailable",
			}
		}
	} else {
		release, admissionErr := admission.AcquireLiveChange(ctx, id)
		if admissionErr != nil {
			return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
				Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
				Message: "live change coordination is unavailable",
				Cause:   admissionErr,
			}
		}
		defer release()
	}
	stateProvider := s.durableLiveChangeStateProvider(id, events)
	if s.liveChangeCoordinator == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "live change coordinator is unavailable",
		}
	}
	operation := factorysessions.LiveChangeOperation{
		StateProvider: stateProvider,
		Events:        events,
		Application:   application,
		Now:           s.now,
	}
	var result factorysessions.LiveChangeResult
	var applyErr error
	if recoverRequestID != "" {
		result, applyErr = s.liveChangeCoordinator.RecoverLiveChange(ctx, id, recoverRequestID, operation)
	} else {
		result, applyErr = s.liveChangeCoordinator.ApplyLiveChange(ctx, id, request, operation)
	}
	if applyErr == nil || result.Outcome == factorysessions.LiveChangeOutcomeReplayed {
		if revision, ok := runtime.(workflowsource.ResourceCapacityRevisionService); ok && result.NewRevision >= 0 {
			revision.SetFactoryRevision(result.NewRevision)
		}
	}
	return result, applyErr
}

func (s *JavaScriptRuntimeService) durableLiveChangeStateProvider(
	sessionID string,
	events factorysessions.LiveChangeEventLog,
) livechange.StateProvider {
	return func(_ context.Context, id string) (factorysessions.LiveChangeSessionState, error) {
		state, err := s.snapshotSessionState(id)
		if err != nil {
			return factorysessions.LiveChangeSessionState{}, err
		}
		projected := livechange.ProjectState(sessionID, events.LiveChangeEvents())
		projected.SessionID = sessionID
		projected.Lifecycle = durableLiveChangeLifecycle(state.session.Status)
		return projected, nil
	}
}

func durableLiveChangeLifecycle(status LifecycleStatus) factorysessions.LiveChangeLifecycle {
	switch status {
	case LifecycleStatusPaused:
		return factorysessions.LiveChangeLifecyclePaused
	case LifecycleStatusSucceeded, LifecycleStatusCanceled, LifecycleStatusTimedOut,
		LifecycleStatusInterrupted, LifecycleStatusTerminated:
		return factorysessions.LiveChangeLifecycleCompleted
	case LifecycleStatusFailed:
		return factorysessions.LiveChangeLifecycleFailed
	case LifecycleStatusQueued, LifecycleStatusAwaitingApproval:
		return factorysessions.LiveChangeLifecycleFailed
	default:
		return factorysessions.LiveChangeLifecycleRunning
	}
}

// durableLiveChangeEventLog appends the coordinator's canonical events to the
// durable session snapshot. It does not use the live Factory ledger because a
// durable session has its own identity, reconnect cursor, and recovery history.
type durableLiveChangeEventLog struct {
	service   *JavaScriptRuntimeService
	sessionID string
}

func (log durableLiveChangeEventLog) LiveChangeEvents() []interfaces.FactoryEvent {
	if log.service == nil {
		return nil
	}
	log.service.mu.RLock()
	state, ok := log.service.sessions[log.sessionID]
	if ok {
		cloned := cloneRuntimeSessionState(state)
		state = &cloned
	}
	log.service.mu.RUnlock()
	if !ok || state == nil {
		return nil
	}
	return decodeDurableFactoryEvents(state.events)
}

func (log durableLiveChangeEventLog) AppendLiveChangeEvent(event interfaces.FactoryEvent) (interfaces.FactoryEvent, error) {
	if log.service == nil {
		return interfaces.FactoryEvent{}, errors.New("durable live change service is unavailable")
	}
	log.service.mu.Lock()
	state, ok := log.service.sessions[log.sessionID]
	if !ok || state == nil {
		log.service.mu.Unlock()
		return interfaces.FactoryEvent{}, factorysessions.ErrSessionNotFound
	}

	event = normalizeDurableLiveChangeEvent(event, state.events)
	encoded, err := json.Marshal(event)
	if err != nil {
		log.service.mu.Unlock()
		return interfaces.FactoryEvent{}, fmt.Errorf("encode durable live change event: %w", err)
	}
	candidate := cloneRuntimeSessionState(state)
	candidate.events = append(candidate.events, encoded)
	if err := log.service.persistSessionSnapshot(candidate); err != nil {
		log.service.mu.Unlock()
		return interfaces.FactoryEvent{}, err
	}
	state.events = append(state.events, encoded)
	log.service.mu.Unlock()

	log.service.presentCurrentFactoryEvents(log.sessionID)
	return event.Clone(), nil
}

func decodeDurableFactoryEvents(raw []json.RawMessage) []interfaces.FactoryEvent {
	if len(raw) == 0 {
		return nil
	}
	events := make([]interfaces.FactoryEvent, 0, len(raw))
	for _, encoded := range raw {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(encoded, &event); err != nil || strings.TrimSpace(string(event.Type)) == "" {
			continue
		}
		events = append(events, event)
	}
	return events
}

func hasDurableLiveChangeEvents(raw []json.RawMessage) bool {
	for _, encoded := range raw {
		var event interfaces.FactoryEvent
		if json.Unmarshal(encoded, &event) != nil {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeFactoryChangeRequest,
			interfaces.FactoryEventTypeFactoryChange,
			interfaces.FactoryEventTypeFactoryChangeFailed:
			return true
		}
	}
	return false
}

func normalizeDurableLiveChangeEvent(event interfaces.FactoryEvent, prior []json.RawMessage) interfaces.FactoryEvent {
	event.SchemaVersion = interfaces.FactoryEventSchemaVersionV1
	event.Context.Sequence = nextDurableLiveChangeEventSequence(prior)
	event.Context.Tick = event.Context.Sequence
	sessionSequence := nextDurableLiveChangeSessionSequence(prior)
	event.Context.SessionSequence = &sessionSequence
	if event.Type == interfaces.FactoryEventTypeFactoryChange {
		var payload interfaces.FactoryChangeEventPayload
		if json.Unmarshal(event.Payload, &payload) == nil {
			sequence := event.Context.Sequence
			payload.EffectiveSequence = &sequence
			if encoded, err := json.Marshal(payload); err == nil {
				event.Payload = encoded
			}
		}
	}
	return event
}

func nextDurableLiveChangeEventSequence(raw []json.RawMessage) int {
	next := 1
	for _, encoded := range raw {
		var event interfaces.FactoryEvent
		if json.Unmarshal(encoded, &event) != nil {
			continue
		}
		if event.Context.Sequence >= next {
			next = event.Context.Sequence + 1
		}
	}
	return next
}

func nextDurableLiveChangeSessionSequence(raw []json.RawMessage) int {
	next := 0
	for _, encoded := range raw {
		var event interfaces.FactoryEvent
		if json.Unmarshal(encoded, &event) != nil || event.Context.SessionSequence == nil {
			continue
		}
		if *event.Context.SessionSequence >= next {
			next = *event.Context.SessionSequence + 1
		}
	}
	return next
}
