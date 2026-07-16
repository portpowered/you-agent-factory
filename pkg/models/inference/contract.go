// Package inference owns provider-neutral direct model invocation contracts.
package inference

import (
	"errors"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// ErrUnsupportedResponseMode reports that an invocation result cannot satisfy
// the requested response mode.
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
	Bindings  []interfaces.ModelOperationBinding
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
	Bindings          []workerexecution.ResolvedModelOperationBinding
	StreamFile        string
	StreamContentType string
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
