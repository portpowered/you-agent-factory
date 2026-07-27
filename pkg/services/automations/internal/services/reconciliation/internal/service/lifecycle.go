package service

import (
	"context"
	"strings"
	"sync"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
)

type sourceRecord struct {
	mu          sync.Mutex
	kind        string
	desired     automations.DesiredLifecycleState
	observation automations.SourceObservation
	terminalErr error
}

func (s *service) StartSource(
	ctx context.Context,
	request automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	if err := validateStartRequest(request); err != nil {
		return automations.StartSourceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		observation := pendingObservation(request.Identity)
		return automations.StartSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleRunning, observation, false,
			),
		}, cancelledOperationError("StartSource", err)
	}
	record, err := s.recordForStart(request)
	if err != nil {
		return automations.StartSourceResult{}, err
	}

	record.mu.Lock()
	result, effect, prior, err := s.planStartLocked(ctx, request, record)
	record.mu.Unlock()
	if err != nil || effect == nil {
		return result, err
	}
	if err := s.effects.Start(ctx, *effect); err != nil {
		record.mu.Lock()
		outcome, terminalErr := commitStartFailure(record, prior, err)
		record.mu.Unlock()
		return automations.StartSourceResult{Outcome: outcome}, terminalErr
	}
	return result, nil
}

type lifecycleSnapshot struct {
	desired     automations.DesiredLifecycleState
	observation automations.SourceObservation
	terminalErr error
}

func (s *service) planStartLocked(
	ctx context.Context,
	request automations.StartSourceRequest,
	record *sourceRecord,
) (automations.StartSourceResult, *reconciliation.StartEffect, lifecycleSnapshot, error) {
	if record.kind != request.Kind {
		return automations.StartSourceResult{}, nil, lifecycleSnapshot{}, operationError(
			"StartSource", automations.ErrorCodeConflict, automations.ErrConflict,
			"source kind differs from the authoritative source",
		)
	}
	if err := validateAuthoritativeResume(request.Resume, record.observation); err != nil {
		return automations.StartSourceResult{}, nil, lifecycleSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return automations.StartSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleRunning, record.observation, false,
			),
		}, nil, lifecycleSnapshot{}, cancelledOperationError("StartSource", err)
	}
	if result, done := existingStartResult(record); done {
		return result, nil, lifecycleSnapshot{}, record.terminalErr
	}
	if s.effects.Start == nil {
		return automations.StartSourceResult{}, nil, lifecycleSnapshot{},
			unavailableEffectsError("StartSource")
	}

	prior := lifecycleSnapshot{
		desired:     record.desired,
		observation: record.observation,
		terminalErr: record.terminalErr,
	}
	record.desired = automations.DesiredLifecycleRunning
	record.observation.State = automations.ObservedLifecycleStarting
	record.terminalErr = nil
	effect := reconciliation.StartEffect{Kind: record.kind, Observation: record.observation}
	return automations.StartSourceResult{Outcome: lifecycleOutcome(
		record.desired, record.observation, automations.ConvergenceStatusProgressing, false,
	)}, &effect, prior, nil
}

func existingStartResult(
	record *sourceRecord,
) (automations.StartSourceResult, bool) {
	switch record.observation.State {
	case automations.ObservedLifecycleRunning:
		return automations.StartSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleRunning,
				record.observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, true
	case automations.ObservedLifecycleStarting, automations.ObservedLifecycleStopping:
		record.desired = automations.DesiredLifecycleRunning
		return automations.StartSourceResult{
			Outcome: lifecycleOutcome(
				record.desired,
				record.observation,
				automations.ConvergenceStatusProgressing,
				true,
			),
		}, true
	case automations.ObservedLifecycleFailed, automations.ObservedLifecycleCancelled:
		if record.desired == automations.DesiredLifecycleRunning {
			return automations.StartSourceResult{
				Outcome: terminalLifecycleOutcome(
					record.desired, record.observation, true,
				),
			}, true
		}
	}
	return automations.StartSourceResult{}, false
}

func commitStartFailure(
	record *sourceRecord,
	prior lifecycleSnapshot,
	err error,
) (automations.LifecycleOutcome, error) {
	if isCancellation(err) {
		record.desired = prior.desired
		record.observation = prior.observation
		record.terminalErr = prior.terminalErr
		return cancelledLifecycleOutcome(
			automations.DesiredLifecycleRunning, prior.observation, false,
		), cancelledOperationError("StartSource", err)
	}
	return recordEffectFailure(
		"StartSource", automations.DesiredLifecycleRunning, record, err,
	)
}

func (s *service) StopSource(
	ctx context.Context,
	request automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	if !validSourceIdentity(request.Identity) {
		return automations.StopSourceResult{}, invalidOperationError(
			"StopSource", "malformed source identity",
		)
	}
	record, ok := s.findRecord(request.Identity)
	if !ok {
		return automations.StopSourceResult{}, operationError(
			"StopSource", automations.ErrorCodeNotFound, automations.ErrNotFound,
			"source is not supervised",
		)
	}

	record.mu.Lock()
	result, effect, prior, err := s.planStopLocked(ctx, record)
	record.mu.Unlock()
	if err != nil || effect == nil {
		return result, err
	}
	if err := s.effects.Stop(ctx, *effect); err != nil {
		record.mu.Lock()
		outcome, terminalErr := commitStopFailure(record, *effect, prior, err)
		record.mu.Unlock()
		return automations.StopSourceResult{Outcome: outcome}, terminalErr
	}
	return result, nil
}

func (s *service) planStopLocked(
	ctx context.Context,
	record *sourceRecord,
) (automations.StopSourceResult, *reconciliation.StopEffect, lifecycleSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return automations.StopSourceResult{
				Outcome: cancelledLifecycleOutcome(
					automations.DesiredLifecycleStopped, record.observation, false,
				),
			}, nil, lifecycleSnapshot{},
			cancelledOperationError("StopSource", err)
	}
	switch record.observation.State {
	case automations.ObservedLifecycleStopped:
		return automations.StopSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleStopped,
				record.observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil, lifecycleSnapshot{}, nil
	case automations.ObservedLifecycleStopping:
		return automations.StopSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleStopped,
				record.observation,
				automations.ConvergenceStatusProgressing,
				true,
			),
		}, nil, lifecycleSnapshot{}, nil
	case automations.ObservedLifecycleFailed, automations.ObservedLifecycleCancelled:
		if record.desired == automations.DesiredLifecycleStopped {
			return automations.StopSourceResult{
				Outcome: terminalLifecycleOutcome(
					record.desired, record.observation, true,
				),
			}, nil, lifecycleSnapshot{}, record.terminalErr
		}
	}

	if s.effects.Stop == nil {
		return automations.StopSourceResult{}, nil, lifecycleSnapshot{},
			unavailableEffectsError("StopSource")
	}

	prior := lifecycleSnapshot{
		desired:     record.desired,
		observation: record.observation,
		terminalErr: record.terminalErr,
	}
	record.desired = automations.DesiredLifecycleStopped
	record.observation.State = automations.ObservedLifecycleStopping
	record.terminalErr = nil
	effect := reconciliation.StopEffect{Observation: record.observation}
	return automations.StopSourceResult{
		Outcome: lifecycleOutcome(
			record.desired,
			record.observation,
			automations.ConvergenceStatusProgressing,
			false,
		),
	}, &effect, prior, nil
}

func commitStopFailure(
	record *sourceRecord,
	effect reconciliation.StopEffect,
	prior lifecycleSnapshot,
	err error,
) (automations.LifecycleOutcome, error) {
	if record.desired != automations.DesiredLifecycleStopped ||
		record.observation != effect.Observation {
		result, currentErr := currentWaitResult(automations.DesiredLifecycleStopped, record)
		return result.Outcome, currentErr
	}
	if isCancellation(err) {
		record.desired = prior.desired
		record.observation = prior.observation
		record.terminalErr = prior.terminalErr
		return cancelledLifecycleOutcome(
			automations.DesiredLifecycleStopped, prior.observation, false,
		), cancelledOperationError("StopSource", err)
	}
	return recordEffectFailure(
		"StopSource", automations.DesiredLifecycleStopped, record, err,
	)
}

func (s *service) WaitSource(
	ctx context.Context,
	request automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	if !validSourceIdentity(request.Identity) || !validDesired(request.Desired) {
		return automations.WaitSourceResult{}, invalidOperationError(
			"WaitSource", "malformed source identity or desired state",
		)
	}
	record, ok := s.findRecord(request.Identity)
	if !ok {
		return automations.WaitSourceResult{}, operationError(
			"WaitSource", automations.ErrorCodeNotFound, automations.ErrNotFound,
			"source is not supervised",
		)
	}

	record.mu.Lock()
	result, effect, err := s.planWaitLocked(ctx, request, record)
	record.mu.Unlock()
	if err != nil || effect == nil {
		return result, err
	}

	observation, effectErr := s.effects.Wait(ctx, *effect)
	record.mu.Lock()
	result, err = commitWaitLocked(request.Desired, *effect, observation, effectErr, record)
	record.mu.Unlock()
	return result, err
}

func (s *service) planWaitLocked(
	ctx context.Context,
	request automations.WaitSourceRequest,
	record *sourceRecord,
) (automations.WaitSourceResult, *reconciliation.WaitEffect, error) {
	if err := ctx.Err(); err != nil {
		return automations.WaitSourceResult{
			Outcome: cancelledLifecycleOutcome(
				request.Desired, record.observation, false,
			),
		}, nil, cancelledOperationError("WaitSource", err)
	}
	if desiredMatches(request.Desired, record.observation.State) {
		return automations.WaitSourceResult{
			Outcome: lifecycleOutcome(
				request.Desired,
				record.observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil, nil
	}
	if terminalConvergence(record.observation.State) != "" {
		return automations.WaitSourceResult{
			Outcome: terminalLifecycleOutcome(
				request.Desired, record.observation, true,
			),
		}, nil, record.terminalErr
	}
	if s.effects.Wait == nil {
		return automations.WaitSourceResult{}, nil, unavailableEffectsError("WaitSource")
	}

	effect := reconciliation.WaitEffect{
		Desired:     request.Desired,
		Observation: record.observation,
	}
	return automations.WaitSourceResult{}, &effect, nil
}

func commitWaitLocked(
	desired automations.DesiredLifecycleState,
	effect reconciliation.WaitEffect,
	observation automations.SourceObservation,
	effectErr error,
	record *sourceRecord,
) (automations.WaitSourceResult, error) {
	if record.observation != effect.Observation {
		return currentWaitResult(desired, record)
	}
	if effectErr != nil {
		outcome, terminalErr := recordEffectFailure(
			"WaitSource", desired, record, effectErr,
		)
		return automations.WaitSourceResult{Outcome: outcome}, terminalErr
	}
	if err := validateEffectObservation(effect.Observation, observation); err != nil {
		return automations.WaitSourceResult{}, err
	}
	record.observation = observation
	record.terminalErr = observationTerminalError("WaitSource", observation.State)
	convergence := terminalConvergence(observation.State)
	if convergence == "" {
		convergence = automations.ConvergenceStatusProgressing
	}
	if desiredMatches(desired, observation.State) {
		convergence = automations.ConvergenceStatusConverged
	}
	return automations.WaitSourceResult{
		Outcome: lifecycleOutcome(desired, observation, convergence, false),
	}, record.terminalErr
}

func currentWaitResult(
	desired automations.DesiredLifecycleState,
	record *sourceRecord,
) (automations.WaitSourceResult, error) {
	convergence := terminalConvergence(record.observation.State)
	if desiredMatches(desired, record.observation.State) {
		convergence = automations.ConvergenceStatusConverged
	}
	if convergence == "" {
		convergence = automations.ConvergenceStatusProgressing
	}
	return automations.WaitSourceResult{Outcome: lifecycleOutcome(
		desired, record.observation, convergence, true,
	)}, record.terminalErr
}

func (s *service) SourceStatus(
	_ context.Context,
	request automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	if !validSourceIdentity(request.Identity) {
		return automations.SourceStatusResult{}, invalidOperationError(
			"SourceStatus", "malformed source identity",
		)
	}
	record, ok := s.findRecord(request.Identity)
	if !ok {
		return automations.SourceStatusResult{}, operationError(
			"SourceStatus", automations.ErrorCodeNotFound, automations.ErrNotFound,
			"source is not supervised",
		)
	}

	record.mu.Lock()
	defer record.mu.Unlock()
	return automations.SourceStatusResult{Observation: record.observation}, nil
}

func (s *service) recordForStart(
	request automations.StartSourceRequest,
) (*sourceRecord, error) {
	key := identityKey{
		automationID: request.Identity.AutomationID,
		sourceID:     request.Identity.SourceID,
	}
	s.recordsMu.Lock()
	defer s.recordsMu.Unlock()
	if existing, ok := s.records[key]; ok {
		return existing, nil
	}

	observation := pendingObservation(request.Identity)
	if request.Resume != nil {
		observation = *request.Resume
	}
	record := &sourceRecord{
		kind:        request.Kind,
		desired:     automations.DesiredLifecycleRunning,
		observation: observation,
		terminalErr: observationTerminalError("StartSource", observation.State),
	}
	if owner, exists := s.instanceOwnerLocked(observation.InstanceID); exists && owner != key {
		return nil, operationError(
			"StartSource", automations.ErrorCodeConflict, automations.ErrConflict,
			"resume instance belongs to another source",
		)
	}
	s.records[key] = record
	return record, nil
}

func (s *service) instanceOwnerLocked(instanceID string) (identityKey, bool) {
	for key, record := range s.records {
		record.mu.Lock()
		matches := record.observation.InstanceID == instanceID
		record.mu.Unlock()
		if matches {
			return key, true
		}
	}
	return identityKey{}, false
}

func (s *service) findRecord(identity automations.SourceIdentity) (*sourceRecord, bool) {
	key := identityKey{automationID: identity.AutomationID, sourceID: identity.SourceID}
	s.recordsMu.RLock()
	defer s.recordsMu.RUnlock()
	record, ok := s.records[key]
	return record, ok
}

func validateStartRequest(request automations.StartSourceRequest) error {
	if !validSourceIdentity(request.Identity) || strings.TrimSpace(request.Kind) == "" {
		return invalidOperationError("StartSource", "malformed source identity or kind")
	}
	if request.Kind != strings.TrimSpace(request.Kind) {
		return invalidOperationError("StartSource", "source kind contains surrounding whitespace")
	}
	if request.Resume == nil {
		return nil
	}
	resume := *request.Resume
	if resume.Identity != request.Identity ||
		strings.TrimSpace(resume.InstanceID) == "" ||
		!validObserved(resume.State) {
		return invalidOperationError("StartSource", "malformed resume observation")
	}
	return nil
}

func validateAuthoritativeResume(
	resume *automations.SourceObservation,
	authoritative automations.SourceObservation,
) error {
	if resume == nil || *resume == authoritative {
		return nil
	}
	return operationError(
		"StartSource", automations.ErrorCodeConflict, automations.ErrConflict,
		"resume observation is stale or contradicts authoritative state",
	)
}

func validateEffectObservation(
	current automations.SourceObservation,
	next automations.SourceObservation,
) error {
	if next.Identity != current.Identity ||
		next.InstanceID != current.InstanceID ||
		!validObserved(next.State) {
		return invalidOperationError("WaitSource", "supervision returned a foreign observation")
	}
	return nil
}

func validSourceIdentity(identity automations.SourceIdentity) bool {
	return identity.AutomationID != "" &&
		identity.SourceID != "" &&
		identity.AutomationID == strings.TrimSpace(identity.AutomationID) &&
		identity.SourceID == strings.TrimSpace(identity.SourceID)
}

func desiredMatches(
	desired automations.DesiredLifecycleState,
	observed automations.ObservedLifecycleState,
) bool {
	return desired == automations.DesiredLifecycleRunning &&
		observed == automations.ObservedLifecycleRunning ||
		desired == automations.DesiredLifecycleStopped &&
			observed == automations.ObservedLifecycleStopped
}
