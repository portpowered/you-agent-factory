package http

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

var (
	// ErrInvalidReconcileDesired reports malformed desired specs at the Automations
	// reconcile HTTP adapter edge.
	ErrInvalidReconcileDesired = errors.New("automations http: invalid reconcile desired spec")
	// ErrInvalidReconcileObserved reports malformed observed instances at the
	// Automations reconcile HTTP adapter edge.
	ErrInvalidReconcileObserved = errors.New("automations http: invalid reconcile observed instance")
	// ErrInvalidInstanceIdentity reports missing or blank instance identifiers
	// at the Automations status/cursor HTTP adapter edge.
	ErrInvalidInstanceIdentity = errors.New("automations http: invalid instance identity")
)

// DesiredSpecInput carries one decoded desired automation spec for reconcile.
type DesiredSpecInput struct {
	AutomationID string
	SourceID     string
	Kind         string
	State        string
}

// ObservedInstanceInput carries one decoded observed automation instance for
// reconcile.
type ObservedInstanceInput struct {
	AutomationID string
	SourceID     string
	InstanceID   string
	State        string
}

// ReconcileInput carries decoded HTTP inputs for one Automations reconcile
// operation owned by this adapter.
type ReconcileInput struct {
	Desired  []DesiredSpecInput
	Observed []ObservedInstanceInput
}

// GetStatusInput carries decoded HTTP inputs for one Automations instance-status
// operation owned by this adapter.
type GetStatusInput struct {
	InstanceID string
}

// GetCursorInput carries decoded HTTP inputs for one Automations cursor read
// operation owned by this adapter.
type GetCursorInput struct {
	InstanceID     string
	ExpectedCursor string
}

// ConvergenceOutcomeResponse is the adapter-owned HTTP success shape for one
// reconcile convergence outcome.
type ConvergenceOutcomeResponse struct {
	AutomationID string `json:"automationId"`
	SourceID     string `json:"sourceId"`
	InstanceID   string `json:"instanceId"`
	Action       string `json:"action"`
	Desired      string `json:"desired"`
	Observed     string `json:"observed"`
	Convergence  string `json:"convergence"`
}

// GeneratedWorkRequestResponse is the adapter-owned HTTP success shape for one
// generated work request.
type GeneratedWorkRequestResponse struct {
	AutomationID     string `json:"automationId"`
	SourceID         string `json:"sourceId"`
	RequestID        string `json:"requestId"`
	Payload          []byte `json:"payload,omitempty"`
	PayloadReference string `json:"payloadReference,omitempty"`
}

// GeneratedWorkRequestOutcomeResponse is the adapter-owned HTTP success shape
// for one generated work request admission outcome.
type GeneratedWorkRequestOutcomeResponse struct {
	Request           GeneratedWorkRequestResponse `json:"request"`
	Status            string                       `json:"status"`
	RejectionReason   string                       `json:"rejectionReason,omitempty"`
	OriginalRequestID string                       `json:"originalRequestId,omitempty"`
}

// ReconcileResponse is the adapter-owned HTTP success shape for reconcile.
type ReconcileResponse struct {
	Outcomes              []ConvergenceOutcomeResponse            `json:"outcomes"`
	GeneratedWorkRequests []GeneratedWorkRequestOutcomeResponse `json:"generatedWorkRequests,omitempty"`
}

// GetStatusResponse is the adapter-owned HTTP success shape for instance status.
type GetStatusResponse struct {
	AutomationID string `json:"automationId"`
	InstanceID   string `json:"instanceId"`
	Status       string `json:"status"`
}

// GetCursorResponse is the adapter-owned HTTP success shape for cursor reads.
type GetCursorResponse struct {
	AutomationID string `json:"automationId"`
	InstanceID   string `json:"instanceId"`
	Cursor       string `json:"cursor"`
	Checkpoint   string `json:"checkpoint,omitempty"`
}

// IsConvergenceBadRequest reports whether an error is a decode/validation failure
// that maps to a typed bad-request HTTP outcome before root invocation.
func IsConvergenceBadRequest(err error) bool {
	return errors.Is(err, ErrInvalidReconcileDesired) ||
		errors.Is(err, ErrInvalidReconcileObserved) ||
		errors.Is(err, ErrInvalidInstanceIdentity)
}

// ReconcileRequestFromHTTP maps one reconcile HTTP request into the accepted
// Automations root request vocabulary.
func ReconcileRequestFromHTTP(input ReconcileInput) (automations.ReconcileRequest, error) {
	desired := make([]automations.DesiredSpec, 0, len(input.Desired))
	for i, spec := range input.Desired {
		mapped, err := desiredSpecFromHTTP(spec)
		if err != nil {
			return automations.ReconcileRequest{}, fmt.Errorf("%w at index %d: %v", ErrInvalidReconcileDesired, i, err)
		}
		desired = append(desired, mapped)
	}
	observed := make([]automations.ObservedInstance, 0, len(input.Observed))
	for i, instance := range input.Observed {
		mapped, err := observedInstanceFromHTTP(instance)
		if err != nil {
			return automations.ReconcileRequest{}, fmt.Errorf("%w at index %d: %v", ErrInvalidReconcileObserved, i, err)
		}
		observed = append(observed, mapped)
	}
	return automations.ReconcileRequest{
		Desired:  desired,
		Observed: observed,
	}, nil
}

// GetStatusRequestFromHTTP maps one instance-status HTTP request into the
// accepted Automations root request vocabulary.
func GetStatusRequestFromHTTP(input GetStatusInput) (automations.GetStatusRequest, error) {
	instanceID, err := instanceIdentityFromHTTP(input.InstanceID)
	if err != nil {
		return automations.GetStatusRequest{}, err
	}
	return automations.GetStatusRequest{InstanceID: instanceID}, nil
}

// GetCursorRequestFromHTTP maps one cursor HTTP request into the accepted
// Automations root request vocabulary.
func GetCursorRequestFromHTTP(input GetCursorInput) (automations.GetCursorRequest, error) {
	instanceID, err := instanceIdentityFromHTTP(input.InstanceID)
	if err != nil {
		return automations.GetCursorRequest{}, err
	}
	return automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: automations.Cursor(strings.TrimSpace(input.ExpectedCursor)),
	}, nil
}

// ReconcileResponseToHTTP encodes one fake-root reconcile result into the
// adapter-owned HTTP success response shape.
func ReconcileResponseToHTTP(result automations.ReconcileResult) ReconcileResponse {
	outcomes := make([]ConvergenceOutcomeResponse, 0, len(result.Outcomes))
	for _, outcome := range result.Outcomes {
		outcomes = append(outcomes, convergenceOutcomeToHTTP(outcome))
	}
	generated := make([]GeneratedWorkRequestOutcomeResponse, 0, len(result.GeneratedWorkRequests))
	for _, outcome := range result.GeneratedWorkRequests {
		generated = append(generated, generatedWorkRequestOutcomeToHTTP(outcome))
	}
	return ReconcileResponse{
		Outcomes:              outcomes,
		GeneratedWorkRequests: generated,
	}
}

// GetStatusResponseToHTTP encodes one fake-root instance-status result into
// the adapter-owned HTTP success response shape.
func GetStatusResponseToHTTP(result automations.GetStatusResult) GetStatusResponse {
	return GetStatusResponse{
		AutomationID: result.AutomationID,
		InstanceID:   result.InstanceID,
		Status:       string(result.Status),
	}
}

// GetCursorResponseToHTTP encodes one fake-root cursor result into the
// adapter-owned HTTP success response shape.
func GetCursorResponseToHTTP(result automations.GetCursorResult) GetCursorResponse {
	return GetCursorResponse{
		AutomationID: result.AutomationID,
		InstanceID:   result.InstanceID,
		Cursor:       string(result.Cursor),
		Checkpoint:   result.Checkpoint,
	}
}

func desiredSpecFromHTTP(input DesiredSpecInput) (automations.DesiredSpec, error) {
	automationID := strings.TrimSpace(input.AutomationID)
	sourceID := strings.TrimSpace(input.SourceID)
	kind := strings.TrimSpace(input.Kind)
	if automationID == "" || sourceID == "" || kind == "" {
		return automations.DesiredSpec{}, errors.New("automationId, sourceId, and kind are required")
	}
	state, err := desiredLifecycleFromHTTP(input.State)
	if err != nil {
		return automations.DesiredSpec{}, err
	}
	return automations.DesiredSpec{
		AutomationID: automationID,
		SourceID:     sourceID,
		Kind:         kind,
		State:        state,
	}, nil
}

func observedInstanceFromHTTP(input ObservedInstanceInput) (automations.ObservedInstance, error) {
	automationID := strings.TrimSpace(input.AutomationID)
	sourceID := strings.TrimSpace(input.SourceID)
	instanceID := strings.TrimSpace(input.InstanceID)
	state := strings.TrimSpace(input.State)
	if automationID == "" || sourceID == "" || instanceID == "" || state == "" {
		return automations.ObservedInstance{}, errors.New("automationId, sourceId, instanceId, and state are required")
	}
	return automations.ObservedInstance{
		AutomationID: automationID,
		SourceID:     sourceID,
		InstanceID:   instanceID,
		State:        automations.ObservedLifecycleState(state),
	}, nil
}

func instanceIdentityFromHTTP(instanceID string) (string, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return "", ErrInvalidInstanceIdentity
	}
	return instanceID, nil
}

func convergenceOutcomeToHTTP(outcome automations.ConvergenceOutcome) ConvergenceOutcomeResponse {
	return ConvergenceOutcomeResponse{
		AutomationID: outcome.AutomationID,
		SourceID:     outcome.SourceID,
		InstanceID:   outcome.InstanceID,
		Action:       string(outcome.Action),
		Desired:      string(outcome.Desired),
		Observed:     string(outcome.Observed),
		Convergence:  string(outcome.Convergence),
	}
}

func generatedWorkRequestOutcomeToHTTP(
	outcome automations.GeneratedWorkRequestOutcome,
) GeneratedWorkRequestOutcomeResponse {
	return GeneratedWorkRequestOutcomeResponse{
		Request: GeneratedWorkRequestResponse{
			AutomationID:     outcome.Request.Identity.AutomationID,
			SourceID:         outcome.Request.Identity.SourceID,
			RequestID:        outcome.Request.Identity.RequestID,
			Payload:          outcome.Request.Payload,
			PayloadReference: outcome.Request.PayloadReference,
		},
		Status:            string(outcome.Status),
		RejectionReason:   string(outcome.RejectionReason),
		OriginalRequestID: outcome.OriginalRequestID,
	}
}
