package providers

import (
	"errors"
	"fmt"
	"strings"
)

// ErrExecuteCancelled reports that one provider execution attempt was cancelled
// through context cancellation or an explicit provider-side cancel outcome.
var ErrExecuteCancelled = errors.New("provider execution cancelled")

// ErrExecuteTimeout reports that one provider execution attempt exceeded its
// deadline.
var ErrExecuteTimeout = errors.New("provider execution timed out")

// ErrExecuteFailed reports a normalized provider execution failure that is not
// one of the more specific cancellation or timeout outcomes.
var ErrExecuteFailed = errors.New("provider execution failed")

// ExecuteFailureKind is a provider-neutral one-attempt failure category.
// Providers owns the normalized attempt outcome; Workers and other callers own
// retry, throttle, and scheduling policy.
type ExecuteFailureKind string

const (
	ExecuteFailureKindCanceled       ExecuteFailureKind = "canceled"
	ExecuteFailureKindTimeout        ExecuteFailureKind = "timeout"
	ExecuteFailureKindAuthentication ExecuteFailureKind = "authentication"
	ExecuteFailureKindInvalidRequest ExecuteFailureKind = "invalid_request"
	ExecuteFailureKindMisconfigured  ExecuteFailureKind = "misconfigured"
	ExecuteFailureKindThrottled      ExecuteFailureKind = "throttled"
	ExecuteFailureKindDependency     ExecuteFailureKind = "dependency"
	ExecuteFailureKindUnknown        ExecuteFailureKind = "unknown"
)

// ExecuteFailure retains normalized one-attempt failure facts peers can branch
// on with errors.Is / errors.As without importing Workers provider internals.
// SessionRef carries an optional detached provider session established before
// the attempt failed.
type ExecuteFailure struct {
	Kind        ExecuteFailureKind
	Message     string
	SessionRef  *SessionRef
	Diagnostics *ExecuteDiagnostics
}

func (failure ExecuteFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		return sentinelForExecuteFailureKind(failure.Kind).Error()
	}
	return fmt.Sprintf("%s: %s", sentinelForExecuteFailureKind(failure.Kind).Error(), message)
}

func (failure ExecuteFailure) Unwrap() error {
	return sentinelForExecuteFailureKind(failure.Kind)
}

// Clone returns a detached execute-failure copy.
func (failure ExecuteFailure) Clone() ExecuteFailure {
	if failure.SessionRef != nil {
		session := failure.SessionRef.Clone()
		failure.SessionRef = &session
	}
	if failure.Diagnostics != nil {
		diagnostics := failure.Diagnostics.Clone()
		failure.Diagnostics = &diagnostics
	}
	return failure
}

func sentinelForExecuteFailureKind(kind ExecuteFailureKind) error {
	switch kind {
	case ExecuteFailureKindCanceled:
		return ErrExecuteCancelled
	case ExecuteFailureKindTimeout:
		return ErrExecuteTimeout
	default:
		return ErrExecuteFailed
	}
}

// ExecuteRequest is the plain one-attempt execute vocabulary. Providers owns
// exactly one normalized native attempt per call; callers own selection,
// retry, throttle, and scheduling policy.
type ExecuteRequest struct {
	Provider           ID
	AttemptID          string
	WorkerType         string
	WorkstationName    string
	Model              string
	ReasoningEffort    string
	SkipPermissions    bool
	SystemPrompt       string
	UserMessage        string
	InputTokens        []any
	OutputSchema       string
	ResumeSession      *SessionRef
	WorkingDirectory   string
	Worktree           string
	EnvVars            map[string]string
	ProcessEnvironment []string
}

// Validate checks request fields whose validity does not depend on catalog
// state.
func (request ExecuteRequest) Validate() error {
	if err := request.Provider.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return fmt.Errorf("%w: empty attempt id", ErrExecuteFailed)
	}
	if _, ok := ReasoningEffort(request.ReasoningEffort).Canonical(); !ok {
		return fmt.Errorf("%w: unsupported reasoning effort %q", ErrExecuteFailed, request.ReasoningEffort)
	}
	if request.ResumeSession != nil {
		if err := request.ResumeSession.Validate(); err != nil {
			return fmt.Errorf("%w", err)
		}
	}
	return nil
}

// ReasoningEffort is a provider-neutral inference effort value.
type ReasoningEffort string

// Canonical trims and lowercases a supported inference effort.
func (value ReasoningEffort) Canonical() (string, bool) {
	canonical := strings.ToLower(strings.TrimSpace(string(value)))
	switch canonical {
	case "", "minimal", "low", "medium", "high", "xhigh", "max":
		return canonical, true
	default:
		return "", false
	}
}

// Clone returns a detached execute-request copy.
func (request ExecuteRequest) Clone() ExecuteRequest {
	cloned := request
	cloned.EnvVars = cloneStringMap(request.EnvVars)
	if request.ProcessEnvironment != nil {
		cloned.ProcessEnvironment = append([]string(nil), request.ProcessEnvironment...)
	}
	if request.InputTokens != nil {
		cloned.InputTokens = append([]any(nil), request.InputTokens...)
	}
	if request.ResumeSession != nil {
		resume := request.ResumeSession.Clone()
		cloned.ResumeSession = &resume
	}
	return cloned
}

// ExecuteProgress carries bounded in-flight progress facts for one attempt.
// Providers exposes this detached value for peer contracts without requiring
// transport or Workers provider streaming types.
type ExecuteProgress struct {
	Phase    string
	Detail   string
	Metadata map[string]string
}

// Clone returns a detached progress copy.
func (progress ExecuteProgress) Clone() ExecuteProgress {
	progress.Metadata = cloneStringMap(progress.Metadata)
	return progress
}

// ExecuteDiagnostics carries sanitized one-attempt diagnostic facts on success
// or failure without raw provider command output.
type ExecuteDiagnostics struct {
	DurationMillis int64
	Progress       []ExecuteProgress
	Metadata       map[string]string
}

// Clone returns a detached diagnostics copy.
func (diagnostics ExecuteDiagnostics) Clone() ExecuteDiagnostics {
	progress := diagnostics.Progress
	diagnostics.Progress = make([]ExecuteProgress, len(progress))
	for i := range progress {
		diagnostics.Progress[i] = progress[i].Clone()
	}
	diagnostics.Metadata = cloneStringMap(diagnostics.Metadata)
	return diagnostics
}

// ExecuteResult is the detached result of one normalized provider attempt.
type ExecuteResult struct {
	Content     string
	SessionRef  *SessionRef
	Diagnostics *ExecuteDiagnostics
}

// Clone returns a detached execute-result copy.
func (result ExecuteResult) Clone() ExecuteResult {
	cloned := result
	if result.SessionRef != nil {
		session := result.SessionRef.Clone()
		cloned.SessionRef = &session
	}
	if result.Diagnostics != nil {
		diagnostics := result.Diagnostics.Clone()
		cloned.Diagnostics = &diagnostics
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// ErrInvalidControlRequest reports that a control-attempt request has an
// empty attempt id or an unrecognized control action. Empty provider identity
// fails with ErrInvalidID for consistency with the rest of the root contract.
var ErrInvalidControlRequest = errors.New("provider control request is invalid")

// ErrControlSignalFailed reports that a claimed, truthfully-supported control
// signal was not accepted by its owning live attempt for a genuine operation
// reason (for example a broken ACP connection or a timed-out notification
// send). It is returned as an error, distinct from the successful
// ControlOutcomeUnsupported result: the capability was truthfully live, the
// live registration was already removed, and the signal attempt itself
// failed.
var ErrControlSignalFailed = errors.New("provider control signal was not accepted")

// ControlAction is the closed Providers-owned attempt-control action
// vocabulary. Peers branch on these typed values instead of provider-specific
// control strings.
type ControlAction string

const (
	ControlActionPause     ControlAction = "pause"
	ControlActionCancel    ControlAction = "cancel"
	ControlActionTerminate ControlAction = "terminate"
)

// Validate checks that action is one of the closed, non-zero control-action
// values.
func (action ControlAction) Validate() error {
	switch action {
	case ControlActionPause, ControlActionCancel, ControlActionTerminate:
		return nil
	default:
		return fmt.Errorf("%w: unsupported control action %q", ErrInvalidControlRequest, string(action))
	}
}

// ControlOutcome is the closed Providers-owned attempt-control outcome
// vocabulary. Unsupported is a successful capability result distinct from a
// request-validation error or a genuine operation failure.
type ControlOutcome string

const (
	ControlOutcomeCompleted   ControlOutcome = "completed"
	ControlOutcomeUnsupported ControlOutcome = "unsupported"
)

// ControlAttemptRequest identifies one Providers-owned provider attempt and
// the requested pause, cancel, or terminate action.
type ControlAttemptRequest struct {
	Provider  ID
	AttemptID string
	Action    ControlAction
}

// Validate checks that Provider is non-empty, AttemptID is non-empty after
// trimming, and Action is one of the closed control-action values.
func (request ControlAttemptRequest) Validate() error {
	if err := request.Provider.Validate(); err != nil {
		return fmt.Errorf("%w", err)
	}
	if strings.TrimSpace(request.AttemptID) == "" {
		return fmt.Errorf("%w: empty attempt id", ErrInvalidControlRequest)
	}
	return request.Action.Validate()
}

// ControlAttemptResult echoes the requested provider, attempt, and action
// alongside the closed completed/unsupported outcome. Every field is a plain
// value, so a result is always detached and safe to hold or compare directly.
type ControlAttemptResult struct {
	Provider  ID
	AttemptID string
	Action    ControlAction
	Outcome   ControlOutcome
}
