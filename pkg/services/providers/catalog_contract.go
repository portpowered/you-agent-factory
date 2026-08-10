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

// Readiness identifies the current provider availability outcome, or explains
// that the catalog has not performed a request-time readiness probe.
type Readiness string

const (
	ReadinessReady       Readiness = "ready"
	ReadinessUnavailable Readiness = "unavailable"
	ReadinessDegraded    Readiness = "degraded"
	// ReadinessUnverified means that the descriptor contains static catalog
	// facts only; the current machine, account, and executable have not been
	// probed.
	ReadinessUnverified Readiness = "unverified"
)

// TechnicalSupportLevel is the maintainer-verified support posture published
// by the authored provider manifest. It is independent from current-machine
// readiness.
type TechnicalSupportLevel string

const (
	TechnicalSupportProduction   TechnicalSupportLevel = "production"
	TechnicalSupportExperimental TechnicalSupportLevel = "experimental"
	TechnicalSupportNotSupported TechnicalSupportLevel = "not-supported"
)

// ImplementationAvailability identifies how the provider integration is
// supplied by the published package.
type ImplementationAvailability string

const (
	ImplementationBundled            ImplementationAvailability = "bundled"
	ImplementationExternallySupplied ImplementationAvailability = "externally-supplied"
	ImplementationCatalogOnly        ImplementationAvailability = "catalog-only"
)

// PrerequisiteKind identifies a sanitized requirement for provider readiness.
type PrerequisiteKind string

const (
	PrerequisiteConfiguration  PrerequisiteKind = "configuration"
	PrerequisiteCredential     PrerequisiteKind = "credential"
	PrerequisiteDependency     PrerequisiteKind = "dependency"
	PrerequisiteExecutable     PrerequisiteKind = "executable"
	PrerequisiteAuthentication PrerequisiteKind = "authentication"
	PrerequisiteWorkspace      PrerequisiteKind = "workspace"
)

// PrerequisiteStatus identifies whether a provider prerequisite is known to be
// satisfied, missing, or merely required by the static catalog.
type PrerequisiteStatus string

const (
	PrerequisiteSatisfied PrerequisiteStatus = "satisfied"
	PrerequisiteMissing   PrerequisiteStatus = "missing"
	// PrerequisiteRequired is an authored requirement whose current state has
	// not been checked by a request-time readiness probe.
	PrerequisiteRequired PrerequisiteStatus = "required"
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

// ModalityDirection identifies whether a provider model consumes or emits a
// modality.
type ModalityDirection string

const (
	ModalityDirectionInput  ModalityDirection = "input"
	ModalityDirectionOutput ModalityDirection = "output"
)

// ModalityKind identifies one provider model input or output category.
type ModalityKind string

const (
	ModalityText  ModalityKind = "text"
	ModalityImage ModalityKind = "image"
	ModalityAudio ModalityKind = "audio"
	ModalityVideo ModalityKind = "video"
)

// ModalitySupport distinguishes an explicitly unsupported modality from an
// omitted or unknown fact.
type ModalitySupport string

const (
	ModalitySupported   ModalitySupport = "supported"
	ModalityUnsupported ModalitySupport = "unsupported"
)

// ModalityTransport identifies how a supported modality crosses the provider
// boundary. Unsupported modalities use ModalityTransportNone.
type ModalityTransport string

const (
	ModalityTransportInline   ModalityTransport = "inline"
	ModalityTransportFilePath ModalityTransport = "file_path"
	ModalityTransportNone     ModalityTransport = "none"
)

// Modality is one explicit directional provider-model fact.
type Modality struct {
	Direction ModalityDirection
	Kind      ModalityKind
	Support   ModalitySupport
	Transport ModalityTransport
}

// Clone returns a detached modality value.
func (modality Modality) Clone() Modality {
	return modality
}

// ModelDescriptor contains the capability facts for one provider model.
type ModelDescriptor struct {
	ID         string
	Efforts    []ReasoningEffort
	Modalities []Modality
}

// Clone returns a detached model descriptor copy.
func (model ModelDescriptor) Clone() ModelDescriptor {
	model.Efforts = append([]ReasoningEffort(nil), model.Efforts...)
	model.Modalities = append([]Modality(nil), model.Modalities...)
	return model
}

// ToolSupport distinguishes an available named tool from an explicit
// unsupported tool fact.
type ToolSupport string

const (
	ToolSupported   ToolSupport = "supported"
	ToolUnsupported ToolSupport = "unsupported"
)

// Tool is one named provider tool fact.
type Tool struct {
	Name        string
	Support     ToolSupport
	Description string
}

// Clone returns a detached tool value.
func (tool Tool) Clone() Tool {
	return tool
}

// KnownLimitKind identifies the meaning of a known limit value.
type KnownLimitKind string

const (
	KnownLimitMaximum  KnownLimitKind = "maximum"
	KnownLimitDefault  KnownLimitKind = "default"
	KnownLimitBehavior KnownLimitKind = "behavior"
)

// KnownLimit is a named, bounded provider constraint or documented behavior.
// Numeric values are pointers so zero means "not published", not an invalid
// or silently omitted limit.
type KnownLimit struct {
	Name        string
	Kind        KnownLimitKind
	Unit        string
	Description string
	Maximum     *int64
	Default     *int64
	Value       string
}

// Clone returns a detached known-limit copy.
func (limit KnownLimit) Clone() KnownLimit {
	limit.Maximum = cloneInt64(limit.Maximum)
	limit.Default = cloneInt64(limit.Default)
	return limit
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
