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
	defer record.mu.Unlock()
	if record.kind != request.Kind {
		return automations.StartSourceResult{}, operationError(
			"StartSource", automations.ErrorCodeConflict, automations.ErrConflict,
			"source kind differs from the authoritative source",
		)
	}
	if err := validateAuthoritativeResume(request.Resume, record.observation); err != nil {
		return automations.StartSourceResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return automations.StartSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleRunning, record.observation, false,
			),
		}, cancelledOperationError("StartSource", err)
	}

	switch record.observation.State {
	case automations.ObservedLifecycleRunning:
		return automations.StartSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleRunning,
				record.observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil
	case automations.ObservedLifecycleStarting, automations.ObservedLifecycleStopping:
		record.desired = automations.DesiredLifecycleRunning
		return automations.StartSourceResult{
			Outcome: lifecycleOutcome(
				record.desired,
				record.observation,
				automations.ConvergenceStatusProgressing,
				true,
			),
		}, nil
	case automations.ObservedLifecycleFailed, automations.ObservedLifecycleCancelled:
		if record.desired == automations.DesiredLifecycleRunning {
			return automations.StartSourceResult{
				Outcome: terminalLifecycleOutcome(
					record.desired, record.observation, true,
				),
			}, record.terminalErr
		}
	}

	if s.effects.Start == nil {
		return automations.StartSourceResult{}, unavailableEffectsError("StartSource")
	}
	if err := s.effects.Start(ctx, reconciliation.StartEffect{
		Kind:        record.kind,
		Observation: record.observation,
	}); err != nil {
		outcome, terminalErr := recordEffectFailure(
			"StartSource", automations.DesiredLifecycleRunning, record, err,
		)
		return automations.StartSourceResult{Outcome: outcome}, terminalErr
	}
	record.desired = automations.DesiredLifecycleRunning
	record.observation.State = automations.ObservedLifecycleStarting
	record.terminalErr = nil
	return automations.StartSourceResult{
		Outcome: lifecycleOutcome(
			record.desired,
			record.observation,
			automations.ConvergenceStatusProgressing,
			false,
		),
	}, nil
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
	defer record.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return automations.StopSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleStopped, record.observation, false,
			),
		}, cancelledOperationError("StopSource", err)
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
		}, nil
	case automations.ObservedLifecycleStopping:
		return automations.StopSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleStopped,
				record.observation,
				automations.ConvergenceStatusProgressing,
				true,
			),
		}, nil
	case automations.ObservedLifecycleFailed, automations.ObservedLifecycleCancelled:
		if record.desired == automations.DesiredLifecycleStopped {
			return automations.StopSourceResult{
				Outcome: terminalLifecycleOutcome(
					record.desired, record.observation, true,
				),
			}, record.terminalErr
		}
	}

	if s.effects.Stop == nil {
		return automations.StopSourceResult{}, unavailableEffectsError("StopSource")
	}
	if err := s.effects.Stop(ctx, reconciliation.StopEffect{
		Observation: record.observation,
	}); err != nil {
		outcome, terminalErr := recordEffectFailure(
			"StopSource", automations.DesiredLifecycleStopped, record, err,
		)
		return automations.StopSourceResult{Outcome: outcome}, terminalErr
	}
	record.desired = automations.DesiredLifecycleStopped
	record.observation.State = automations.ObservedLifecycleStopping
	record.terminalErr = nil
	return automations.StopSourceResult{
		Outcome: lifecycleOutcome(
			record.desired,
			record.observation,
			automations.ConvergenceStatusProgressing,
			false,
		),
	}, nil
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
	defer record.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return automations.WaitSourceResult{
			Outcome: cancelledLifecycleOutcome(
				request.Desired, record.observation, false,
			),
		}, cancelledOperationError("WaitSource", err)
	}
	if desiredMatches(request.Desired, record.observation.State) {
		return automations.WaitSourceResult{
			Outcome: lifecycleOutcome(
				request.Desired,
				record.observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil
	}
	if terminalConvergence(record.observation.State) != "" {
		return automations.WaitSourceResult{
			Outcome: terminalLifecycleOutcome(
				request.Desired, record.observation, true,
			),
		}, record.terminalErr
	}
	if s.effects.Wait == nil {
		return automations.WaitSourceResult{}, unavailableEffectsError("WaitSource")
	}

	observation, err := s.effects.Wait(ctx, reconciliation.WaitEffect{
		Desired:     request.Desired,
		Observation: record.observation,
	})
	if err != nil {
		outcome, terminalErr := recordEffectFailure(
			"WaitSource", request.Desired, record, err,
		)
		return automations.WaitSourceResult{Outcome: outcome}, terminalErr
	}
	if err := validateEffectObservation(record.observation, observation); err != nil {
		return automations.WaitSourceResult{}, err
	}
	record.observation = observation
	record.terminalErr = observationTerminalError("WaitSource", observation.State)
	convergence := terminalConvergence(observation.State)
	if convergence == "" {
		convergence = automations.ConvergenceStatusProgressing
	}
	if desiredMatches(request.Desired, observation.State) {
		convergence = automations.ConvergenceStatusConverged
	}
	return automations.WaitSourceResult{
		Outcome: lifecycleOutcome(request.Desired, observation, convergence, false),
	}, record.terminalErr
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
