// Package livechange owns the Factory Sessions admission state machine. It
// deliberately knows only the session root contracts and explicit runtime
// ports; resource-specific mutation remains behind LiveChangeApplication.
package livechange

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
)

// StateProvider supplies the current lifecycle and revision projection for a
// session. It is called only after request identity/replay checks permit a new
// admission attempt.
type StateProvider func(context.Context, string) (factorysessions.LiveChangeSessionState, error)

// Service is the pure admission coordinator with injected clock and safe log
// sink. It has no mutable request registry: canonical events are the recovery
// source of truth.
type Service struct {
	now    func() time.Time
	logger *zap.Logger
}

// New constructs an admission coordinator with explicit process effects.
func New(now func() time.Time, logger *zap.Logger) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{now: now, logger: logger}
}

// Apply validates, admits, applies, and closes one live change. A request
// event is appended before application only after all pre-admission checks
// succeed.
func (s *Service) Apply(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	stateProvider StateProvider,
	events factorysessions.LiveChangeEventLog,
	application factorysessions.LiveChangeApplication,
) (factorysessions.LiveChangeResult, error) {
	normalized, err := factorysessions.NormalizeLiveChangeRequest(request)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if err := requireSessionID(sessionID, normalized); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	if err := requireDependencies(stateProvider, events); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	records := events.LiveChangeEvents()
	pending, terminal, conflict := findRequest(records, sessionID, normalized)
	if conflict != nil {
		return factorysessions.LiveChangeResult{}, conflict
	}
	if terminal != nil {
		return s.replayTerminal(sessionID, normalized, *terminal)
	}

	state, err := stateProvider(ctx, sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, stateError(err, normalized)
	}
	if err := validateLifecycle(state.Lifecycle, normalized); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	if pending == nil && state.EffectiveRevision != normalized.ExpectedRevision {
		return factorysessions.LiveChangeResult{}, revisionError(normalized, state.EffectiveRevision)
	}

	if pending != nil {
		return s.applyPending(ctx, sessionID, state, pending.Request, events, application)
	}
	return s.applyFresh(ctx, sessionID, state, normalized, events, application)
}

func (s *Service) applyFresh(
	ctx context.Context,
	sessionID string,
	state factorysessions.LiveChangeSessionState,
	request factorysessions.LiveChangeRequest,
	events factorysessions.LiveChangeEventLog,
	application factorysessions.LiveChangeApplication,
) (factorysessions.LiveChangeResult, error) {
	applicationRequest := &factorysessions.LiveChangeApplicationRequest{
		SessionID:        sessionID,
		Request:          request,
		PreviousRevision: request.ExpectedRevision,
		CurrentFactory:   cloneSnapshot(state.Factory),
	}
	if err := s.preflight(ctx, request, applicationRequest, application); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	if _, err := appendRequest(events, sessionID, request, s.now()); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	s.logger.Info("live change admitted", logFields(sessionID, request, state.EffectiveRevision)...)
	return s.applyApplication(ctx, sessionID, request, applicationRequest, events, application)
}

func (s *Service) applyPending(
	ctx context.Context,
	sessionID string,
	state factorysessions.LiveChangeSessionState,
	request factorysessions.LiveChangeRequest,
	events factorysessions.LiveChangeEventLog,
	application factorysessions.LiveChangeApplication,
) (factorysessions.LiveChangeResult, error) {
	// Recovery uses the exact normalized body retained in the request event; it
	// must not re-run target-specific preflight and accidentally turn one
	// admitted request into a second decision.
	if state.EffectiveRevision != request.ExpectedRevision {
		return s.closeFailure(
			events,
			sessionID,
			request,
			state.EffectiveRevision,
			"REVISION_CHANGED_DURING_RECOVERY",
			"the effective revision changed before recovery completed",
		)
	}
	applicationRequest := &factorysessions.LiveChangeApplicationRequest{
		SessionID:        sessionID,
		Request:          request,
		PreviousRevision: request.ExpectedRevision,
		CurrentFactory:   cloneSnapshot(state.Factory),
	}
	return s.applyApplication(ctx, sessionID, request, applicationRequest, events, application)
}

func (s *Service) applyApplication(
	ctx context.Context,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	applicationRequest *factorysessions.LiveChangeApplicationRequest,
	events factorysessions.LiveChangeEventLog,
	application factorysessions.LiveChangeApplication,
) (factorysessions.LiveChangeResult, error) {
	if application == nil {
		return s.closeFailure(
			events,
			sessionID,
			request,
			request.ExpectedRevision,
			string(factorysessions.LiveChangeErrorApplicationUnavailable),
			"the live change application is unavailable",
		)
	}
	result, applyErr := application.ApplyLiveChange(ctx, *applicationRequest)
	if applyErr != nil {
		code, _ := safeApplicationFailure(applyErr)
		return s.closeFailure(events, sessionID, request, request.ExpectedRevision, code, "")
	}
	if result.Factory == nil {
		return s.closeFailure(
			events,
			sessionID,
			request,
			request.ExpectedRevision,
			string(factorysessions.LiveChangeErrorApplicationFailed),
			"the live change application returned no effective Factory snapshot",
		)
	}

	return s.closeSuccess(events, sessionID, request, result.Factory, s.now())
}

// Recover closes a request event that has no terminal event. It is intentionally
// request-ID based so recovery after process restart does not need the original
// request body from the caller.
func (s *Service) Recover(
	ctx context.Context,
	sessionID string,
	requestID string,
	stateProvider StateProvider,
	events factorysessions.LiveChangeEventLog,
	application factorysessions.LiveChangeApplication,
) (factorysessions.LiveChangeResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorInvalidRequest, Field: "requestId", Message: "request id is required",
		}
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorSessionNotFound, Message: "Factory Session was not found", RequestID: requestID,
		}
	}
	if err := requireDependencies(stateProvider, events); err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	var pending *requestRecord
	for _, event := range events.LiveChangeEvents() {
		if event.Type != interfaces.FactoryEventTypeFactoryChangeRequest ||
			!eventBelongsToSession(event, sessionID) || event.Context.RequestID == nil || *event.Context.RequestID != requestID {
			continue
		}
		var payload interfaces.FactoryChangeRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			return factorysessions.LiveChangeResult{}, malformedEventError(requestID, err)
		}
		request, err := normalizeRecordedRequest(requestFromPayload(requestID, payload))
		if err != nil {
			return factorysessions.LiveChangeResult{}, malformedEventError(requestID, err)
		}
		if pending != nil {
			return factorysessions.LiveChangeResult{}, malformedEventError(requestID, errors.New("duplicate live change request event"))
		}
		pending = &requestRecord{Event: event, Request: request}
	}
	if pending == nil {
		return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorRecoveryUnavailable, Message: "no pending live change request was found", RequestID: requestID,
		}
	}
	_, terminal, conflict := findRequest(events.LiveChangeEvents(), sessionID, pending.Request)
	if conflict != nil {
		return factorysessions.LiveChangeResult{}, conflict
	}
	if terminal != nil {
		return s.replayTerminal(sessionID, pending.Request, *terminal)
	}
	return s.Apply(ctx, sessionID, pending.Request, stateProvider, events, application)
}

type requestRecord struct {
	Event   interfaces.FactoryEvent
	Request factorysessions.LiveChangeRequest
}

type terminalRecord struct {
	Event   interfaces.FactoryEvent
	Success *interfaces.FactoryChangeEventPayload
	Failure *interfaces.FactoryChangeFailedEventPayload
}

func requireDependencies(stateProvider StateProvider, events factorysessions.LiveChangeEventLog) error {
	if stateProvider == nil || events == nil {
		return &factorysessions.LiveChangeError{
			Code:    factorysessions.LiveChangeErrorApplicationUnavailable,
			Message: "live change admission is unavailable",
		}
	}
	return nil
}

func findRequest(events []interfaces.FactoryEvent, sessionID string, request factorysessions.LiveChangeRequest) (*requestRecord, *terminalRecord, error) {
	var pending *requestRecord
	var terminal *terminalRecord
	for _, event := range events {
		if !eventBelongsToSession(event, sessionID) {
			continue
		}
		foundPending, foundTerminal, err := matchRequestEvent(event, request)
		if err != nil {
			return nil, nil, err
		}
		if foundPending != nil {
			if pending != nil {
				return nil, nil, malformedEventError(request.RequestID, errors.New("duplicate live change request event"))
			}
			pending = foundPending
		}
		if foundTerminal != nil {
			if terminal != nil {
				return nil, nil, malformedEventError(request.RequestID, errors.New("duplicate live change terminal event"))
			}
			terminal = foundTerminal
		}
	}
	if terminal != nil {
		if pending == nil || pending.Event.Context.Sequence > terminal.Event.Context.Sequence {
			return nil, nil, malformedEventError(request.RequestID, errors.New("live change terminal event has no preceding request"))
		}
		return pending, terminal, nil
	}
	return pending, nil, nil
}

func matchRequestEvent(
	event interfaces.FactoryEvent,
	request factorysessions.LiveChangeRequest,
) (*requestRecord, *terminalRecord, error) {
	switch event.Type {
	case interfaces.FactoryEventTypeFactoryChangeRequest:
		return matchRequestRecord(event, request)
	case interfaces.FactoryEventTypeFactoryChange:
		return matchSuccessRecord(event, request)
	case interfaces.FactoryEventTypeFactoryChangeFailed:
		return matchFailureRecord(event, request)
	default:
		return nil, nil, nil
	}
}

func matchRequestRecord(
	event interfaces.FactoryEvent,
	request factorysessions.LiveChangeRequest,
) (*requestRecord, *terminalRecord, error) {
	var payload interfaces.FactoryChangeRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return nil, nil, malformedEventError(request.RequestID, err)
	}
	recordedID := valueOf(event.Context.RequestID)
	recorded, err := normalizeRecordedRequest(requestFromPayload(recordedID, payload))
	if err != nil {
		return nil, nil, malformedEventError(request.RequestID, err)
	}
	if recorded.ChangeID == request.ChangeID && recordedID != request.RequestID {
		return nil, nil, requestConflict(request)
	}
	if recordedID != request.RequestID {
		return nil, nil, nil
	}
	if fingerprint(recorded) != fingerprint(request) {
		return nil, nil, requestConflict(request)
	}
	return &requestRecord{Event: event, Request: recorded}, nil, nil
}

func matchSuccessRecord(
	event interfaces.FactoryEvent,
	request factorysessions.LiveChangeRequest,
) (*requestRecord, *terminalRecord, error) {
	var payload interfaces.FactoryChangeEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return nil, nil, malformedEventError(request.RequestID, err)
	}
	matched, err := terminalMatchesRequest(request, valueOf(event.Context.RequestID), payload.ChangeID)
	if err != nil || !matched {
		return nil, nil, err
	}
	return nil, &terminalRecord{Event: event, Success: &payload}, nil
}

func matchFailureRecord(
	event interfaces.FactoryEvent,
	request factorysessions.LiveChangeRequest,
) (*requestRecord, *terminalRecord, error) {
	var payload interfaces.FactoryChangeFailedEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		return nil, nil, malformedEventError(request.RequestID, err)
	}
	matched, err := terminalMatchesRequest(request, valueOf(event.Context.RequestID), payload.ChangeID)
	if err != nil || !matched {
		return nil, nil, err
	}
	return nil, &terminalRecord{Event: event, Failure: &payload}, nil
}

func terminalMatchesRequest(
	request factorysessions.LiveChangeRequest,
	eventRequestID string,
	changeID string,
) (bool, error) {
	if changeID == request.ChangeID && eventRequestID != request.RequestID {
		return false, requestConflict(request)
	}
	if eventRequestID != request.RequestID {
		return false, nil
	}
	if changeID != "" && changeID != request.ChangeID {
		return false, requestConflict(request)
	}
	return true, nil
}

func requestFromPayload(requestID string, payload interfaces.FactoryChangeRequestEventPayload) factorysessions.LiveChangeRequest {
	return factorysessions.LiveChangeRequest{
		RequestID:        requestID,
		ChangeID:         payload.ChangeID,
		ExpectedRevision: payload.ExpectedRevision,
		Operation:        payload.Operation,
		TargetID:         payload.TargetID,
		RequestedValue:   append(json.RawMessage(nil), payload.RequestedValue...),
		Actor:            payload.Actor,
		Source:           payload.Source,
		Reason:           payload.Reason,
	}
}

func normalizeRecordedRequest(request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeRequest, error) {
	return factorysessions.NormalizeLiveChangeRequest(request)
}

func requireSessionID(sessionID string, request factorysessions.LiveChangeRequest) error {
	if sessionID != "" {
		return nil
	}
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorSessionNotFound, Message: "Factory Session was not found",
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func eventBelongsToSession(event interfaces.FactoryEvent, sessionID string) bool {
	if event.Context.SessionID == nil {
		return false
	}
	return strings.TrimSpace(*event.Context.SessionID) != "" &&
		strings.TrimSpace(*event.Context.SessionID) == sessionID
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func fingerprint(request factorysessions.LiveChangeRequest) string {
	encoded, _ := json.Marshal(request)
	return string(encoded)
}

func requestConflict(request factorysessions.LiveChangeRequest) error {
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorRequestConflict, Message: "request id was already used with a different normalized body",
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func validateLifecycle(lifecycle factorysessions.LiveChangeLifecycle, request factorysessions.LiveChangeRequest) error {
	switch lifecycle {
	case factorysessions.LiveChangeLifecycleRunning, factorysessions.LiveChangeLifecycleIdle, factorysessions.LiveChangeLifecyclePaused:
		return nil
	default:
		return &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorLifecycleConflict, Message: "Factory Session lifecycle is not eligible for a live change",
			RequestID: request.RequestID, ChangeID: request.ChangeID,
		}
	}
}

func revisionError(request factorysessions.LiveChangeRequest, current int) error {
	return &factorysessions.LiveChangeError{
		Code:      factorysessions.LiveChangeErrorRevisionConflict,
		Message:   fmt.Sprintf("expected revision %d does not match effective revision %d", request.ExpectedRevision, current),
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func stateError(err error, request factorysessions.LiveChangeRequest) error {
	if errors.Is(err, factorysessions.ErrSessionNotFound) {
		return &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorSessionNotFound, Message: "Factory Session was not found",
			RequestID: request.RequestID, ChangeID: request.ChangeID, Cause: err,
		}
	}
	if typed := new(factorysessions.LiveChangeError); errors.As(err, &typed) {
		return typed
	}
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorApplicationUnavailable, Message: "Factory Session state is unavailable",
		RequestID: request.RequestID, ChangeID: request.ChangeID, Cause: err,
	}
}

func (s *Service) preflight(
	ctx context.Context,
	request factorysessions.LiveChangeRequest,
	applicationRequest *factorysessions.LiveChangeApplicationRequest,
	application factorysessions.LiveChangeApplication,
) error {
	preflight, ok := application.(factorysessions.LiveChangePreflight)
	if !ok {
		return nil
	}
	result, err := preflight.PreflightLiveChange(ctx, *applicationRequest)
	if err != nil {
		return preflightError(err, request)
	}
	if result.Factory != nil {
		applicationRequest.CurrentFactory = result.Factory.Clone()
	}
	if result.NoOp {
		return &factorysessions.LiveChangeError{
			Code: factorysessions.LiveChangeErrorNoOp, Message: "live change would not alter the effective Factory",
			RequestID: request.RequestID, ChangeID: request.ChangeID,
		}
	}
	if result.Admissible {
		return nil
	}
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorTargetNotFound, Message: "live change target was not found",
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func preflightError(err error, request factorysessions.LiveChangeRequest) error {
	if typed := new(factorysessions.LiveChangeError); errors.As(err, &typed) {
		typed.RequestID = request.RequestID
		typed.ChangeID = request.ChangeID
		return typed
	}
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorInvalidRequest, Message: "live change preflight rejected the request",
		RequestID: request.RequestID, ChangeID: request.ChangeID, Cause: err,
	}
}

func appendRequest(
	events factorysessions.LiveChangeEventLog,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	eventTime time.Time,
) (interfaces.FactoryEvent, error) {
	payload, err := json.Marshal(interfaces.FactoryChangeRequestEventPayload{
		ChangeID: request.ChangeID, ExpectedRevision: request.ExpectedRevision,
		Operation: request.Operation, TargetID: request.TargetID,
		RequestedValue: append(json.RawMessage(nil), request.RequestedValue...),
		Actor:          request.Actor, Source: request.Source, Reason: request.Reason,
	})
	if err != nil {
		return interfaces.FactoryEvent{}, eventAppendError(request, err)
	}
	event := interfaces.FactoryEvent{
		Type:    interfaces.FactoryEventTypeFactoryChangeRequest,
		Id:      "factory-event/factory-change-request/" + request.ChangeID,
		Context: changeContext(sessionID, request, eventTime),
		Payload: payload,
	}
	appended, err := events.AppendLiveChangeEvent(event)
	if err != nil {
		return interfaces.FactoryEvent{}, eventAppendError(request, err)
	}
	return appended, nil
}

func (s *Service) closeSuccess(
	events factorysessions.LiveChangeEventLog,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	factorySnapshot *interfaces.FactorySnapshot,
	eventTime time.Time,
) (factorysessions.LiveChangeResult, error) {
	previous := request.ExpectedRevision
	next := previous + 1
	payload, err := json.Marshal(interfaces.FactoryChangeEventPayload{
		Factory:          factorySnapshot.Clone(),
		ChangeID:         request.ChangeID,
		Operation:        request.Operation,
		TargetID:         request.TargetID,
		PreviousRevision: &previous,
		NewRevision:      &next,
	})
	if err != nil {
		return factorysessions.LiveChangeResult{}, eventAppendError(request, err)
	}
	event := interfaces.FactoryEvent{
		Type:    interfaces.FactoryEventTypeFactoryChange,
		Id:      "factory-event/factory-change/" + request.ChangeID,
		Context: changeContext(sessionID, request, eventTime),
		Payload: payload,
	}
	appended, err := events.AppendLiveChangeEvent(event)
	if err != nil {
		return factorysessions.LiveChangeResult{}, eventAppendError(request, err)
	}
	sequence := appended.Context.Sequence
	if decoded := decodeEffectiveSequence(appended.Payload); decoded != nil {
		sequence = *decoded
	}
	result := factorysessions.LiveChangeResult{
		SessionID: sessionID, RequestID: request.RequestID, ChangeID: request.ChangeID,
		Outcome: factorysessions.LiveChangeOutcomeApplied, PreviousRevision: previous,
		NewRevision: next, EffectiveSequence: sequence, Factory: factorySnapshot.Clone(),
	}
	s.logger.Info("live change completed", logFields(sessionID, request, next, zap.String("outcome", string(result.Outcome)))...)
	return result, nil
}

func (s *Service) closeFailure(
	events factorysessions.LiveChangeEventLog,
	sessionID string,
	request factorysessions.LiveChangeRequest,
	previousRevision int,
	code string,
	_ string,
) (factorysessions.LiveChangeResult, error) {
	failureCode := safeCode(code)
	failureMessage := safeFailureMessageForCode(failureCode)
	payload, err := json.Marshal(interfaces.FactoryChangeFailedEventPayload{
		ChangeID: request.ChangeID, Operation: request.Operation, TargetID: request.TargetID,
		ExpectedRevision: request.ExpectedRevision, PreviousRevision: previousRevision,
		FailureCode: failureCode, FailureMessage: failureMessage,
	})
	if err != nil {
		return factorysessions.LiveChangeResult{}, eventAppendError(request, err)
	}
	event := interfaces.FactoryEvent{
		Type:    interfaces.FactoryEventTypeFactoryChangeFailed,
		Id:      "factory-event/factory-change-failed/" + request.ChangeID,
		Context: changeContext(sessionID, request, s.now()),
		Payload: payload,
	}
	appended, err := events.AppendLiveChangeEvent(event)
	if err != nil {
		return factorysessions.LiveChangeResult{}, eventAppendError(request, err)
	}
	sequence := appended.Context.Sequence
	result := factorysessions.LiveChangeResult{
		SessionID: sessionID, RequestID: request.RequestID, ChangeID: request.ChangeID,
		Outcome: factorysessions.LiveChangeOutcomeFailed, PreviousRevision: previousRevision,
		NewRevision: previousRevision, EffectiveSequence: sequence,
		FailureCode: failureCode, FailureMessage: failureMessage,
	}
	s.logger.Info("live change completed", logFields(sessionID, request, previousRevision, zap.String("outcome", string(result.Outcome)), zap.String("failure_code", result.FailureCode))...)
	return result, terminalError(request, failureCode, failureMessage)
}

func (s *Service) replayTerminal(
	sessionID string,
	request factorysessions.LiveChangeRequest,
	terminal terminalRecord,
) (factorysessions.LiveChangeResult, error) {
	if terminal.Success != nil {
		payload := terminal.Success
		if payload.Factory == nil || payload.PreviousRevision == nil || payload.NewRevision == nil {
			return factorysessions.LiveChangeResult{}, malformedEventError(request.RequestID, errors.New("live change success event is incomplete"))
		}
		previous, next := intValue(payload.PreviousRevision), intValue(payload.NewRevision)
		sequence := terminal.Event.Context.Sequence
		if payload.EffectiveSequence != nil {
			sequence = *payload.EffectiveSequence
		}
		result := factorysessions.LiveChangeResult{
			SessionID: sessionID, RequestID: request.RequestID, ChangeID: request.ChangeID,
			Outcome: factorysessions.LiveChangeOutcomeReplayed, PreviousRevision: previous,
			NewRevision: next, EffectiveSequence: sequence, Factory: cloneSnapshot(payload.Factory),
		}
		return result, nil
	}
	if terminal.Failure != nil {
		failure := terminal.Failure
		result := factorysessions.LiveChangeResult{
			SessionID: sessionID, RequestID: request.RequestID, ChangeID: request.ChangeID,
			Outcome:          factorysessions.LiveChangeOutcomeReplayed,
			PreviousRevision: failure.PreviousRevision, NewRevision: failure.PreviousRevision,
			EffectiveSequence: terminal.Event.Context.Sequence,
			FailureCode:       safeCode(failure.FailureCode), FailureMessage: safeFailureMessageForCode(failure.FailureCode),
		}
		return result, terminalError(request, result.FailureCode, result.FailureMessage)
	}
	return factorysessions.LiveChangeResult{}, &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorRecoveryUnavailable, Message: "live change terminal event was malformed",
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func changeContext(sessionID string, request factorysessions.LiveChangeRequest, eventTime time.Time) interfaces.FactoryEventContext {
	return interfaces.FactoryEventContext{
		EventTime: eventTime,
		RequestID: stringPointer(request.RequestID),
		SessionID: stringPointer(sessionID),
		Source:    stringPointer(request.Source),
	}
}

func logFields(sessionID string, request factorysessions.LiveChangeRequest, revision int, extra ...zap.Field) []zap.Field {
	fields := []zap.Field{
		zap.String("session_id", sessionID),
		zap.String("request_id", request.RequestID),
		zap.String("change_id", request.ChangeID),
		zap.Int("revision", revision),
		zap.String("operation", request.Operation),
		zap.String("target_id", request.TargetID),
	}
	return append(fields, extra...)
}

func safeApplicationFailure(err error) (string, string) {
	if typed := new(factorysessions.LiveChangeError); errors.As(err, &typed) {
		code := string(typed.Code)
		if code == "" {
			code = string(factorysessions.LiveChangeErrorApplicationFailed)
		}
		return code, safeFailureMessageForCode(code)
	}
	return string(factorysessions.LiveChangeErrorApplicationFailed), "live change application failed"
}

func eventAppendError(request factorysessions.LiveChangeRequest, cause error) error {
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorEventAppendFailed, Message: "live change event could not be appended",
		RequestID: request.RequestID, ChangeID: request.ChangeID, Cause: cause,
	}
}

func malformedEventError(requestID string, cause error) error {
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorRecoveryUnavailable, Message: "live change history contains a malformed event",
		RequestID: requestID, Cause: cause,
	}
}

func safeCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return string(factorysessions.LiveChangeErrorApplicationFailed)
	}
	for _, char := range code {
		if (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			return string(factorysessions.LiveChangeErrorApplicationFailed)
		}
	}
	return code
}

func safeFailureMessageForCode(code string) string {
	switch safeCode(code) {
	case string(factorysessions.LiveChangeErrorTargetNotFound):
		return "live change target was not found"
	case string(factorysessions.LiveChangeErrorNoOp):
		return "live change would not alter the effective Factory"
	case string(factorysessions.LiveChangeErrorRevisionConflict):
		return "live change revision is no longer current"
	case string(factorysessions.LiveChangeErrorApplicationUnavailable):
		return "the live change application is unavailable"
	case "REVISION_CHANGED_DURING_RECOVERY":
		return "the effective revision changed before recovery completed"
	default:
		return "live change application failed"
	}
}

func terminalError(request factorysessions.LiveChangeRequest, code, _ string) *factorysessions.LiveChangeError {
	safe := safeCode(code)
	if !isKnownTerminalErrorCode(safe) {
		safe = string(factorysessions.LiveChangeErrorApplicationFailed)
	}
	return &factorysessions.LiveChangeError{
		Code: factorysessions.LiveChangeErrorCode(safe), Message: safeFailureMessageForCode(safe),
		RequestID: request.RequestID, ChangeID: request.ChangeID,
	}
}

func isKnownTerminalErrorCode(code string) bool {
	switch factorysessions.LiveChangeErrorCode(code) {
	case factorysessions.LiveChangeErrorInvalidRequest,
		factorysessions.LiveChangeErrorSessionNotFound,
		factorysessions.LiveChangeErrorLifecycleConflict,
		factorysessions.LiveChangeErrorRevisionConflict,
		factorysessions.LiveChangeErrorRequestConflict,
		factorysessions.LiveChangeErrorTargetNotFound,
		factorysessions.LiveChangeErrorNoOp,
		factorysessions.LiveChangeErrorApplicationFailed,
		factorysessions.LiveChangeErrorApplicationUnavailable,
		factorysessions.LiveChangeErrorRecoveryUnavailable,
		factorysessions.LiveChangeErrorEventAppendFailed:
		return true
	default:
		return false
	}
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func decodeEffectiveSequence(payload json.RawMessage) *int {
	var decoded interfaces.FactoryChangeEventPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return nil
	}
	return decoded.EffectiveSequence
}

func cloneSnapshot(snapshot *interfaces.FactorySnapshot) *interfaces.FactorySnapshot {
	if snapshot == nil {
		return nil
	}
	return snapshot.Clone()
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
