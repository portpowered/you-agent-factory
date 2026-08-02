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

// SessionOwner coordinates the complete session invocation lifecycle through
// narrow, explicit collaborators.
type SessionOwner struct {
	factoryConfig     func(string) (*factorydefinitions.FactoryConfig, error)
	submitWork        func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	observe           func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	waitNextFn        func(context.Context) error
	telemetry         SessionInvocationTelemetry
	specialCase       SessionInvocationSpecialCase
	resolveDefinition DefinitionResolver
	inputFiles        fileeffects.InvocationInputReader
	workService       work.Service
}

// DefinitionResolver resolves one normalized invocation against a detached
// effective Factory source. The caller supplies copied FILE_CONTENTS bytes;
// the Definitions root performs interpolation and returns immutable policy
// facts without receiving a filesystem reader or Sessions runtime handle.
type DefinitionResolver func(
	context.Context,
	string,
	*factorydefinitions.FactoryConfig,
	*work.InvocationArguments,
	map[string][]byte,
) (factorydefinitions.ResolveInvocationDefinitionResult, error)

// NewSessionOwner constructs the canonical Factory Session invocation owner.
func NewSessionOwner(
	factoryConfig func(string) (*factorydefinitions.FactoryConfig, error),
	submitWork func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error),
	observe func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error),
	waitNext func(context.Context) error,
	telemetry SessionInvocationTelemetry,
	specialCase SessionInvocationSpecialCase,
	resolveDefinition DefinitionResolver,
	inputFiles fileeffects.InvocationInputReader,
	workService work.Service,
) *SessionOwner {
	return &SessionOwner{
		factoryConfig: factoryConfig, submitWork: submitWork, observe: observe,
		waitNextFn: waitNext, telemetry: telemetry, specialCase: specialCase,
		resolveDefinition: resolveDefinition, inputFiles: inputFiles,
		workService: workService,
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
	if o.inputFiles == nil {
		return FactoryInvocationResult{}, fmt.Errorf("Factory Session invocation input file reader is unavailable")
	}
	if o.resolveDefinition == nil {
		return FactoryInvocationResult{}, fmt.Errorf("Factory Definitions invocation resolver is unavailable")
	}
	runtimeArgs := work.RuntimeInvocationArguments(factoryCfg.InvocationSignature, resolved.NormalizedArguments)
	fileInputs, err := o.resolveInvocationFileInputs(runtimeArgs)
	if err != nil {
		o.interpolationFailure(sessionID, factoryCfg, resolved, err)
		return FactoryInvocationResult{}, qualifySessionInvocationError(factoryCfg, err)
	}
	definition, err := o.resolveDefinition(ctx, sessionID, factoryCfg, runtimeArgs, fileInputs)
	if err != nil {
		if isDefaultWorkTypeResolutionError(err) {
			err = fmt.Errorf("resolve invocation work type: %w", err)
			if o.telemetry != nil {
				o.telemetry.SubmissionFailure(factoryCfg, resolved.Source, err)
				o.telemetry.LogSubmissionFailure(sessionID, resolved.Source, factoryCfg, err)
			}
			return FactoryInvocationResult{}, err
		}
		o.interpolationFailure(sessionID, factoryCfg, resolved, err)
		return FactoryInvocationResult{}, qualifySessionInvocationError(factoryCfg, err)
	}
	resolvedFactory := definition.Factory
	workTypeName := strings.TrimSpace(definition.DefaultWorkType)
	if workTypeName == "" {
		err = fmt.Errorf("resolve invocation work type: Factory Definitions returned an empty default Work type")
		if o.telemetry != nil {
			o.telemetry.SubmissionFailure(factoryCfg, resolved.Source, err)
			o.telemetry.LogSubmissionFailure(sessionID, resolved.Source, factoryCfg, err)
		}
		return FactoryInvocationResult{}, err
	}
	resolvedArgs := work.RuntimeInvocationArguments(resolvedFactory.InvocationSignature, resolved.NormalizedArguments)
	resolvedFactoryForWait := &resolvedFactory
	submitResult, err := o.submitWork(ctx, sessionID, work.SubmitRequest{
		RequestID:           trimmedStringValue(request.RequestID),
		WorkTypeID:          workTypeName,
		Content:             resolved.Content,
		InvocationArguments: resolvedArgs,
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
		InvocationReturn: resolvedFactoryForWait.InvocationReturn,
		FactoryConfig:    resolvedFactoryForWait,
		TimeoutMillis:    request.TimeoutMillis,
	})
}

func isDefaultWorkTypeResolutionError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "resolve default Work type")
}

func (o *SessionOwner) resolveInvocationFileInputs(args *work.InvocationArguments) (map[string][]byte, error) {
	if args == nil || len(args.Arguments) == 0 {
		return nil, nil
	}
	var inputs map[string][]byte
	for parameterName, argument := range args.Arguments {
		if argument.ValueMode != work.InvocationParameterValueModeFileContents {
			continue
		}
		if len(argument.Values) != 1 {
			return nil, &work.ArgumentError{
				Code:      work.ArgumentErrorCodeInvalidInterpolation,
				Message:   fmt.Sprintf("invocation parameter %q requires exactly one FILE_CONTENTS path", parameterName),
				Parameter: parameterName,
			}
		}
		path := argument.Values[0]
		data, err := o.inputFiles(path)
		if err != nil {
			return nil, &work.ArgumentError{
				Code:      work.ArgumentErrorCodeInvalidInterpolation,
				Message:   fmt.Sprintf("invocation parameter %q could not read FILE_CONTENTS path %q: %v", parameterName, path, err),
				Parameter: parameterName,
			}
		}
		if inputs == nil {
			inputs = make(map[string][]byte)
		}
		inputs[path] = append([]byte(nil), data...)
	}
	return inputs, nil
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
	if request.Args == nil {
		return o.resolveCompatibilitySessionInvocationInput(ctx, content)
	}
	var signature *factorydefinitions.InvocationSignatureConfig
	if cfg != nil {
		signature = cfg.InvocationSignature
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
	return resolvedSessionInvocationInputFromPrepared(nil, prepared)
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
