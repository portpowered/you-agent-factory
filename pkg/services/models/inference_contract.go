package models

import (
	"errors"

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
