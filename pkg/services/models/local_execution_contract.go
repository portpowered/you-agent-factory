package models

import (
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Provider identifies the model provider used for inference dispatch.
type Provider string

const (
	ProviderClaude   Provider = "claude"
	ProviderCodex    Provider = "codex"
	ProviderGemini   Provider = "gemini"
	ProviderKiro     Provider = "kiro-cli"
	ProviderCursor   Provider = "agent"
	ProviderOpenCode Provider = "opencode"
	ProviderPi       Provider = "pi"
	ProviderAgy      Provider = "agy"
)

// ErrUnsupportedResponseMode reports that an infer/local-invocation result
// cannot satisfy the requested response mode. It is distinct from readiness
// blocked outcomes (ErrMissing, ErrLoading, ErrFailed, ErrUnsupported) so peers
// can branch on typed infer failures through the root contract.
var ErrUnsupportedResponseMode = errors.New("model invocation response mode is not supported")

var (
	// ErrInvalidInferenceDependencies classifies Models Inference construction
	// failures.
	ErrInvalidInferenceDependencies = errors.New("model inference dependencies are invalid")
	// ErrInferenceCancelled reports that model inference was cancelled through
	// its context or the explicit cancellation operation.
	ErrInferenceCancelled = errors.New("model inference cancelled")
	// ErrInferenceTimeout reports that inference exceeded its Models-owned
	// execution deadline.
	ErrInferenceTimeout = errors.New("model inference timed out")
	// ErrInferenceFailed reports a normalized provider/runtime inference
	// failure that is not one of the more specific readiness or lease failures.
	ErrInferenceFailed = errors.New("model inference failed")
	// ErrInferenceArtifactInvalid reports an empty or malformed opaque output
	// artifact reference.
	ErrInferenceArtifactInvalid = errors.New("model inference artifact is invalid")
	// ErrInvocationNotFound reports an unknown invocation capability.
	ErrInvocationNotFound = errors.New("model invocation not found")
	// ErrUnsupportedModelOperation reports that the scoped model does not
	// support the requested operation.
	ErrUnsupportedModelOperation = errors.New("model operation is not supported")
)

// ResponseMode selects the representation returned by direct invocation.
type ResponseMode string

const (
	ResponseModeAudioStream ResponseMode = "AUDIO_STREAM"
)

// Options carries optional direct-invocation response behavior.
type Options struct {
	ResponseMode ResponseMode
}

// Request is the model-owned input for one direct invocation.
type Request struct {
	Operation string
	Content   []work.WorkContentPart
	Bindings  []ModelOperationBinding
	Options   *Options
}

// Result is the model-owned outcome of one direct invocation. Transport
// packages map it to public metadata or streamed response contracts.
type Result struct {
	ModelName         string
	Worker            string
	Operation         string
	ProviderLocality  string
	Content           []work.WorkContentPart
	Bindings          []ResolvedModelOperationBinding
	StreamFile        string
	StreamContentType string
}

// ModelInvocationRef is an opaque Models-owned invocation capability.
type ModelInvocationRef struct {
	value string
}

// Parse restores an invocation reference received from a trusted boundary.
func (ModelInvocationRef) Parse(value string) (ModelInvocationRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ModelInvocationRef{}, ErrInvocationNotFound
	}
	return ModelInvocationRef{value: value}, nil
}

// String serializes the opaque invocation reference.
func (ref ModelInvocationRef) String() string {
	return ref.value
}

// IsZero reports whether no invocation reference was supplied.
func (ref ModelInvocationRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// InferenceArtifactRef is an opaque reference to a Models-owned output
// artifact. It does not reveal a cache or filesystem location.
type InferenceArtifactRef struct {
	value string
}

// Parse restores an artifact reference received from a trusted boundary.
func (InferenceArtifactRef) Parse(value string) (InferenceArtifactRef, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return InferenceArtifactRef{}, ErrInferenceArtifactInvalid
	}
	return InferenceArtifactRef{value: value}, nil
}

// String serializes the opaque artifact reference.
func (ref InferenceArtifactRef) String() string {
	return ref.value
}

// IsZero reports whether no artifact reference was supplied.
func (ref InferenceArtifactRef) IsZero() bool {
	return strings.TrimSpace(ref.value) == ""
}

// InferenceInput is detached Models-owned invocation input.
type InferenceInput struct {
	ContentType string
	Content     string
}

// InferenceContent is detached Models-owned invocation output.
type InferenceContent struct {
	ContentType string
	Content     string
}

// InferenceArtifact contains peer-required output metadata without paths,
// readers, storage records, or runtime artifact handles.
type InferenceArtifact struct {
	Artifact   InferenceArtifactRef
	Name       string
	MediaType  string
	SizeBytes  int64
	Properties map[string]string
}

// Clone returns detached artifact metadata safe for a peer to retain.
func (artifact InferenceArtifact) Clone() InferenceArtifact {
	artifact.Properties = cloneStringMap(artifact.Properties)
	return artifact
}

// ModelInvocationStatus is the observable lifecycle of an invocation.
type ModelInvocationStatus string

const (
	ModelInvocationStatusAccepted  ModelInvocationStatus = "ACCEPTED"
	ModelInvocationStatusCompleted ModelInvocationStatus = "COMPLETED"
	ModelInvocationStatusFailed    ModelInvocationStatus = "FAILED"
	ModelInvocationStatusCancelled ModelInvocationStatus = "CANCELLED"
)

// InvocationLeaseDisposition reports what happened to capacity after an
// invocation outcome.
type InvocationLeaseDisposition string

const (
	InvocationLeaseRetained InvocationLeaseDisposition = "RETAINED"
	InvocationLeaseReleased InvocationLeaseDisposition = "RELEASED"
	InvocationLeaseExpired  InvocationLeaseDisposition = "EXPIRED"
)

// InvokeModelRequest asks Models to run one operation under an issued lease.
type InvokeModelRequest struct {
	Scope        RuntimeScopeRef
	Lease        ModelLeaseRef
	Holder       string
	ModelName    string
	Operation    string
	ResponseMode ResponseMode
	Input        InferenceInput
}

// Validate checks the peer-controlled invocation identity and capacity input.
func (request InvokeModelRequest) Validate() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Lease.IsZero() {
		return ErrHostLeaseNotFound
	}
	if strings.TrimSpace(request.Holder) == "" {
		return ErrHostInvalidHolder
	}
	if strings.TrimSpace(request.ModelName) == "" {
		return ErrNotFound
	}
	if strings.TrimSpace(request.Operation) == "" {
		return ErrUnsupportedModelOperation
	}
	return nil
}

// InvokeModelResult contains detached inference and lease-lifecycle facts.
type InvokeModelResult struct {
	Invocation       ModelInvocationRef
	Scope            RuntimeScopeRef
	Lease            ModelLeaseRef
	ModelName        string
	Operation        string
	Status           ModelInvocationStatus
	Content          []InferenceContent
	Artifacts        []InferenceArtifact
	LeaseDisposition InvocationLeaseDisposition
	// CancellationOutcome is populated when context cancellation ends an
	// accepted invocation, matching explicit CancelInvocation vocabulary.
	CancellationOutcome InvocationCancellationOutcome
}

// Clone returns a detached invocation result safe for a peer to retain.
func (result InvokeModelResult) Clone() InvokeModelResult {
	result.Content = append([]InferenceContent(nil), result.Content...)
	artifacts := result.Artifacts
	result.Artifacts = make([]InferenceArtifact, len(artifacts))
	for i := range artifacts {
		result.Artifacts[i] = artifacts[i].Clone()
	}
	return result
}

// CancelInvocationRequest identifies one invocation within its issuing scope.
type CancelInvocationRequest struct {
	Scope      RuntimeScopeRef
	Invocation ModelInvocationRef
}

// Validate checks the cancellation capability input.
func (request CancelInvocationRequest) Validate() error {
	if request.Scope.IsZero() {
		return ErrRuntimeScopeInvalid
	}
	if request.Invocation.IsZero() {
		return ErrInvocationNotFound
	}
	return nil
}

// InvocationCancellationOutcome distinguishes first, repeated, and late
// cancellation without parsing implementation-specific strings.
type InvocationCancellationOutcome string

const (
	InvocationCancellationRequested        InvocationCancellationOutcome = "CANCELLED"
	InvocationCancellationAlreadyCancelled InvocationCancellationOutcome = "ALREADY_CANCELLED"
	InvocationCancellationAlreadyCompleted InvocationCancellationOutcome = "ALREADY_COMPLETED"
)

// CancelInvocationResult reports the observable cancellation and capacity
// outcome for an issued invocation.
type CancelInvocationResult struct {
	Invocation       ModelInvocationRef
	Status           ModelInvocationStatus
	Outcome          InvocationCancellationOutcome
	LeaseDisposition InvocationLeaseDisposition
}

// ResolvedModelOperationBinding is the provider-neutral binding projection
// returned to model-invocation callers.
type ResolvedModelOperationBinding struct {
	Slot    string
	Source  string
	Content []work.WorkContentPart
}

type ModelOperationBinding struct {
	Slot           string                         `json:"slot" yaml:"slot"`
	Selector       *ModelOperationBindingSelector `json:"selector,omitempty" yaml:"selector,omitempty"`
	Config         []work.WorkContentPart         `json:"config,omitempty" yaml:"config,omitempty"`
	DefaultContent []work.WorkContentPart         `json:"defaultContent,omitempty" yaml:"defaultContent,omitempty"`
}

type ModelOperationBindingSelector struct {
	Slot  string `json:"slot,omitempty" yaml:"slot,omitempty"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
	Type  string `json:"type,omitempty" yaml:"type,omitempty"`
	Role  string `json:"role,omitempty" yaml:"role,omitempty"`
}

// TargetError retains model and worker identity while preserving the domain
// failure that an outward adapter classifies for its customer-facing surface.
type TargetError struct {
	ModelName  string
	WorkerName string
	Operation  string
	Cause      error
}

func (e *TargetError) Error() string {
	if e == nil || e.Cause == nil {
		return ""
	}
	return e.Cause.Error()
}

func (e *TargetError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// LocalInvocationRequest is the plain infer request on the Models root.
// Peers supply Worker/dispatch/bindings vocabulary without importing nested
// inference or local-execution implementation types.
type LocalInvocationRequest struct {
	Scope            RuntimeScopeRef
	Holder           string
	Worker           LocalWorker
	Resources        []LocalResource
	Dispatch         work.WorkDispatch
	ModelOperation   string
	ModelBindings    []ResolvedModelOperationBinding
	WorkingDirectory string
}

// LocalWorker is the Models-owned projection of the authored Worker fields
// required to select and invoke a managed local runtime.
type LocalWorker struct {
	Name          string
	Type          string
	Model         string
	ModelLocality string
	Resources     []LocalResource
}

func (worker LocalWorker) UsesManagedRuntime() bool {
	return RuntimeWorker{Type: worker.Type, ModelLocality: worker.ModelLocality}.UsesManagedRuntime()
}

// LocalResource is the Models-owned projection of a Factory resource.
type LocalResource struct {
	ID         string
	Name       string
	Type       string
	Capacity   int
	Model      string
	Backend    string
	LoadPolicy string
	Provider   string
}

// LocalInvocationResult is the plain infer result on the Models root. Handled
// true means Models owned the invocation; false means Models declined.
type LocalInvocationResult struct {
	Handled bool
	Content string
}

// ValidateLocalInvocationRequest checks the plain infer/local-invocation
// request. Managed-runtime workers with an empty Model fail closed.
func ValidateLocalInvocationRequest(request LocalInvocationRequest) error {
	if request.Worker.UsesManagedRuntime() && strings.TrimSpace(request.Worker.Model) == "" {
		return fmt.Errorf("%w: empty managed runtime model name", ErrNotFound)
	}
	return nil
}
