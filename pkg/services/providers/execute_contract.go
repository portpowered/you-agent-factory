package providers

import (
	"errors"
	"fmt"
	"strings"
	"time"
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
	// ExecuteFailureKindSessionNotFound reports that a resolved provider did
	// not recognize the exact requested Provider Session id as live. Ordinary
	// Execute never produces this kind: only a private Continue-triggered
	// attempt can observe a prior session. Continue translates this
	// kind into the typed stale continuation failure before returning to its
	// caller, so this kind never reaches a Continue caller directly either.
	ExecuteFailureKindSessionNotFound ExecuteFailureKind = "session_not_found"
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
	Provider        ID
	AttemptID       string
	WorkerType      string
	WorkstationName string
	Model           string
	ReasoningEffort string
	SkipPermissions bool
	// PrintTimeout carries an invocation's requested native print limit. It is
	// kept as provider-neutral execution metadata so a native adapter can
	// forward it without importing worker-definition types.
	PrintTimeout       time.Duration
	SystemPrompt       string
	UserMessage        string
	InputTokens        []any
	OutputSchema       string
	WorkingDirectory   string
	Worktree           string
	EnvVars            map[string]string
	ProcessEnvironment []string
	// SessionObserver receives a detached exact Provider Session reference as
	// soon as the native provider reports it, while the attempt is still live.
	// It is an invocation-scoped observation hook rather than a selection or
	// resume input: Execute never accepts a pre-existing SessionRef. Callers
	// must not assume that every provider can report a session before it
	// completes.
	SessionObserver SessionObserver
	// ProgressObserver receives each bounded progress fact as soon as the
	// provider reports it, while the attempt is still live. Like
	// SessionObserver it is an invocation-scoped observation hook, never a
	// selection or resume input. An adapter that cannot report progress before
	// it completes simply leaves the observer uncalled and returns its facts
	// in ExecuteDiagnostics.Progress as before.
	ProgressObserver ProgressObserver
}

// SessionObserver receives one detached Provider-owned session identity
// observed during a live execution attempt.
type SessionObserver func(SessionRef)

// ProgressObserver receives one bounded progress fact observed during a live
// execution attempt.
type ProgressObserver func(ExecuteProgress)

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
	return cloned
}

// ObserveSession forwards one complete detached Provider Session reference to
// the request's invocation-scoped observer. Native adapters call this only
// after they have received a provider-authored reference, never from model or
// runner configuration. Invalid observations are ignored rather than exposed
// as a plausible resumable identity.
func (request ExecuteRequest) ObserveSession(reference SessionRef) {
	if request.SessionObserver == nil || reference.Validate() != nil {
		return
	}
	request.SessionObserver(reference.Clone())
}

// ObserveProgress forwards one detached progress fact to the request's
// invocation-scoped observer. It is nil-safe so an adapter can report progress
// unconditionally without first checking whether a caller is listening.
func (request ExecuteRequest) ObserveProgress(progress ExecuteProgress) {
	if request.ProgressObserver == nil {
		return
	}
	request.ProgressObserver(progress.Clone())
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
	// ProgressAlreadyObserved reports that every fact in Progress was already
	// delivered through ExecuteRequest.ProgressObserver, in this exact order,
	// while the attempt was still live. A caller that published those live
	// observations MUST NOT republish Progress, or every fact appears twice.
	// Adapters that do not stream leave this false and Progress remains the
	// only delivery.
	ProgressAlreadyObserved bool
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

// ErrInvalidContinuationRequest reports that a continuation request is
// malformed: a blank provider, session kind, or session identity in Reference,
// or an otherwise invalid Attempt. It is returned before any provider adapter
// is invoked.
var ErrInvalidContinuationRequest = errors.New("provider continuation request is invalid")

// ErrContinuationForeign reports that a continuation request names an Attempt
// provider that does not match the Reference provider it continues. Providers
// never substitutes the resolved canonical provider for the one a foreign
// reference actually names.
var ErrContinuationForeign = errors.New("provider continuation reference is foreign")

// ErrContinuationStale reports that a continuation reference names a Provider
// Session identity its resolved provider no longer recognizes as live.
var ErrContinuationStale = errors.New("provider continuation reference is stale")

// ContinuationOutcome is the closed Providers-owned continuation success
// vocabulary. Unsupported is a successful capability result distinct from any
// ContinuationFailure: the resolved provider or session kind truthfully
// cannot continue, so no provider adapter is invoked.
type ContinuationOutcome string

const (
	ContinuationOutcomeResumed     ContinuationOutcome = "resumed"
	ContinuationOutcomeUnsupported ContinuationOutcome = "unsupported"
)

// ContinuationFailureKind is a provider-neutral continuation failure
// category. Unsupported is not a member of this vocabulary - see
// ContinuationOutcomeUnsupported.
type ContinuationFailureKind string

const (
	ContinuationFailureKindInvalid ContinuationFailureKind = "invalid"
	ContinuationFailureKindForeign ContinuationFailureKind = "foreign"
	ContinuationFailureKindStale   ContinuationFailureKind = "stale"
)

// ContinuationFailure retains the normalized continuation failure fact and the
// rejected detached reference, so peers can review exactly which Provider
// Session reference was rejected without importing Providers internals or
// re-deriving the reference from the original request.
type ContinuationFailure struct {
	Kind      ContinuationFailureKind
	Message   string
	Reference SessionRef
}

func (failure ContinuationFailure) Error() string {
	message := strings.TrimSpace(failure.Message)
	if message == "" {
		return sentinelForContinuationFailureKind(failure.Kind).Error()
	}
	return fmt.Sprintf("%s: %s", sentinelForContinuationFailureKind(failure.Kind).Error(), message)
}

func (failure ContinuationFailure) Unwrap() error {
	return sentinelForContinuationFailureKind(failure.Kind)
}

// Clone returns a detached continuation-failure copy.
func (failure ContinuationFailure) Clone() ContinuationFailure {
	failure.Reference = failure.Reference.Clone()
	return failure
}

func sentinelForContinuationFailureKind(kind ContinuationFailureKind) error {
	switch kind {
	case ContinuationFailureKindForeign:
		return ErrContinuationForeign
	case ContinuationFailureKindStale:
		return ErrContinuationStale
	default:
		return ErrInvalidContinuationRequest
	}
}

// ContinueRequest identifies the exact prior Provider Session to resume -
// provider identity, provider-specific session kind, and exact opaque session
// identity, carried by Reference - and the next attempt input to run against
// that continued session. Attempt.Provider must equal Reference.Provider;
// continuation is requested exclusively through this contract, never through
// the ordinary Execute vocabulary.
type ContinueRequest struct {
	Reference SessionRef
	Attempt   ExecuteRequest
}

// Validate checks that Reference is a complete session identity, that Attempt
// is itself valid, and that Attempt names the same provider as Reference - all
// before any provider adapter is invoked.
func (request ContinueRequest) Validate() error {
	if err := request.Reference.Validate(); err != nil {
		return ContinuationFailure{
			Kind:      ContinuationFailureKindInvalid,
			Message:   err.Error(),
			Reference: request.Reference,
		}
	}
	if err := request.Attempt.Validate(); err != nil {
		return ContinuationFailure{
			Kind:      ContinuationFailureKindInvalid,
			Message:   err.Error(),
			Reference: request.Reference,
		}
	}
	if request.Attempt.Provider != request.Reference.Provider {
		return ContinuationFailure{
			Kind: ContinuationFailureKindForeign,
			Message: fmt.Sprintf(
				"attempt provider %q does not match reference provider %q",
				request.Attempt.Provider, request.Reference.Provider,
			),
			Reference: request.Reference,
		}
	}
	return nil
}

// Clone returns a detached continuation-request copy.
func (request ContinueRequest) Clone() ContinueRequest {
	cloned := request
	cloned.Reference = request.Reference.Clone()
	cloned.Attempt = request.Attempt.Clone()
	return cloned
}

// ContinueResult is the detached result of one continuation intent: either
// the closed unsupported outcome (no provider adapter was invoked) or the
// resumed outcome carrying the continued attempt's ExecuteResult. Reference
// echoes the exact continued Provider Session identity unchanged.
type ContinueResult struct {
	Reference SessionRef
	Outcome   ContinuationOutcome
	Result    ExecuteResult
}

// Clone returns a detached continuation-result copy.
func (result ContinueResult) Clone() ContinueResult {
	cloned := result
	cloned.Reference = result.Reference.Clone()
	cloned.Result = result.Result.Clone()
	return cloned
}
