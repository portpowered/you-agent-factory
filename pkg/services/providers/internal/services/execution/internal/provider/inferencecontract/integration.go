package inferencecontract

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Identity is an opaque, stable provider integration identity.
type Identity string

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

// CapabilitySet is an immutable set of provider-neutral capabilities.
type CapabilitySet struct {
	values []Capability
}

// NewCapabilitySet creates a detached capability set. Duplicate capabilities
// are collapsed while preserving their first declaration order.
func NewCapabilitySet(values ...Capability) CapabilitySet {
	seen := make(map[Capability]struct{}, len(values))
	detached := make([]Capability, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		detached = append(detached, value)
	}
	return CapabilitySet{values: detached}
}

// Values returns a detached copy of the declared capabilities.
func (c CapabilitySet) Values() []Capability {
	return slices.Clone(c.values)
}

// Has reports whether the set contains capability.
func (c CapabilitySet) Has(capability Capability) bool {
	return slices.Contains(c.values, capability)
}

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
	kind        PrerequisiteKind
	name        string
	status      PrerequisiteStatus
	description string
}

// NewPrerequisite constructs one immutable readiness requirement.
func NewPrerequisite(kind PrerequisiteKind, name string, status PrerequisiteStatus, description string) Prerequisite {
	return Prerequisite{kind: kind, name: name, status: status, description: description}
}

func (p Prerequisite) Kind() PrerequisiteKind     { return p.kind }
func (p Prerequisite) Name() string               { return p.name }
func (p Prerequisite) Status() PrerequisiteStatus { return p.status }
func (p Prerequisite) Description() string        { return p.description }

// Discovery is an immutable, provider-neutral readiness report.
type Discovery struct {
	readiness     Readiness
	prerequisites []Prerequisite
}

// NewDiscovery constructs a detached readiness report.
func NewDiscovery(readiness Readiness, prerequisites ...Prerequisite) Discovery {
	return Discovery{readiness: readiness, prerequisites: slices.Clone(prerequisites)}
}

func (d Discovery) Readiness() Readiness { return d.readiness }

// Prerequisites returns a detached copy of the readiness requirements.
func (d Discovery) Prerequisites() []Prerequisite { return slices.Clone(d.prerequisites) }

// ProviderSession is optional generic provider-owned session metadata.
type ProviderSession struct {
	provider string
	kind     string
	id       string
	metadata map[string]string
}

// NewProviderSession creates detached provider-session metadata.
func NewProviderSession(provider, kind, id string, metadata map[string]string) ProviderSession {
	return ProviderSession{provider: provider, kind: kind, id: id, metadata: maps.Clone(metadata)}
}

func (s ProviderSession) Provider() string { return s.provider }
func (s ProviderSession) Kind() string     { return s.kind }
func (s ProviderSession) ID() string       { return s.id }

// Metadata returns a detached copy of generic provider-session metadata.
func (s ProviderSession) Metadata() map[string]string { return maps.Clone(s.metadata) }

// InvocationInput contains provider-neutral authored inference content.
type InvocationInput struct {
	InvocationID    string
	Model           string
	SystemPrompt    string
	UserMessage     string
	OutputSchema    string
	Required        CapabilitySet
	ProviderSession *ProviderSession
	Execution       workers.ProviderInferenceRequest
}

// InvocationRequest is an immutable provider invocation request.
type InvocationRequest struct {
	invocationID    string
	model           string
	systemPrompt    string
	userMessage     string
	outputSchema    string
	required        CapabilitySet
	providerSession *ProviderSession
	execution       workers.ProviderInferenceRequest
}

// NewInvocationRequest detaches all mutable input from the caller.
func NewInvocationRequest(input InvocationInput) InvocationRequest {
	return InvocationRequest{
		invocationID:    input.InvocationID,
		model:           input.Model,
		systemPrompt:    input.SystemPrompt,
		userMessage:     input.UserMessage,
		outputSchema:    input.OutputSchema,
		required:        NewCapabilitySet(input.Required.Values()...),
		providerSession: cloneProviderSession(input.ProviderSession),
		execution:       workers.CloneProviderInferenceRequest(input.Execution),
	}
}

func (r InvocationRequest) InvocationID() string { return r.invocationID }
func (r InvocationRequest) Model() string        { return r.model }
func (r InvocationRequest) SystemPrompt() string { return r.systemPrompt }
func (r InvocationRequest) UserMessage() string  { return r.userMessage }
func (r InvocationRequest) OutputSchema() string { return r.outputSchema }
func (r InvocationRequest) RequiredCapabilities() CapabilitySet {
	return NewCapabilitySet(r.required.Values()...)
}
func (r InvocationRequest) ProviderSession() *ProviderSession {
	return cloneProviderSession(r.providerSession)
}

// Execution returns a detached worker execution context for provider-owned
// process construction. It preserves runtime configuration while the
// conductor retains ownership of provider selection and response delivery.
func (r InvocationRequest) Execution() workers.ProviderInferenceRequest {
	return workers.CloneProviderInferenceRequest(r.execution)
}

// FailureKind is a provider-neutral invocation failure category.
type FailureKind string

const (
	FailureAuthentication  FailureKind = "authentication"
	FailureInvalidRequest  FailureKind = "invalid_request"
	FailureThrottled       FailureKind = "throttled"
	FailureTimeout         FailureKind = "timeout"
	FailureCanceled        FailureKind = "canceled"
	FailureDependency      FailureKind = "dependency"
	FailureMalformedOutput FailureKind = "malformed_output"
	FailureUnknown         FailureKind = "unknown"
)

// FailureInput contains normalized, customer-safe failure facts.
type FailureInput struct {
	Kind            FailureKind
	Message         string
	Retryable       bool
	ProviderSession *ProviderSession
	Diagnostics     map[string]string
}

// Failure is an immutable normalized provider failure.
type Failure struct {
	kind            FailureKind
	message         string
	retryable       bool
	providerSession *ProviderSession
	diagnostics     map[string]string
}

// NewFailure creates a detached normalized provider failure.
func NewFailure(input FailureInput) Failure {
	return Failure{
		kind:            input.Kind,
		message:         input.Message,
		retryable:       input.Retryable,
		providerSession: cloneProviderSession(input.ProviderSession),
		diagnostics:     maps.Clone(input.Diagnostics),
	}
}

func (f Failure) Kind() FailureKind                 { return f.kind }
func (f Failure) Message() string                   { return f.message }
func (f Failure) Retryable() bool                   { return f.retryable }
func (f Failure) ProviderSession() *ProviderSession { return cloneProviderSession(f.providerSession) }
func (f Failure) Diagnostics() map[string]string    { return maps.Clone(f.diagnostics) }

// ResponseInput contains the authoritative provider response.
type ResponseInput struct {
	Content         string
	ProviderSession *ProviderSession
	Metadata        map[string]string
}

// Response is an immutable authoritative provider response.
type Response struct {
	content         string
	providerSession *ProviderSession
	metadata        map[string]string
}

// NewResponse creates a detached authoritative response.
func NewResponse(input ResponseInput) Response {
	return Response{
		content:         input.Content,
		providerSession: cloneProviderSession(input.ProviderSession),
		metadata:        maps.Clone(input.Metadata),
	}
}

func (r Response) Content() string                   { return r.content }
func (r Response) ProviderSession() *ProviderSession { return cloneProviderSession(r.providerSession) }
func (r Response) Metadata() map[string]string       { return maps.Clone(r.metadata) }

// Completion contains one authoritative response or normalized failure.
// SuccessfulCompletion and FailedCompletion construct the valid outcomes;
// protocol validation rejects the empty zero value.
type Completion struct {
	response *Response
	failure  *Failure
}

// SuccessfulCompletion constructs a successful terminal outcome.
func SuccessfulCompletion(response Response) Completion {
	clone := NewResponse(ResponseInput{
		Content:         response.Content(),
		ProviderSession: response.ProviderSession(),
		Metadata:        response.Metadata(),
	})
	return Completion{response: &clone}
}

// FailedCompletion constructs a failed terminal outcome.
func FailedCompletion(failure Failure) Completion {
	clone := NewFailure(FailureInput{
		Kind:            failure.Kind(),
		Message:         failure.Message(),
		Retryable:       failure.Retryable(),
		ProviderSession: failure.ProviderSession(),
		Diagnostics:     failure.Diagnostics(),
	})
	return Completion{failure: &clone}
}

func (c Completion) Response() *Response {
	if c.response == nil {
		return nil
	}
	clone := SuccessfulCompletion(*c.response)
	return clone.response
}

func (c Completion) Failure() *Failure {
	if c.failure == nil {
		return nil
	}
	clone := FailedCompletion(*c.failure)
	return clone.failure
}

// EventDraftInput contains the provider-writable portion of the Workers-owned
// response-event draft vocabulary. Factory Session identity, dispatch identity,
// publication sequence, timestamps, and replay gaps are intentionally absent.
type EventDraftInput struct {
	RunID              string
	Kind               workers.Kind
	Phase              workers.Phase
	Provenance         workers.Provenance
	Payload            []byte
	TurnID             string
	ItemID             string
	ParentItemID       string
	ProviderSessionRef string
}

// EventDraft is an immutable provider-owned semantic response-event draft.
type EventDraft struct {
	draft workers.Draft
}

// NewEventDraft constructs a detached semantic draft. STREAM_GAP is reserved
// for Factory Session publication and cannot be authored by a provider.
func NewEventDraft(input EventDraftInput) (EventDraft, error) {
	if input.Kind == workers.KindStreamGap {
		return EventDraft{}, errors.New("STREAM_GAP is reserved for Factory Session publication")
	}
	return EventDraft{draft: workers.CloneDraft(workers.Draft{
		RunID:              input.RunID,
		Kind:               input.Kind,
		Phase:              input.Phase,
		Provenance:         input.Provenance,
		Payload:            input.Payload,
		TurnID:             input.TurnID,
		ItemID:             input.ItemID,
		ParentItemID:       input.ParentItemID,
		ProviderSessionRef: input.ProviderSessionRef,
	})}, nil
}

// Draft returns a detached Workers-owned semantic draft for orchestration.
func (e EventDraft) Draft() workers.Draft { return workers.CloneDraft(e.draft) }

// ResponseWriter accepts provider-owned semantic drafts and one authoritative
// completion. Factory Session envelopes, publication order, and retention are
// deliberately outside this contract.
type ResponseWriter interface {
	WriteEvent(context.Context, EventDraft) error
	Close(context.Context, Completion) error
}

// Integration is the single customer-implementable provider protocol.
type Integration interface {
	Identity() Identity
	MaximumCapabilities() CapabilitySet
	Discover(context.Context) (Discovery, error)
	Capabilities(context.Context, InvocationRequest) (CapabilitySet, error)
	Invoke(context.Context, InvocationRequest, ResponseWriter) error
}

func cloneProviderSession(session *ProviderSession) *ProviderSession {
	if session == nil {
		return nil
	}
	clone := NewProviderSession(session.Provider(), session.Kind(), session.ID(), session.Metadata())
	return &clone
}
