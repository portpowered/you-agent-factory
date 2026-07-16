// Package adapter defines the provider-neutral subprocess response adapter
// kernel. It contains semantic provider boundaries, not CLI or HTTP rendering.
package adapter

import (
	"context"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work/materialize"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

// Identity is the stable registry key for one provider adapter.
type Identity string

// CommandContext contains invocation facts used to construct one subprocess
// request. Command execution remains owned by orchestration.
type CommandContext struct {
	Request            workerexecution.ProviderInferenceRequest
	SkipPermissions    bool
	MaterializeOptions *materialize.Options
}

// CommandBuildResult is the complete subprocess request produced by an adapter.
type CommandBuildResult struct {
	Request workerprocess.CommandRequest
	Cleanup func()
}

// DecoderContext scopes a stateful decoder to one provider invocation.
type DecoderContext struct {
	RunID      string
	DispatchID string
}

// OutputStream identifies independently observed subprocess output channels.
type OutputStream string

const (
	OutputStreamStdout OutputStream = "stdout"
	OutputStreamStderr OutputStream = "stderr"
)

// Observation is one ordered chunk from exactly one subprocess output stream.
type Observation struct {
	Stream OutputStream
	Chunk  []byte
}

// FlushReason explains why orchestration is draining buffered decoder state.
type FlushReason string

const (
	FlushReasonCompleted  FlushReason = "completed"
	FlushReasonTerminated FlushReason = "terminated"
	FlushReasonCanceled   FlushReason = "canceled"
)

// FlushContext carries the terminal fact used to drain buffered decoder state.
type FlushContext struct {
	Reason FlushReason
}

// Diagnostic is bounded, provider-safe information about ignored or malformed
// native input. Message must not contain submitted prompts or raw payloads.
type Diagnostic struct {
	Code    string
	Message string
}

// DecodeResult contains canonical drafts and safe diagnostics emitted while
// observing one invocation. Publication metadata is intentionally absent.
type DecodeResult struct {
	Drafts      []responseevents.Draft
	Diagnostics []Diagnostic
}

// Decoder owns native stream parsing state for exactly one invocation.
type Decoder interface {
	Observe(context.Context, Observation) (DecodeResult, error)
	Flush(context.Context, FlushContext) (DecodeResult, error)
}

// FinalParseContext carries completed subprocess and invocation-correlation
// facts to the authoritative final-result parser after decoder buffers flush.
type FinalParseContext struct {
	CommandResult workerprocess.CommandResult
	CommandError  error
	FlushReason   FlushReason
	RunID         string
	DispatchID    string
}

// FinalParseResult is the authoritative provider result and any semantic drafts
// that can only be produced after command completion.
type FinalParseResult struct {
	Response workerexecution.InferenceResponse
	Drafts   []responseevents.Draft
}

// CapabilityContext permits model- or invocation-specific capability reports.
type CapabilityContext struct {
	Request workerexecution.ProviderInferenceRequest
}

// Capabilities describes observed semantic fidelity without exposing native
// event names or transport presentation policy.
type Capabilities struct {
	NativeStreaming    bool
	MessageDeltas      bool
	MessageSnapshots   bool
	ReasoningSummaries bool
	ToolLifecycle      bool
	ToolOutputDeltas   bool
	StableItemIDs      bool
	FinalOnly          bool
}

// CapabilityResult reports the adapter's capability profile.
type CapabilityResult struct {
	Capabilities Capabilities
}

// FailureContext contains invocation outcomes used for neutral classification.
type FailureContext struct {
	CommandResult workerprocess.CommandResult
	CommandError  error
	DecodeError   error
	FlushError    error
	ParseError    error
	FlushReason   FlushReason
}

// RetryGuidance reports provider-neutral retry facts. Retry counts, backoff,
// scheduling, CLI exits, and HTTP statuses remain caller-owned policy.
type RetryGuidance struct {
	Retryable  bool
	RetryAfter *time.Duration
}

// FailureFacts is the bounded normalized failure returned by an adapter.
type FailureFacts struct {
	Family          workerexecution.WorkFailureFamily
	Type            workerexecution.WorkFailureType
	Message         string
	Retry           RetryGuidance
	ProviderSession *workerexecution.ProviderSessionMetadata
}

// FailureResult reports whether the invocation failed and, when it did, the
// normalized facts callers use for retry and terminal outcome policy.
type FailureResult struct {
	Failure *FailureFacts
}

// FallbackContext contains the completed first-attempt facts an adapter may
// use to decide whether a second subprocess launch is provably safe. Native
// syntax remains provider-owned; orchestration only enforces a single retry.
type FallbackContext struct {
	CommandResult workerprocess.CommandResult
	CommandError  error
	DecodeError   error
	FlushError    error
	ParseError    error
	FlushReason   FlushReason
	Drafts        []responseevents.Draft
	Diagnostics   []Diagnostic
}

// FallbackPlan selects the adapter for one final fallback attempt and carries
// the bounded degradation signal published with its capability update.
type FallbackPlan struct {
	Adapter    Adapter
	Diagnostic Diagnostic
}

// FallbackPlanner is an optional provider-owned extension. Execute consults it
// once after the first attempt and never recursively retries its returned
// adapter.
type FallbackPlanner interface {
	PlanFallback(context.Context, FallbackContext) (FallbackPlan, bool, error)
}

// Adapter separates every provider-owned operation needed by neutral
// subprocess orchestration. Implementations interpret native syntax; callers
// consume only the typed results defined here.
type Adapter interface {
	Identity() Identity
	BuildCommand(context.Context, CommandContext) (CommandBuildResult, error)
	NewDecoder(context.Context, DecoderContext) (Decoder, error)
	ParseFinal(context.Context, FinalParseContext) (FinalParseResult, error)
	Capabilities(context.Context, CapabilityContext) (CapabilityResult, error)
	ClassifyFailure(context.Context, FailureContext) FailureResult
}
