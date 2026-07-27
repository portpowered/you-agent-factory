package providers

import (
	"errors"
	"fmt"
)

// ErrUnknownProvider reports that a provider identity is not present in the
// catalog.
var ErrUnknownProvider = errors.New("provider is unknown")

// ErrProviderUnavailable reports that a catalog provider exists but is not
// selectable because availability or prerequisite facts block use.
var ErrProviderUnavailable = errors.New("provider is unavailable")

// Availability identifies why a catalog provider is or is not selectable.
// Support posture is static catalog metadata and does not imply live readiness.
type Availability string

const (
	AvailabilityCatalogOnly             Availability = "catalog-only"
	AvailabilityNotSupported            Availability = "not-supported"
	AvailabilitySupportedButUnavailable Availability = "supported-but-unavailable"
	AvailabilitySelectable              Availability = "selectable"
)

// Readiness identifies the current provider availability outcome.
type Readiness string

const (
	ReadinessReady       Readiness = "ready"
	ReadinessUnavailable Readiness = "unavailable"
	ReadinessDegraded    Readiness = "degraded"
)

// PrerequisiteKind identifies a sanitized requirement for provider readiness.
type PrerequisiteKind string

const (
	PrerequisiteConfiguration PrerequisiteKind = "configuration"
	PrerequisiteCredential    PrerequisiteKind = "credential"
	PrerequisiteDependency    PrerequisiteKind = "dependency"
)

// PrerequisiteStatus identifies whether a provider prerequisite is satisfied.
type PrerequisiteStatus string

const (
	PrerequisiteSatisfied PrerequisiteStatus = "satisfied"
	PrerequisiteMissing   PrerequisiteStatus = "missing"
)

// Prerequisite is a provider-neutral readiness requirement. Description is
// intended for bounded setup guidance, never raw environment or native output.
type Prerequisite struct {
	Kind        PrerequisiteKind
	Name        string
	Status      PrerequisiteStatus
	Description string
}

// Clone returns a detached prerequisite copy.
func (prerequisite Prerequisite) Clone() Prerequisite {
	return prerequisite
}

// Capability is one provider-neutral execution or response-fidelity feature.
type Capability string

const (
	CapabilityPromptSubmission   Capability = "prompt_submission"
	CapabilityImageInput         Capability = "image_input"
	CapabilitySessionResume      Capability = "session_resume"
	CapabilityStructuredOutput   Capability = "structured_output"
	CapabilityNativeStreaming    Capability = "native_streaming"
	CapabilityMessageDeltas      Capability = "message_deltas"
	CapabilityMessageSnapshots   Capability = "message_snapshots"
	CapabilityReasoningSummaries Capability = "reasoning_summaries"
	CapabilityToolLifecycle      Capability = "tool_lifecycle"
	CapabilityToolOutputDeltas   Capability = "tool_output_deltas"
	CapabilityFileChanges        Capability = "file_changes"
	CapabilityPlans              Capability = "plans"
	CapabilityUsage              Capability = "usage"
	CapabilityStableItemIDs      Capability = "stable_item_ids"
	CapabilityProviderReconnect  Capability = "provider_reconnect"
)

// ListProvidersRequest is the plain catalog list request vocabulary.
type ListProvidersRequest struct{}

// ListProvidersResult is the detached result of one catalog list.
type ListProvidersResult struct {
	Providers []Descriptor
}

// GetProviderRequest is the plain catalog get request. Peers identify a
// provider by Providers-owned ID without importing Workers provider registry
// types.
type GetProviderRequest struct {
	ID ID
}

// Validate checks request fields whose validity does not depend on catalog
// state.
func (request GetProviderRequest) Validate() error {
	if err := request.ID.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}
	return nil
}

// GetProviderResult is the detached result of one catalog lookup.
type GetProviderResult struct {
	Provider Descriptor
}

func clonePrerequisites(prerequisites []Prerequisite) []Prerequisite {
	if prerequisites == nil {
		return nil
	}
	cloned := make([]Prerequisite, len(prerequisites))
	copy(cloned, prerequisites)
	return cloned
}
