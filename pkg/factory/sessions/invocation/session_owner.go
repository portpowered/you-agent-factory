package invocation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workinvocation "github.com/portpowered/infinite-you/pkg/work/invocation"
)

// StructuredArgumentsInputSource identifies invocation input resolved from the
// structured API argument carrier rather than compatibility content.
const StructuredArgumentsInputSource workinvocation.InputSourceLabel = "signature_args"

// InvocationInputSourceKind identifies the transport-independent source
// category supplied for one Factory Session invocation.
type InvocationInputSourceKind string

const InvocationInputSourceKindText InvocationInputSourceKind = "text"

// InvocationRequest carries one normalized transport request into the Factory
// Session invocation owner. ContentProvided distinguishes an omitted content
// field from an explicitly supplied empty content value for validation.
type InvocationRequest struct {
	Args            *map[string]any
	Content         []work.WorkContentPart
	ContentProvided bool
	RequestID       *string
	SourceKind      *InvocationInputSourceKind
	TimeoutMillis   *int64
}

// FactoryInvocationResult is the shared domain outcome constructed by the
// Factory Session invocation owner.
type FactoryInvocationResult = interfaces.FactoryInvocationResult

// SessionInvoker is the canonical Factory Session invocation boundary.
type SessionInvoker interface {
	InvokeFactorySession(context.Context, string, InvocationRequest) (FactoryInvocationResult, error)
}

// SessionInvocationWaitInput carries the submitted invocation identity and
// policy through canonical event-derived result waiting.
type SessionInvocationWaitInput struct {
	RequestID        string
	TraceID          string
	InputSource      workinvocation.InputSourceLabel
	InvocationReturn *interfaces.InvocationReturnConfig
	FactoryConfig    *interfaces.FactoryConfig
	TimeoutMillis    *int64
}

// SessionInvocationTelemetry owns metric and safe-log emissions coordinated by
// SessionOwner.
type SessionInvocationTelemetry interface {
	NormalizationAttempt(*interfaces.FactoryConfig, workinvocation.InputSourceLabel)
	NormalizationFailure(*interfaces.FactoryConfig, workinvocation.InputSourceLabel, error)
	NormalizationSuccess(*interfaces.FactoryConfig, workinvocation.InputSourceLabel)
	InterpolationFailure(*interfaces.FactoryConfig, workinvocation.InputSourceLabel, error)
	SubmissionFailure(*interfaces.FactoryConfig, workinvocation.InputSourceLabel, error)
	InvocationSubmitted(*interfaces.FactoryConfig, workinvocation.InputSourceLabel)
	InvocationCompleted(*interfaces.FactoryConfig, workinvocation.InputSourceLabel, []work.WorkContentPart)
	InvocationFailed(*interfaces.FactoryConfig, workinvocation.InputSourceLabel, string)
	LogArgumentFailure(string, workinvocation.InputSourceLabel, *interfaces.FactoryConfig, *workinvocation.NormalizedArguments, error, string)
	LogSubmissionFailure(string, workinvocation.InputSourceLabel, *interfaces.FactoryConfig, error)
	LogInvocationSubmitted(string, workinvocation.InputSourceLabel, *interfaces.FactoryConfig, work.WorkRequestSubmitResult)
	LogInvocationCompleted(string, SessionInvocationWaitInput, workinvocation.PrimaryResultSelection)
	LogInvocationFailed(string, SessionInvocationWaitInput, FactoryInvocationResult, string)
}

// SessionOwnerDependencies are the explicit runtime boundaries required to
// resolve, submit, observe, and wait for one Factory Session invocation.
type SessionOwnerDependencies struct {
	FactoryConfig func(string) (*interfaces.FactoryConfig, error)
	SubmitWork    func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe       func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	WaitNext      func(context.Context) error
	// BeforeSubmit performs session-scoped setup after invocation validation and
	// before the first Work can enter the runtime.
	BeforeSubmit func(context.Context, string, *interfaces.FactoryConfig, *workinvocation.NormalizedArguments) error
	// AfterSubmit performs session-scoped activation once the invocation Work is
	// available to the runtime.
	AfterSubmit func(context.Context, string, *interfaces.FactoryConfig, *workinvocation.NormalizedArguments) error
	Telemetry   SessionInvocationTelemetry
	SpecialCase SessionInvocationSpecialCase
}

// SessionOwner coordinates the complete session invocation lifecycle through
// narrow, explicit collaborators.
type SessionOwner struct {
	deps SessionOwnerDependencies
}

// NewSessionOwner constructs the canonical Factory Session invocation owner.
func NewSessionOwner(deps SessionOwnerDependencies) *SessionOwner {
	return &SessionOwner{deps: deps}
}

// InvokeFactorySession resolves and validates one request, submits exactly one
// Work item, then delegates event-derived result waiting to the injected waiter.
func (o *SessionOwner) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request InvocationRequest,
) (FactoryInvocationResult, error) {
	if o == nil || o.deps.FactoryConfig == nil || o.deps.SubmitWork == nil || o.deps.Observe == nil {
		return FactoryInvocationResult{}, fmt.Errorf("factory session invocation owner dependencies are unavailable")
	}
	factoryCfg, resolved, err := o.prepareInvocation(ctx, sessionID, request)
	if err != nil {
		return FactoryInvocationResult{}, err
	}
	submitResult, err := o.submitInvocation(ctx, sessionID, request, factoryCfg, resolved)
	if err != nil {
		return FactoryInvocationResult{}, err
	}
	return o.waitForResult(ctx, sessionID, SessionInvocationWaitInput{
		RequestID:        submitResult.RequestID,
		TraceID:          submitResult.TraceID,
		InputSource:      resolved.Source,
		InvocationReturn: factoryCfg.InvocationReturn,
		FactoryConfig:    factoryCfg,
		TimeoutMillis:    request.TimeoutMillis,
	})
}

func (o *SessionOwner) prepareInvocation(ctx context.Context, sessionID string, request InvocationRequest) (*interfaces.FactoryConfig, ResolvedSessionInvocationInput, error) {
	factoryCfg, err := o.deps.FactoryConfig(sessionID)
	if err != nil {
		return nil, ResolvedSessionInvocationInput{}, err
	}
	if factoryCfg == nil {
		return nil, ResolvedSessionInvocationInput{}, fmt.Errorf("factory session runtime config is unavailable")
	}
	sourceHint := SessionInvocationSourceHint(request)
	o.normalizationAttempt(factoryCfg, sourceHint)
	resolved, err := ResolveSessionInvocationInput(factoryCfg, request)
	if err != nil {
		o.normalizationFailure(sessionID, factoryCfg, sourceHint, err)
		return nil, ResolvedSessionInvocationInput{}, err
	}
	o.normalizationSuccess(factoryCfg, resolved.Source)
	if err := workinvocation.ValidateInvocationInterpolation(factoryCfg, workinvocation.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments), os.ReadFile); err != nil {
		o.interpolationFailure(sessionID, factoryCfg, resolved, err)
		return nil, ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	if o.deps.BeforeSubmit != nil {
		if err := o.deps.BeforeSubmit(ctx, sessionID, factoryCfg, resolved.NormalizedArguments); err != nil {
			o.interpolationFailure(sessionID, factoryCfg, resolved, err)
			return nil, ResolvedSessionInvocationInput{}, err
		}
	}
	return factoryCfg, resolved, nil
}

func (o *SessionOwner) submitInvocation(ctx context.Context, sessionID string, request InvocationRequest, factoryCfg *interfaces.FactoryConfig, resolved ResolvedSessionInvocationInput) (work.WorkRequestSubmitResult, error) {
	workTypeName, err := factoryrun.DefaultHandlingWorkTypeName(factoryCfg)
	if err != nil {
		return work.WorkRequestSubmitResult{}, o.submissionError(sessionID, factoryCfg, resolved.Source, fmt.Errorf("resolve invocation work type: %w", err))
	}
	submitResult, err := o.deps.SubmitWork(ctx, sessionID, work.SubmitRequest{
		RequestID: trimmedStringValue(request.RequestID), WorkTypeID: workTypeName, Content: resolved.Content,
		InvocationArguments: workinvocation.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments),
	})
	if err != nil {
		return work.WorkRequestSubmitResult{}, o.submissionError(sessionID, factoryCfg, resolved.Source, err)
	}
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.InvocationSubmitted(factoryCfg, resolved.Source)
		o.deps.Telemetry.LogInvocationSubmitted(sessionID, resolved.Source, factoryCfg, submitResult)
	}
	if o.deps.AfterSubmit != nil {
		if err := o.deps.AfterSubmit(ctx, sessionID, factoryCfg, resolved.NormalizedArguments); err != nil {
			return work.WorkRequestSubmitResult{}, err
		}
	}
	return submitResult, nil
}

func (o *SessionOwner) submissionError(sessionID string, factoryCfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, err error) error {
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.SubmissionFailure(factoryCfg, source, err)
		o.deps.Telemetry.LogSubmissionFailure(sessionID, source, factoryCfg, err)
	}
	return err
}

// ResolvedSessionInvocationInput is the normalized input carried into Work submission.
type ResolvedSessionInvocationInput struct {
	Source              workinvocation.InputSourceLabel
	Content             []work.WorkContentPart
	NormalizedArguments *workinvocation.NormalizedArguments
}

// ResolveSessionInvocationInput applies the shared compatibility-content and
// structured-argument contract used by API and CLI invocation paths.
func ResolveSessionInvocationInput(cfg *interfaces.FactoryConfig, request InvocationRequest) (ResolvedSessionInvocationInput, error) {
	content, err := sessionInvocationCompatibilityContent(request)
	if err != nil {
		return ResolvedSessionInvocationInput{}, err
	}
	directArgs, err := sessionInvocationStructuredArgs(request)
	if err != nil {
		return ResolvedSessionInvocationInput{}, err
	}
	if request.Args == nil {
		return resolveCompatibilitySessionInvocationInput(content)
	}
	var signature *interfaces.InvocationSignatureConfig
	if cfg != nil {
		signature = cfg.InvocationSignature
	}
	return resolveStructuredSessionInvocationInput(signature, directArgs, content)
}

func resolveCompatibilitySessionInvocationInput(content []work.WorkContentPart) (ResolvedSessionInvocationInput, error) {
	if len(content) == 0 {
		return ResolvedSessionInvocationInput{}, &interfaces.RequestValidationError{Message: "content is required when args are omitted"}
	}
	normalized, err := workinvocation.NormalizeArguments(workinvocation.NormalizeArgumentsInput{CompatibilityContent: content})
	if err != nil {
		return ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	if normalized.CompatibilityInput == nil {
		return ResolvedSessionInvocationInput{}, &interfaces.RequestValidationError{Message: "content did not resolve to one logical invocation input"}
	}
	return ResolvedSessionInvocationInput{
		Source:              normalized.CompatibilityInput.Source,
		Content:             normalized.CompatibilityInput.Content,
		NormalizedArguments: &normalized,
	}, nil
}

func resolveStructuredSessionInvocationInput(
	signature *interfaces.InvocationSignatureConfig,
	directArgs []workinvocation.NamedArgumentInput,
	content []work.WorkContentPart,
) (ResolvedSessionInvocationInput, error) {
	if signature == nil {
		return ResolvedSessionInvocationInput{}, &workinvocation.ArgumentError{
			Code:     workinvocation.ArgumentErrorCodeInvalidActiveSignature,
			Message:  "named arguments require a factory invocationSignature",
			Argument: firstStructuredArgumentKey(directArgs),
		}
	}
	normalized, err := workinvocation.NormalizeArguments(workinvocation.NormalizeArgumentsInput{
		Signature:            signature,
		DirectArgs:           directArgs,
		CompatibilityContent: content,
	})
	if err != nil {
		return ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	source := StructuredArgumentsInputSource
	if normalized.CompatibilityInput != nil {
		source = normalized.CompatibilityInput.Source
	}
	return ResolvedSessionInvocationInput{Source: source, NormalizedArguments: &normalized}, nil
}

func sessionInvocationCompatibilityContent(request InvocationRequest) ([]work.WorkContentPart, error) {
	if !request.ContentProvided {
		if request.SourceKind != nil && *request.SourceKind != InvocationInputSourceKindText {
			return nil, &interfaces.RequestValidationError{Message: "sourceKind must be text"}
		}
		return nil, nil
	}
	if request.SourceKind == nil || *request.SourceKind != InvocationInputSourceKindText {
		return nil, &interfaces.RequestValidationError{Message: "sourceKind must be text"}
	}
	return work.CloneWorkContentParts(request.Content), nil
}

func sessionInvocationStructuredArgs(request InvocationRequest) ([]workinvocation.NamedArgumentInput, error) {
	if request.Args == nil {
		return nil, nil
	}
	directArgs, err := workinvocation.NamedArgumentInputsFromAnyMap(*request.Args)
	if err != nil {
		return nil, &interfaces.RequestValidationError{Message: err.Error()}
	}
	return directArgs, nil
}

func firstStructuredArgumentKey(arguments []workinvocation.NamedArgumentInput) string {
	if len(arguments) == 0 {
		return ""
	}
	return strings.TrimSpace(arguments[0].Key)
}

func trimmedStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizeSessionInvocationError(err error) error {
	var validationErr *workinvocation.TextContentValidationError
	if errors.As(err, &validationErr) {
		return &interfaces.RequestValidationError{Message: validationErr.Message}
	}
	return err
}

// SessionInvocationSourceHint reports a low-cardinality source before full normalization.
func SessionInvocationSourceHint(request InvocationRequest) workinvocation.InputSourceLabel {
	if request.Args != nil {
		return StructuredArgumentsInputSource
	}
	return workinvocation.InputSourceLabel(workinvocation.ArgumentSourceKindCompatibilityContent)
}

func (o *SessionOwner) normalizationAttempt(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) {
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.NormalizationAttempt(cfg, source)
	}
}

func (o *SessionOwner) normalizationFailure(sessionID string, cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel, err error) {
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.NormalizationFailure(cfg, source, err)
		o.deps.Telemetry.LogArgumentFailure(sessionID, source, cfg, nil, err, "normalization_failure")
	}
}

func (o *SessionOwner) normalizationSuccess(cfg *interfaces.FactoryConfig, source workinvocation.InputSourceLabel) {
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.NormalizationSuccess(cfg, source)
	}
}

func (o *SessionOwner) interpolationFailure(
	sessionID string,
	cfg *interfaces.FactoryConfig,
	resolved ResolvedSessionInvocationInput,
	err error,
) {
	if o.deps.Telemetry != nil {
		o.deps.Telemetry.InterpolationFailure(cfg, resolved.Source, err)
		o.deps.Telemetry.LogArgumentFailure(sessionID, resolved.Source, cfg, resolved.NormalizedArguments, err, "interpolation_failure")
	}
}
