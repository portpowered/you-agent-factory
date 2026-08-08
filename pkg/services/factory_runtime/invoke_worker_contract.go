package factory

import (
	"errors"
	"strings"
)

// InvokeWorkerRequest names one Worker an orchestrator has already resolved.
//
// It carries selections rather than a workstation because its Worker has no
// authored workstation: a JavaScript workflow child holds a literal prompt and
// an explicit model, so there is nothing for workstation prompt rendering,
// worktree preparation, or colour parsing to act on.
type InvokeWorkerRequest struct {
	// DispatchID is the orchestrator-minted identity for this Worker. It
	// becomes the Worker Session identity too, exactly as a Petri dispatch's
	// does, so one dispatch is never two Workers.
	DispatchID string
	// Label is the human-facing name for this Worker, used only for
	// observation.
	Label string
	// Prompt is the fully resolved user message. Runtime does not render,
	// template, or augment it.
	Prompt string
	// SystemPrompt is optional already-resolved system context.
	SystemPrompt string
	// Model, ModelProvider, and ReasoningEffort are the caller's resolved
	// selections. Runtime does not substitute defaults for them: a Worker whose
	// model was chosen by its orchestrator must not silently run on another.
	Model           string
	ModelProvider   string
	ReasoningEffort string
	// ExecutorProvider and RunnerID select the provider integration. An empty
	// RunnerID is derived from the executor and model providers.
	ExecutorProvider string
	RunnerID         string
	// OutputSchema is the optional JSON schema the provider must satisfy.
	OutputSchema string
	// WorkingDirectory scopes provider execution. Empty selects the runtime's
	// own working directory.
	WorkingDirectory string
	// SkipPermissions is the invocation-effective worker policy, already
	// resolved by the caller.
	SkipPermissions bool
	// MaxAttempts bounds provider attempts, not retries after the first. Zero
	// and one are both a single attempt.
	MaxAttempts int
}

// Validate reports whether req names a runnable Worker. Validate is pure and
// does not mutate req.
func (req InvokeWorkerRequest) Validate() error {
	if strings.TrimSpace(req.DispatchID) == "" {
		return ErrInvalidInvokeWorkerRequest
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return ErrInvalidInvokeWorkerRequest
	}
	if req.MaxAttempts < 0 {
		return ErrInvalidInvokeWorkerRequest
	}
	return nil
}

// InvokeWorkerOutcome classifies one completed Worker invocation.
type InvokeWorkerOutcome string

const (
	InvokeWorkerOutcomeCompleted InvokeWorkerOutcome = "COMPLETED"
	InvokeWorkerOutcomeFailed    InvokeWorkerOutcome = "FAILED"
	InvokeWorkerOutcomeCanceled  InvokeWorkerOutcome = "CANCELED"
)

// InvokeWorkerResult is the detached outcome of one Worker invocation.
type InvokeWorkerResult struct {
	// DispatchID and WorkerSessionID are the same value, retained as separate
	// fields so callers correlating either identity do not have to know they
	// coincide.
	DispatchID      string
	WorkerSessionID string
	Outcome         InvokeWorkerOutcome
	// Output is the provider's content for a completed Worker.
	Output string
	// ProviderSessionRef and Provider retain the Providers-owned identity the
	// attempt observed, when one was reported.
	Provider           string
	ProviderSessionRef string
	// Diagnostic is a bounded, safe failure description. It never carries
	// provider command lines, paths, or credentials.
	Diagnostic string
	// FailureReason is the Workers-owned closed classification for a failed
	// attempt, and Retryable is that classification's retry verdict. Both are
	// bounded vocabulary rather than provider text, which is why they may cross
	// this boundary when the provider's own message may not. They are what a
	// caller's public dispatch record reports as the cause.
	FailureReason string
	Retryable     *bool
	// Attempts is the number of provider attempts actually made.
	Attempts int
}

// ErrInvalidInvokeWorkerRequest reports a malformed Worker invocation request.
var ErrInvalidInvokeWorkerRequest = errors.New("factory runtime: invalid worker invocation request")
