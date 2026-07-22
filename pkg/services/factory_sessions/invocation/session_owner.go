package invocation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
)

// StructuredArgumentsInputSource identifies invocation input resolved from the
// structured API argument carrier rather than compatibility content.
const StructuredArgumentsInputSource work.InputSourceLabel = "signature_args"

type InvocationInputSourceKind = factorysessions.InvocationInputSourceKind

const InvocationInputSourceKindText = factorysessions.InvocationInputSourceKindText

type InvocationRequest = factorysessions.InvocationRequest

// FactoryInvocationResult is the shared domain outcome constructed by the
// Factory Session invocation owner.
type FactoryInvocationResult = factorydefinitions.FactoryInvocationResult

// SessionInvoker is the canonical Factory Session invocation boundary.
type SessionInvoker = factorysessions.SessionInvoker

// SessionInvocationWaitInput carries the submitted invocation identity and
// policy through canonical event-derived result waiting.
type SessionInvocationWaitInput struct {
	RequestID        string
	TraceID          string
	InputSource      work.InputSourceLabel
	InvocationReturn *factorydefinitions.InvocationReturnConfig
	FactoryConfig    *factorydefinitions.FactoryConfig
	TimeoutMillis    *int64
}

// SessionInvocationTelemetry owns metric and safe-log emissions coordinated by
// SessionOwner.
type SessionInvocationTelemetry interface {
	NormalizationAttempt(*factorydefinitions.FactoryConfig, work.InputSourceLabel)
	NormalizationFailure(*factorydefinitions.FactoryConfig, work.InputSourceLabel, error)
	NormalizationSuccess(*factorydefinitions.FactoryConfig, work.InputSourceLabel)
	InterpolationFailure(*factorydefinitions.FactoryConfig, work.InputSourceLabel, error)
	SubmissionFailure(*factorydefinitions.FactoryConfig, work.InputSourceLabel, error)
	InvocationSubmitted(*factorydefinitions.FactoryConfig, work.InputSourceLabel)
	InvocationCompleted(*factorydefinitions.FactoryConfig, work.InputSourceLabel, []work.WorkContentPart)
	InvocationFailed(*factorydefinitions.FactoryConfig, work.InputSourceLabel, string)
	LogArgumentFailure(string, work.InputSourceLabel, *factorydefinitions.FactoryConfig, *work.NormalizedArguments, error, string)
	LogSubmissionFailure(string, work.InputSourceLabel, *factorydefinitions.FactoryConfig, error)
	LogInvocationSubmitted(string, work.InputSourceLabel, *factorydefinitions.FactoryConfig, work.WorkRequestSubmitResult)
	LogInvocationCompleted(string, SessionInvocationWaitInput, work.PrimaryResultSelection)
	LogInvocationFailed(string, SessionInvocationWaitInput, FactoryInvocationResult, string)
}

// SessionOwner coordinates the complete session invocation lifecycle through
// narrow, explicit collaborators.
type SessionOwner struct {
	factoryConfig func(string) (*factorydefinitions.FactoryConfig, error)
	submitWork    func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	observe       func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	waitNextFn    func(context.Context) error
	telemetry     SessionInvocationTelemetry
	specialCase   SessionInvocationSpecialCase
	interpolation factorydefinitions.InvocationInterpolationService
	workTypes     factorydefinitions.InvocationWorkTypeService
	inputFiles    fileeffects.InvocationInputReader
}

// NewSessionOwner constructs the canonical Factory Session invocation owner.
func NewSessionOwner(
	factoryConfig func(string) (*factorydefinitions.FactoryConfig, error),
	submitWork func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error),
	observe func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error),
	waitNext func(context.Context) error,
	telemetry SessionInvocationTelemetry,
	specialCase SessionInvocationSpecialCase,
	interpolation factorydefinitions.InvocationInterpolationService,
	workTypes factorydefinitions.InvocationWorkTypeService,
	inputFiles fileeffects.InvocationInputReader,
) *SessionOwner {
	return &SessionOwner{
		factoryConfig: factoryConfig, submitWork: submitWork, observe: observe,
		waitNextFn: waitNext, telemetry: telemetry, specialCase: specialCase,
		interpolation: interpolation, workTypes: workTypes, inputFiles: inputFiles,
	}
}

// InvokeFactorySession resolves and validates one request, submits exactly one
// Work item, then delegates event-derived result waiting to the injected waiter.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (o *SessionOwner) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request InvocationRequest,
) (FactoryInvocationResult, error) {
	if o == nil || o.factoryConfig == nil || o.submitWork == nil || o.observe == nil {
		return FactoryInvocationResult{}, fmt.Errorf("factory session invocation owner dependencies are unavailable")
	}
	factoryCfg, err := o.factoryConfig(sessionID)
	if err != nil {
		return FactoryInvocationResult{}, err
	}
	if factoryCfg == nil {
		return FactoryInvocationResult{}, fmt.Errorf("factory session runtime config is unavailable")
	}

	sourceHint := SessionInvocationSourceHint(request)
	o.normalizationAttempt(factoryCfg, sourceHint)
	resolved, err := ResolveSessionInvocationInput(factoryCfg, request)
	if err != nil {
		o.normalizationFailure(sessionID, factoryCfg, sourceHint, err)
		return FactoryInvocationResult{}, err
	}
	o.normalizationSuccess(factoryCfg, resolved.Source)
	if o.interpolation == nil {
		return FactoryInvocationResult{}, fmt.Errorf("Factory Definition invocation interpolation service is unavailable")
	}
	if o.inputFiles == nil {
		return FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation input file reader is unavailable")
	}
	if err := o.interpolation.ValidateInvocationInterpolation(factoryCfg, work.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments), factorydefinitions.FileReader(o.inputFiles)); err != nil {
		o.interpolationFailure(sessionID, factoryCfg, resolved, err)
		return FactoryInvocationResult{}, normalizeSessionInvocationError(err)
	}

	if o.workTypes == nil {
		return FactoryInvocationResult{}, fmt.Errorf("Factory Definition invocation Work Type service is unavailable")
	}
	workTypeName, err := o.workTypes.DefaultWorkType(factoryCfg)
	if err != nil {
		err = fmt.Errorf("resolve invocation work type: %w", err)
		if o.telemetry != nil {
			o.telemetry.SubmissionFailure(factoryCfg, resolved.Source, err)
			o.telemetry.LogSubmissionFailure(sessionID, resolved.Source, factoryCfg, err)
		}
		return FactoryInvocationResult{}, err
	}
	submitResult, err := o.submitWork(ctx, sessionID, work.SubmitRequest{
		RequestID:           trimmedStringValue(request.RequestID),
		WorkTypeID:          workTypeName,
		Content:             resolved.Content,
		InvocationArguments: work.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments),
	})
	if err != nil {
		if o.telemetry != nil {
			o.telemetry.SubmissionFailure(factoryCfg, resolved.Source, err)
			o.telemetry.LogSubmissionFailure(sessionID, resolved.Source, factoryCfg, err)
		}
		return FactoryInvocationResult{}, err
	}
	if o.telemetry != nil {
		o.telemetry.InvocationSubmitted(factoryCfg, resolved.Source)
		o.telemetry.LogInvocationSubmitted(sessionID, resolved.Source, factoryCfg, submitResult)
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

// ResolvedSessionInvocationInput is the normalized input carried into Work submission.
type ResolvedSessionInvocationInput struct {
	Source              work.InputSourceLabel
	Content             []work.WorkContentPart
	NormalizedArguments *work.NormalizedArguments
}

// ResolveInvocationInput exposes input normalization as an exact Factory
// Sessions service role for orchestrator-specific invocation operations.
func (o *SessionOwner) ResolveInvocationInput(
	cfg *factorydefinitions.FactoryConfig,
	request factorysessions.InvocationRequest,
) (factorysessions.ResolvedInvocationInput, error) {
	resolved, err := ResolveSessionInvocationInput(cfg, request)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	return factorysessions.ResolvedInvocationInput{
		Source: resolved.Source, Content: resolved.Content,
		NormalizedArguments: resolved.NormalizedArguments,
	}, nil
}

// ResolveSessionInvocationInput applies the shared compatibility-content and
// structured-argument contract used by API and CLI invocation paths.
func ResolveSessionInvocationInput(cfg *factorydefinitions.FactoryConfig, request InvocationRequest) (ResolvedSessionInvocationInput, error) {
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
	var signature *factorydefinitions.InvocationSignatureConfig
	if cfg != nil {
		signature = cfg.InvocationSignature
	}
	return resolveStructuredSessionInvocationInput(signature, directArgs, content)
}

func resolveCompatibilitySessionInvocationInput(content []work.WorkContentPart) (ResolvedSessionInvocationInput, error) {
	if len(content) == 0 {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{Message: "content is required when args are omitted"}
	}
	normalized, err := work.NormalizeArguments(work.NormalizeArgumentsInput{CompatibilityContent: content})
	if err != nil {
		return ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	if normalized.CompatibilityInput == nil {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{Message: "content did not resolve to one logical invocation input"}
	}
	return ResolvedSessionInvocationInput{
		Source:              normalized.CompatibilityInput.Source,
		Content:             normalized.CompatibilityInput.Content,
		NormalizedArguments: &normalized,
	}, nil
}

func resolveStructuredSessionInvocationInput(
	signature *factorydefinitions.InvocationSignatureConfig,
	directArgs []work.NamedArgumentInput,
	content []work.WorkContentPart,
) (ResolvedSessionInvocationInput, error) {
	if signature == nil {
		return ResolvedSessionInvocationInput{}, &work.ArgumentError{
			Code:     work.ArgumentErrorCodeInvalidActiveSignature,
			Message:  "named arguments require a factory invocationSignature",
			Argument: firstStructuredArgumentKey(directArgs),
		}
	}
	normalized, err := work.NormalizeArguments(work.NormalizeArgumentsInput{
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
	return ResolvedSessionInvocationInput{
		Source:              source,
		Content:             structuredInvocationContent(signature, normalized),
		NormalizedArguments: &normalized,
	}, nil
}

// structuredInvocationContent preserves the primary positional argument as the
// submitted Work content so routed Work can expose the original request.
func structuredInvocationContent(signature *factorydefinitions.InvocationSignatureConfig, normalized work.NormalizedArguments) []work.WorkContentPart {
	if signature == nil {
		return nil
	}
	for _, parameter := range signature.Parameters {
		if !hasPrimaryPositionalBinding(parameter.Bindings) {
			continue
		}
		argument, ok := normalized.Arguments[parameter.Name]
		if !ok || len(argument.Values) != 1 {
			return nil
		}
		return []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: argument.Values[0]}}
	}
	return nil
}

func hasPrimaryPositionalBinding(bindings []factorydefinitions.InvocationParameterBindingConfig) bool {
	for _, binding := range bindings {
		if binding.Kind == factorydefinitions.InvocationParameterBindingKindPositional && binding.Position == 1 {
			return true
		}
	}
	return false
}

func sessionInvocationCompatibilityContent(request InvocationRequest) ([]work.WorkContentPart, error) {
	if !request.ContentProvided {
		if request.SourceKind != nil && *request.SourceKind != InvocationInputSourceKindText {
			return nil, &factorydefinitions.RequestValidationError{Message: "sourceKind must be text"}
		}
		return nil, nil
	}
	if request.SourceKind == nil || *request.SourceKind != InvocationInputSourceKindText {
		return nil, &factorydefinitions.RequestValidationError{Message: "sourceKind must be text"}
	}
	return work.CloneWorkContentParts(request.Content), nil
}

func sessionInvocationStructuredArgs(request InvocationRequest) ([]work.NamedArgumentInput, error) {
	if request.Args == nil {
		return nil, nil
	}
	directArgs, err := work.NamedArgumentInputsFromAnyMap(*request.Args)
	if err != nil {
		return nil, &factorydefinitions.RequestValidationError{Message: err.Error()}
	}
	return directArgs, nil
}

func firstStructuredArgumentKey(arguments []work.NamedArgumentInput) string {
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
	var validationErr *work.TextContentValidationError
	if errors.As(err, &validationErr) {
		return &factorydefinitions.RequestValidationError{Message: validationErr.Message}
	}
	return err
}

// SessionInvocationSourceHint reports a low-cardinality source before full normalization.
func SessionInvocationSourceHint(request InvocationRequest) work.InputSourceLabel {
	if request.Args != nil {
		return StructuredArgumentsInputSource
	}
	return work.InputSourceLabel(work.ArgumentSourceKindCompatibilityContent)
}

func (o *SessionOwner) normalizationAttempt(cfg *factorydefinitions.FactoryConfig, source work.InputSourceLabel) {
	if o.telemetry != nil {
		o.telemetry.NormalizationAttempt(cfg, source)
	}
}

func (o *SessionOwner) normalizationFailure(sessionID string, cfg *factorydefinitions.FactoryConfig, source work.InputSourceLabel, err error) {
	if o.telemetry != nil {
		o.telemetry.NormalizationFailure(cfg, source, err)
		o.telemetry.LogArgumentFailure(sessionID, source, cfg, nil, err, "normalization_failure")
	}
}

func (o *SessionOwner) normalizationSuccess(cfg *factorydefinitions.FactoryConfig, source work.InputSourceLabel) {
	if o.telemetry != nil {
		o.telemetry.NormalizationSuccess(cfg, source)
	}
}

func (o *SessionOwner) interpolationFailure(
	sessionID string,
	cfg *factorydefinitions.FactoryConfig,
	resolved ResolvedSessionInvocationInput,
	err error,
) {
	if o.telemetry != nil {
		o.telemetry.InterpolationFailure(cfg, resolved.Source, err)
		o.telemetry.LogArgumentFailure(sessionID, resolved.Source, cfg, resolved.NormalizedArguments, err, "interpolation_failure")
	}
}
