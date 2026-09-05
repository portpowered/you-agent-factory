package invocation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
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
type SessionInvoker = roles.SessionInvoker

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

// SessionInvocationWaiter blocks until the next result observation attempt is
// warranted. ReleaseSessionInvocationWaiter frees any resources the waiter
// holds once its invocation wait loop ends.
type (
	SessionInvocationWaiter        = func(context.Context) error
	ReleaseSessionInvocationWaiter = func()
)

// SessionOwner coordinates the complete session invocation lifecycle through
// narrow, explicit collaborators.
type SessionOwner struct {
	factoryConfig func(string) (*factorydefinitions.FactoryConfig, error)
	submitWork    func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	observe       func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	waitNextFn    func(context.Context) error
	waitSessionFn func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter)
	telemetry     SessionInvocationTelemetry
	specialCase   SessionInvocationSpecialCase
	interpolation factorydefinitions.InvocationInterpolationService
	workTypes     factorydefinitions.InvocationWorkTypeService
	inputFiles    fileeffects.InvocationInputReader
	workService   work.Service
}

// NewSessionOwner constructs the canonical Factory Session invocation owner.
// waitSession, when present, opens one event-driven waiter per invocation wait
// loop; waitNext remains the per-iteration fallback wait.
func NewSessionOwner(
	factoryConfig func(string) (*factorydefinitions.FactoryConfig, error),
	submitWork func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error),
	observe func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error),
	waitNext func(context.Context) error,
	waitSession func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter),
	telemetry SessionInvocationTelemetry,
	specialCase SessionInvocationSpecialCase,
	interpolation factorydefinitions.InvocationInterpolationService,
	workTypes factorydefinitions.InvocationWorkTypeService,
	inputFiles fileeffects.InvocationInputReader,
	workService work.Service,
) *SessionOwner {
	return &SessionOwner{
		factoryConfig: factoryConfig, submitWork: submitWork, observe: observe,
		waitNextFn: waitNext, waitSessionFn: waitSession,
		telemetry: telemetry, specialCase: specialCase,
		interpolation: interpolation, workTypes: workTypes, inputFiles: inputFiles,
		workService: workService,
	}
}

// Invoke resolves and validates one request, submits exactly one Work item,
// then delegates event-derived result waiting to the injected waiter.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (o *SessionOwner) Invoke(
	ctx context.Context,
	sessionID string,
	request InvocationRequest,
) (FactoryInvocationResult, error) {
	if o == nil || o.factoryConfig == nil || o.submitWork == nil || o.observe == nil || o.workService == nil {
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
	resolved, err := o.resolveSessionInvocationInput(ctx, factoryCfg, request)
	if err != nil {
		o.normalizationFailure(sessionID, factoryCfg, sourceHint, err)
		return FactoryInvocationResult{}, qualifySessionInvocationError(factoryCfg, err)
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
		return FactoryInvocationResult{}, qualifySessionInvocationError(factoryCfg, err)
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
	submissionContextErr := contextError(ctx)
	submitResult, err := o.submitWork(ctx, sessionID, work.SubmitRequest{
		RequestID:           trimmedStringValue(request.RequestID),
		WorkTypeID:          workTypeName,
		Content:             resolved.Content,
		InvocationArguments: work.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments),
	})
	if submissionContextErr == nil {
		if contextErr := contextError(ctx); contextErr != nil {
			return o.waitErrorResult(sessionID, SessionInvocationWaitInput{
				InputSource: resolved.Source, FactoryConfig: factoryCfg,
			}, contextErr)
		}
	}
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

// InvokeFactorySession preserves the compatibility-shaped private capability
// while keeping the canonical owner operation as the one-way implementation.
func (o *SessionOwner) InvokeFactorySession(
	ctx context.Context,
	sessionID string,
	request InvocationRequest,
) (FactoryInvocationResult, error) {
	return o.Invoke(ctx, sessionID, request)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
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
	if o == nil || o.workService == nil {
		return factorysessions.ResolvedInvocationInput{}, fmt.Errorf("factory session invocation owner dependencies are unavailable")
	}
	resolved, err := o.resolveSessionInvocationInput(context.Background(), cfg, request)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	return factorysessions.ResolvedInvocationInput{
		Source: resolved.Source, Content: resolved.Content,
		NormalizedArguments: resolved.NormalizedArguments,
	}, nil
}

// resolveSessionInvocationInput applies the shared compatibility-content and
// structured-argument contract used by API and CLI invocation paths.
func (o *SessionOwner) resolveSessionInvocationInput(
	ctx context.Context,
	cfg *factorydefinitions.FactoryConfig,
	request InvocationRequest,
) (ResolvedSessionInvocationInput, error) {
	if request.PreparedInvocationInput != nil {
		return resolvedPreparedSessionInvocationInput(cfg, request)
	}
	content, err := sessionInvocationCompatibilityContent(request)
	if err != nil {
		return ResolvedSessionInvocationInput{}, err
	}
	directArgs, err := sessionInvocationStructuredArgs(request)
	if err != nil {
		return ResolvedSessionInvocationInput{}, err
	}
	var signature *factorydefinitions.InvocationSignatureConfig
	if cfg != nil {
		signature = cfg.InvocationSignature
	}
	if request.Args == nil {
		return o.resolveCompatibilitySessionInvocationInput(ctx, signature, content)
	}
	return o.resolveStructuredSessionInvocationInput(ctx, signature, directArgs, content)
}

func resolvedPreparedSessionInvocationInput(
	cfg *factorydefinitions.FactoryConfig,
	request InvocationRequest,
) (ResolvedSessionInvocationInput, error) {
	if request.Args != nil || request.ContentProvided {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{
			Message: "prepared invocation input cannot be combined with args or content",
		}
	}
	prepared := request.PreparedInvocationInput.Clone()
	if prepared == nil ||
		(prepared.NormalizedArguments == nil) == (prepared.ResolvedInput == nil) {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{
			Message: "prepared invocation input must contain exactly one canonical result",
		}
	}
	var signature *factorydefinitions.InvocationSignatureConfig
	if cfg != nil {
		signature = cfg.InvocationSignature
	}
	if prepared.NormalizedArguments != nil && signature == nil {
		return ResolvedSessionInvocationInput{}, &work.ArgumentError{
			Code:    work.ArgumentErrorCodeInvalidActiveSignature,
			Message: "prepared arguments require a factory invocationSignature",
		}
	}
	if prepared.ResolvedInput != nil {
		if signature != nil {
			return ResolvedSessionInvocationInput{}, &work.ArgumentError{
				Code:    work.ArgumentErrorCodeInvalidActiveSignature,
				Message: "prepared compatibility input requires a factory without an invocationSignature",
			}
		}
		resolved := *prepared.ResolvedInput
		return ResolvedSessionInvocationInput{
			Source:  resolved.Source,
			Content: resolved.Content,
			NormalizedArguments: &work.NormalizedArguments{
				CompatibilityInput: &resolved,
			},
		}, nil
	}
	return ResolvedSessionInvocationInput{
		Source:              StructuredArgumentsInputSource,
		Content:             structuredInvocationContent(signature, *prepared.NormalizedArguments),
		NormalizedArguments: prepared.NormalizedArguments,
	}, nil
}

func (o *SessionOwner) resolveCompatibilitySessionInvocationInput(
	ctx context.Context,
	signature *factorydefinitions.InvocationSignatureConfig,
	content []work.WorkContentPart,
) (ResolvedSessionInvocationInput, error) {
	if len(content) == 0 {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{Message: "content is required when args are omitted"}
	}
	prepared, err := o.workService.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		CompatibilityContent: content,
	})
	if err != nil {
		return ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	if signature != nil {
		parameterName := primaryTextParameter(signature)
		if parameterName == "" {
			return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{
				Message: "text content requires a primary positional or stdin invocation parameter",
			}
		}
		if prepared.ResolvedInput == nil || strings.TrimSpace(prepared.ResolvedInput.Text) == "" {
			return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{
				Message: "content did not resolve to one logical invocation input",
			}
		}
		return o.resolveStructuredSessionInvocationInput(ctx, signature, []work.NamedArgumentInput{{
			Key: parameterName, Values: []string{prepared.ResolvedInput.Text},
		}}, nil)
	}
	return resolvedSessionInvocationInputFromPrepared(nil, prepared)
}

// primaryTextParameter finds the signature parameter that receives ordinary
// unstructured transport text. Positional slot one has precedence because it
// is the Factory's declared primary Work content; a stdin binding is the
// fallback for signatures that intentionally expose text only through stdin.
func primaryTextParameter(signature *factorydefinitions.InvocationSignatureConfig) string {
	if signature == nil {
		return ""
	}
	stdinParameter := ""
	for _, parameter := range signature.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		for _, binding := range parameter.Bindings {
			switch binding.Kind {
			case factorydefinitions.InvocationParameterBindingKindPositional:
				if binding.Position == 1 {
					return name
				}
			case factorydefinitions.InvocationParameterBindingKindStdin:
				if stdinParameter == "" {
					stdinParameter = name
				}
			}
		}
	}
	return stdinParameter
}

func (o *SessionOwner) resolveStructuredSessionInvocationInput(
	ctx context.Context,
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
	prepared, err := o.workService.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Signature:            signature,
		DirectArgs:           directArgs,
		CompatibilityContent: content,
	})
	if err != nil {
		return ResolvedSessionInvocationInput{}, normalizeSessionInvocationError(err)
	}
	return resolvedSessionInvocationInputFromPrepared(signature, prepared)
}

func resolvedSessionInvocationInputFromPrepared(
	signature *factorydefinitions.InvocationSignatureConfig,
	prepared work.PreparedInvocationInput,
) (ResolvedSessionInvocationInput, error) {
	if prepared.ResolvedInput != nil {
		resolved := *prepared.ResolvedInput
		return ResolvedSessionInvocationInput{
			Source:              resolved.Source,
			Content:             resolved.Content,
			NormalizedArguments: &work.NormalizedArguments{CompatibilityInput: &resolved},
		}, nil
	}
	if prepared.NormalizedArguments == nil {
		return ResolvedSessionInvocationInput{}, &factorydefinitions.RequestValidationError{
			Message: "content did not resolve to one logical invocation input",
		}
	}
	normalized := *prepared.NormalizedArguments
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
	var inputErr *work.InputError
	if errors.As(err, &inputErr) {
		return inputErr
	}
	var argumentErr *work.ArgumentError
	if errors.As(err, &argumentErr) {
		return argumentErr
	}
	var validationErr *work.TextContentValidationError
	if errors.As(err, &validationErr) {
		return &factorydefinitions.RequestValidationError{Message: validationErr.Message}
	}
	return err
}

func qualifySessionInvocationError(
	factoryCfg *factorydefinitions.FactoryConfig,
	err error,
) error {
	err = normalizeSessionInvocationError(err)
	if factoryCfg == nil {
		return err
	}
	return work.QualifyInvocationArgumentError(err, factorydefinitions.CustomerVisibleFactoryName(factoryCfg))
}

// SessionInvocationSourceHint reports a low-cardinality source before full normalization.
func SessionInvocationSourceHint(request InvocationRequest) work.InputSourceLabel {
	if request.PreparedInvocationInput != nil {
		if request.PreparedInvocationInput.ResolvedInput != nil {
			return request.PreparedInvocationInput.ResolvedInput.Source
		}
		return StructuredArgumentsInputSource
	}
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
