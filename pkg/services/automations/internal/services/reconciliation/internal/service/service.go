package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	reconciliation "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/reconciliation"
)

type service struct{}

var _ reconciliation.Service = (*service)(nil)

// New constructs an inert deterministic reconciliation service.
func New() reconciliation.Service {
	return &service{}
}

func (*service) Reconcile(
	_ context.Context,
	request automations.ReconcileRequest,
) (automations.ReconcileResult, error) {
	desired, observed, err := validateAndIndex(request)
	if err != nil {
		return automations.ReconcileResult{}, err
	}

	keys := unionKeys(desired, observed)
	outcomes := make([]automations.ConvergenceOutcome, 0, len(keys))
	for _, key := range keys {
		spec, wanted := desired[key]
		observation, exists := observed[key]
		if !wanted {
			spec = automations.DesiredSpec{
				AutomationID: observation.AutomationID,
				SourceID:     observation.SourceID,
				State:        automations.DesiredLifecycleStopped,
			}
		}
		outcomes = append(outcomes, decide(spec, observation, exists))
	}
	return automations.ReconcileResult{Outcomes: outcomes}, nil
}

type identityKey struct {
	automationID string
	sourceID     string
}

func validateAndIndex(
	request automations.ReconcileRequest,
) (map[identityKey]automations.DesiredSpec, map[identityKey]automations.ObservedInstance, error) {
	if len(request.Desired) == 0 && len(request.Observed) == 0 {
		return nil, nil, invalidError("at least one desired or observed source is required")
	}

	desired := make(map[identityKey]automations.DesiredSpec, len(request.Desired))
	for _, spec := range request.Desired {
		key := identityKey{automationID: spec.AutomationID, sourceID: spec.SourceID}
		if !validIdentity(key) || strings.TrimSpace(spec.Kind) == "" || !validDesired(spec.State) {
			return nil, nil, invalidError("malformed desired source")
		}
		if _, duplicate := desired[key]; duplicate {
			return nil, nil, invalidError("duplicate desired source identity")
		}
		desired[key] = spec
	}

	observed := make(map[identityKey]automations.ObservedInstance, len(request.Observed))
	instanceOwners := make(map[string]identityKey, len(request.Observed))
	for _, observation := range request.Observed {
		key := identityKey{
			automationID: observation.AutomationID,
			sourceID:     observation.SourceID,
		}
		if !validIdentity(key) ||
			strings.TrimSpace(observation.InstanceID) == "" ||
			!validObserved(observation.State) {
			return nil, nil, invalidError("malformed observed source")
		}
		if _, duplicate := observed[key]; duplicate {
			return nil, nil, invalidError("duplicate observed source identity")
		}
		if owner, duplicate := instanceOwners[observation.InstanceID]; duplicate && owner != key {
			return nil, nil, invalidError("instance identity belongs to multiple sources")
		}
		observed[key] = observation
		instanceOwners[observation.InstanceID] = key
	}
	return desired, observed, nil
}

func decide(
	spec automations.DesiredSpec,
	observation automations.ObservedInstance,
	exists bool,
) automations.ConvergenceOutcome {
	outcome := automations.ConvergenceOutcome{
		AutomationID: spec.AutomationID,
		SourceID:     spec.SourceID,
		InstanceID:   stableInstanceID(spec.AutomationID, spec.SourceID),
		Desired:      spec.State,
		Observed:     automations.ObservedLifecyclePending,
		Action:       automations.ConvergenceActionCreated,
		Convergence:  automations.ConvergenceStatusProgressing,
	}
	if !exists {
		if spec.State == automations.DesiredLifecycleStopped {
			outcome.Action = automations.ConvergenceActionRemoved
			outcome.Observed = automations.ObservedLifecycleStopped
			outcome.Convergence = automations.ConvergenceStatusConverged
		}
		return outcome
	}

	outcome.InstanceID = observation.InstanceID
	outcome.Observed = observation.State
	if observation.State == automations.ObservedLifecycleFailed {
		outcome.Action = automations.ConvergenceActionUnchanged
		outcome.Convergence = automations.ConvergenceStatusFailed
		return outcome
	}
	if observation.State == automations.ObservedLifecycleCancelled {
		outcome.Action = automations.ConvergenceActionUnchanged
		outcome.Convergence = automations.ConvergenceStatusCancelled
		return outcome
	}

	outcome.Action, outcome.Convergence = activeDecision(spec.State, observation.State)
	return outcome
}

func activeDecision(
	desired automations.DesiredLifecycleState,
	observed automations.ObservedLifecycleState,
) (automations.ConvergenceAction, automations.ConvergenceStatus) {
	if desired == automations.DesiredLifecycleRunning {
		switch observed {
		case automations.ObservedLifecycleRunning:
			return automations.ConvergenceActionUnchanged, automations.ConvergenceStatusConverged
		case automations.ObservedLifecyclePending, automations.ObservedLifecycleStarting:
			return automations.ConvergenceActionUnchanged, automations.ConvergenceStatusProgressing
		default:
			return automations.ConvergenceActionUpdated, automations.ConvergenceStatusProgressing
		}
	}

	switch observed {
	case automations.ObservedLifecycleStopped:
		return automations.ConvergenceActionUnchanged, automations.ConvergenceStatusConverged
	case automations.ObservedLifecycleStopping:
		return automations.ConvergenceActionUnchanged, automations.ConvergenceStatusProgressing
	default:
		return automations.ConvergenceActionRemoved, automations.ConvergenceStatusProgressing
	}
}

func unionKeys(
	desired map[identityKey]automations.DesiredSpec,
	observed map[identityKey]automations.ObservedInstance,
) []identityKey {
	keys := make([]identityKey, 0, len(desired)+len(observed))
	for key := range desired {
		keys = append(keys, key)
	}
	for key := range observed {
		if _, exists := desired[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].automationID != keys[j].automationID {
			return keys[i].automationID < keys[j].automationID
		}
		return keys[i].sourceID < keys[j].sourceID
	})
	return keys
}

func stableInstanceID(automationID, sourceID string) string {
	identity := fmt.Sprintf("%d:%s:%d:%s", len(automationID), automationID, len(sourceID), sourceID)
	sum := sha256.Sum256([]byte("automations-source-instance:" + identity))
	return "automation-instance:" + hex.EncodeToString(sum[:16])
}

func validIdentity(key identityKey) bool {
	return key.automationID != "" &&
		key.sourceID != "" &&
		key.automationID == strings.TrimSpace(key.automationID) &&
		key.sourceID == strings.TrimSpace(key.sourceID)
}

func validDesired(state automations.DesiredLifecycleState) bool {
	return state == automations.DesiredLifecycleRunning ||
		state == automations.DesiredLifecycleStopped
}

func validObserved(state automations.ObservedLifecycleState) bool {
	switch state {
	case automations.ObservedLifecyclePending,
		automations.ObservedLifecycleStarting,
		automations.ObservedLifecycleRunning,
		automations.ObservedLifecycleStopping,
		automations.ObservedLifecycleStopped,
		automations.ObservedLifecycleFailed,
		automations.ObservedLifecycleCancelled:
		return true
	default:
		return false
	}
}

func invalidError(reason string) *automations.Error {
	return &automations.Error{
		Op:   "Reconcile",
		Code: automations.ErrorCodeInvalid,
		Err:  fmt.Errorf("%w: %s", automations.ErrInvalidRequest, reason),
	}
}
