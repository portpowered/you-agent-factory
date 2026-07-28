package http

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

var (
	// ErrInvalidSourceIdentity reports missing or blank automation/source identifiers
	// at the Automations lifecycle HTTP adapter edge.
	ErrInvalidSourceIdentity = errors.New("automations http: invalid source identity")
	// ErrInvalidSourceKind reports a missing or blank source kind for start.
	ErrInvalidSourceKind = errors.New("automations http: invalid source kind")
	// ErrInvalidLifecycleDesired reports an unsupported desired lifecycle state.
	ErrInvalidLifecycleDesired = errors.New("automations http: invalid lifecycle desired state")
	// ErrInvalidResumeObservation reports malformed resume facts on start.
	ErrInvalidResumeObservation = errors.New("automations http: invalid resume observation")
)

// SourceObservationResponse is the adapter-owned HTTP success shape for one
// detached source observation.
type SourceObservationResponse struct {
	AutomationID string `json:"automationId"`
	SourceID     string `json:"sourceId"`
	InstanceID   string `json:"instanceId"`
	State        string `json:"state"`
	Cursor       string `json:"cursor,omitempty"`
}

// LifecycleOutcomeResponse is the adapter-owned HTTP success shape for one
// lifecycle command outcome.
type LifecycleOutcomeResponse struct {
	Desired     string                    `json:"desired"`
	Observation SourceObservationResponse `json:"observation"`
	Convergence string                    `json:"convergence"`
	Idempotent  bool                      `json:"idempotent"`
}

// StartSourceInput carries decoded HTTP inputs for one Automations start-source
// operation owned by this adapter.
type StartSourceInput struct {
	AutomationID string
	SourceID     string
	Kind         string
	Resume       *SourceObservationResponse
}

// StopSourceInput carries decoded HTTP inputs for one Automations stop-source
// operation owned by this adapter.
type StopSourceInput struct {
	AutomationID string
	SourceID     string
}

// WaitSourceInput carries decoded HTTP inputs for one Automations wait-source
// operation owned by this adapter.
type WaitSourceInput struct {
	AutomationID string
	SourceID     string
	Desired      string
}

// SourceStatusInput carries decoded HTTP inputs for one Automations source-status
// operation owned by this adapter.
type SourceStatusInput struct {
	AutomationID string
	SourceID     string
}

// StartSourceResponse is the adapter-owned HTTP success shape for start-source.
type StartSourceResponse struct {
	Outcome LifecycleOutcomeResponse `json:"outcome"`
}

// StopSourceResponse is the adapter-owned HTTP success shape for stop-source.
type StopSourceResponse struct {
	Outcome LifecycleOutcomeResponse `json:"outcome"`
}

// WaitSourceResponse is the adapter-owned HTTP success shape for wait-source.
type WaitSourceResponse struct {
	Outcome LifecycleOutcomeResponse `json:"outcome"`
}

// SourceStatusResponse is the adapter-owned HTTP success shape for source-status.
type SourceStatusResponse struct {
	Observation SourceObservationResponse `json:"observation"`
}

// IsLifecycleBadRequest reports whether an error is a decode/validation failure
// that maps to a typed bad-request HTTP outcome before root invocation.
func IsLifecycleBadRequest(err error) bool {
	return errors.Is(err, ErrInvalidSourceIdentity) ||
		errors.Is(err, ErrInvalidSourceKind) ||
		errors.Is(err, ErrInvalidLifecycleDesired) ||
		errors.Is(err, ErrInvalidResumeObservation)
}

// StartSourceRequestFromHTTP maps one start-source HTTP request into the accepted
// Automations root request vocabulary.
func StartSourceRequestFromHTTP(input StartSourceInput) (automations.StartSourceRequest, error) {
	identity, err := sourceIdentityFromHTTP(input.AutomationID, input.SourceID)
	if err != nil {
		return automations.StartSourceRequest{}, err
	}
	kind := strings.TrimSpace(input.Kind)
	if kind == "" {
		return automations.StartSourceRequest{}, ErrInvalidSourceKind
	}
	request := automations.StartSourceRequest{
		Identity: identity,
		Kind:     kind,
	}
	if input.Resume == nil {
		return request, nil
	}
	resume, err := resumeObservationFromHTTP(identity, *input.Resume)
	if err != nil {
		return automations.StartSourceRequest{}, err
	}
	request.Resume = &resume
	return request, nil
}

// StopSourceRequestFromHTTP maps one stop-source HTTP request into the accepted
// Automations root request vocabulary.
func StopSourceRequestFromHTTP(input StopSourceInput) (automations.StopSourceRequest, error) {
	identity, err := sourceIdentityFromHTTP(input.AutomationID, input.SourceID)
	if err != nil {
		return automations.StopSourceRequest{}, err
	}
	return automations.StopSourceRequest{Identity: identity}, nil
}

// WaitSourceRequestFromHTTP maps one wait-source HTTP request into the accepted
// Automations root request vocabulary.
func WaitSourceRequestFromHTTP(input WaitSourceInput) (automations.WaitSourceRequest, error) {
	identity, err := sourceIdentityFromHTTP(input.AutomationID, input.SourceID)
	if err != nil {
		return automations.WaitSourceRequest{}, err
	}
	desired, err := desiredLifecycleFromHTTP(input.Desired)
	if err != nil {
		return automations.WaitSourceRequest{}, err
	}
	return automations.WaitSourceRequest{
		Identity: identity,
		Desired:  desired,
	}, nil
}

// SourceStatusRequestFromHTTP maps one source-status HTTP request into the
// accepted Automations root request vocabulary.
func SourceStatusRequestFromHTTP(input SourceStatusInput) (automations.SourceStatusRequest, error) {
	identity, err := sourceIdentityFromHTTP(input.AutomationID, input.SourceID)
	if err != nil {
		return automations.SourceStatusRequest{}, err
	}
	return automations.SourceStatusRequest{Identity: identity}, nil
}

// StartSourceResponseToHTTP encodes one fake-root start-source result into the
// adapter-owned HTTP success response shape.
func StartSourceResponseToHTTP(result automations.StartSourceResult) StartSourceResponse {
	return StartSourceResponse{Outcome: lifecycleOutcomeToHTTP(result.Outcome)}
}

// StopSourceResponseToHTTP encodes one fake-root stop-source result into the
// adapter-owned HTTP success response shape.
func StopSourceResponseToHTTP(result automations.StopSourceResult) StopSourceResponse {
	return StopSourceResponse{Outcome: lifecycleOutcomeToHTTP(result.Outcome)}
}

// WaitSourceResponseToHTTP encodes one fake-root wait-source result into the
// adapter-owned HTTP success response shape.
func WaitSourceResponseToHTTP(result automations.WaitSourceResult) WaitSourceResponse {
	return WaitSourceResponse{Outcome: lifecycleOutcomeToHTTP(result.Outcome)}
}

// SourceStatusResponseToHTTP encodes one fake-root source-status result into
// the adapter-owned HTTP success response shape.
func SourceStatusResponseToHTTP(result automations.SourceStatusResult) SourceStatusResponse {
	return SourceStatusResponse{Observation: sourceObservationToHTTP(result.Observation)}
}

func sourceIdentityFromHTTP(automationID, sourceID string) (automations.SourceIdentity, error) {
	automationID = strings.TrimSpace(automationID)
	sourceID = strings.TrimSpace(sourceID)
	if automationID == "" || sourceID == "" {
		return automations.SourceIdentity{}, ErrInvalidSourceIdentity
	}
	return automations.SourceIdentity{
		AutomationID: automationID,
		SourceID:     sourceID,
	}, nil
}

func desiredLifecycleFromHTTP(value string) (automations.DesiredLifecycleState, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(automations.DesiredLifecycleRunning):
		return automations.DesiredLifecycleRunning, nil
	case string(automations.DesiredLifecycleStopped):
		return automations.DesiredLifecycleStopped, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidLifecycleDesired, value)
	}
}

func resumeObservationFromHTTP(
	identity automations.SourceIdentity,
	input SourceObservationResponse,
) (automations.SourceObservation, error) {
	instanceID := strings.TrimSpace(input.InstanceID)
	state := strings.TrimSpace(input.State)
	if instanceID == "" || state == "" {
		return automations.SourceObservation{}, ErrInvalidResumeObservation
	}
	resumeIdentity, err := sourceIdentityFromHTTP(input.AutomationID, input.SourceID)
	if err != nil {
		return automations.SourceObservation{}, err
	}
	if resumeIdentity != identity {
		return automations.SourceObservation{}, fmt.Errorf(
			"%w: resume identity must match path identity",
			ErrInvalidResumeObservation,
		)
	}
	return automations.SourceObservation{
		Identity:   resumeIdentity,
		InstanceID: instanceID,
		State:      automations.ObservedLifecycleState(state),
		Cursor:     automations.Cursor(strings.TrimSpace(input.Cursor)),
	}, nil
}

func lifecycleOutcomeToHTTP(outcome automations.LifecycleOutcome) LifecycleOutcomeResponse {
	return LifecycleOutcomeResponse{
		Desired:     string(outcome.Desired),
		Observation: sourceObservationToHTTP(outcome.Observation),
		Convergence: string(outcome.Convergence),
		Idempotent:  outcome.Idempotent,
	}
}

func sourceObservationToHTTP(observation automations.SourceObservation) SourceObservationResponse {
	return SourceObservationResponse{
		AutomationID: observation.Identity.AutomationID,
		SourceID:     observation.Identity.SourceID,
		InstanceID:   observation.InstanceID,
		State:        string(observation.State),
		Cursor:       string(observation.Cursor),
	}
}
