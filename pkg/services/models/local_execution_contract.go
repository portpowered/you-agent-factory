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
