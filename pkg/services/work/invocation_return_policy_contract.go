package work

import (
	"context"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/invocationreturnpolicy"
)

// Invocation/return-policy typed failures peers can distinguish on the root
// Service slice (PrepareInvocationInput / ResolvePrimaryResult).
var (
	ErrInvalidInvocationInput    = errors.New("invalid invocation input")
	ErrUnsupportedReturnPolicy   = errors.New("unsupported invocation return policy")
)

const (
	ReturnPolicySubmittedWorkTerminal = "SUBMITTED_WORK_TERMINAL"
	ReturnPolicyExplicit              = "EXPLICIT"
)

type ArgumentSourceKind string

const (
	ArgumentSourceKindPositional           ArgumentSourceKind = "POSITIONAL"
	ArgumentSourceKindNamed                ArgumentSourceKind = "NAMED"
	ArgumentSourceKindStructured           ArgumentSourceKind = "STRUCTURED"
	ArgumentSourceKindStdin                ArgumentSourceKind = "STDIN"
	ArgumentSourceKindDefault              ArgumentSourceKind = "DEFAULT"
	ArgumentSourceKindCompatibilityText    ArgumentSourceKind = "COMPATIBILITY_TEXT"
	ArgumentSourceKindCompatibilityContent ArgumentSourceKind = "COMPATIBILITY_CONTENT"
)

type ArgumentErrorCode string

const (
	ArgumentErrorCodeInvalidActiveSignature   ArgumentErrorCode = "INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE"
	ArgumentErrorCodeMissingRequiredInput     ArgumentErrorCode = "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT"
	ArgumentErrorCodeUnknownArgument          ArgumentErrorCode = "INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT"
	ArgumentErrorCodeSourceConflict           ArgumentErrorCode = "INVOCATION_ARGUMENT_SOURCE_CONFLICT"
	ArgumentErrorCodeStringValidationMismatch ArgumentErrorCode = "INVOCATION_ARGUMENT_STRING_VALIDATION_MISMATCH"
	ArgumentErrorCodePositionalOverflow       ArgumentErrorCode = "INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW"
	ArgumentErrorCodeUnroutableStdin          ArgumentErrorCode = "INVOCATION_ARGUMENT_UNROUTABLE_STDIN"
	ArgumentErrorCodeInvalidInterpolation     ArgumentErrorCode = "INVOCATION_ARGUMENT_INVALID_INTERPOLATION"
)

type NamedArgumentInput struct {
	Key    string
	Values []string
}

type ArgumentSource struct {
	Kind   ArgumentSourceKind
	Name   string
	Redact bool
}

type NormalizedArgument struct {
	Values    []string
	Sensitive bool
	Sources   []ArgumentSource
}

type NormalizedArguments struct {
	Arguments          map[string]NormalizedArgument
	UnknownNamedArgs   map[string][]string
	CompatibilityInput *ResolvedInput
}

type NormalizeArgumentsInput struct {
	Signature            *InvocationSignatureConfig
	PositionalArgs       []string
	NamedArgs            []NamedArgumentInput
	DirectArgs           []NamedArgumentInput
	StdinText            *string
	CompatibilityText    *string
	CompatibilityContent []WorkContentPart
}

type ArgumentError struct {
	Code       ArgumentErrorCode
	Message    string
	Parameter  string
	Argument   string
	SourceKind ArgumentSourceKind
}

func (e *ArgumentError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type InvocationInputPreparationRequest struct {
	Arguments            []string
	Signature            *InvocationSignatureConfig
	StdinText            *string
	DirectArgs           []NamedArgumentInput
	CompatibilityContent []WorkContentPart
}

type PreparedInvocationInput struct {
	Source              InputSourceLabel
	ResolvedInput       *ResolvedInput
	NormalizedArguments *NormalizedArguments
}

func (input *PreparedInvocationInput) Clone() *PreparedInvocationInput {
	if input == nil {
		return nil
	}
	cloned := &PreparedInvocationInput{Source: input.Source}
	if input.ResolvedInput != nil {
		resolved := *input.ResolvedInput
		resolved.Content = CloneWorkContentParts(resolved.Content)
		cloned.ResolvedInput = &resolved
	}
	if input.NormalizedArguments != nil {
		outer := normalizedArgumentsFromInternal(normalizedArgumentsToInternal(*input.NormalizedArguments))
		cloned.NormalizedArguments = &outer
	}
	return cloned
}

type InvocationInputPreparation interface {
	PrepareInvocationInput(context.Context, InvocationInputPreparationRequest) (PreparedInvocationInput, error)
}

type PrimaryResultErrorCode string

const (
	PrimaryResultErrorCodeUnresolved  PrimaryResultErrorCode = "INVOCATION_PRIMARY_RESULT_UNRESOLVED"
	PrimaryResultErrorCodeFailed      PrimaryResultErrorCode = "INVOCATION_RUNTIME_FAILURE"
	PrimaryResultErrorCodeBlocked     PrimaryResultErrorCode = "INVOCATION_BLOCKED"
	PrimaryResultErrorCodeNeedsHuman  PrimaryResultErrorCode = "INVOCATION_NEEDS_HUMAN"
	PrimaryResultErrorCodePaused      PrimaryResultErrorCode = "INVOCATION_PAUSED"
	PrimaryResultErrorCodeInterrupted PrimaryResultErrorCode = "INVOCATION_INTERRUPTED"
)

type PrimaryResultSelectionInput struct {
	RequestID        string
	InvocationReturn *InvocationReturnConfig
	WorldState       InvocationWorldStateProvider
}

type PrimaryResultSelection struct {
	RequestID     string
	Policy        string
	WorkID        string
	WorkTypeName  string
	WorkName      string
	TerminalState string
	PrimaryResult []WorkContentPart
}

type InvocationFailureContext struct {
	SessionID string
	WorkID    string
	WorkName  string
	WorkState string
}

type PrimaryResultError struct {
	Code      PrimaryResultErrorCode
	Message   string
	RequestID string
	Policy    string
	Context   InvocationFailureContext
}

func (e *PrimaryResultError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type InvocationExampleNormalizer struct{}

func (InvocationExampleNormalizer) NormalizeLegacyInvocationExample(
	arguments []string,
	signature *InvocationSignatureConfig,
	stdinText *string,
) (*NormalizedArguments, error) {
	normalized, err := invocationreturnpolicy.InvocationExampleNormalizer{}.NormalizeLegacyInvocationExample(
		arguments,
		invocationSignatureToInternal(signature),
		stdinText,
	)
	if err != nil {
		return nil, mapInvocationReturnPolicyError(err)
	}
	if normalized == nil {
		return nil, nil
	}
	outer := normalizedArgumentsFromInternal(*normalized)
	return &outer, nil
}

func RuntimeInvocationArguments(
	signature *InvocationSignatureConfig,
	normalized *NormalizedArguments,
) *InvocationArguments {
	if normalized == nil {
		return nil
	}
	inner := normalizedArgumentsToInternal(*normalized)
	return runtimeInvocationArgumentsFromInternal(signature, &inner)
}

func InvocationSignatureHash(signature *InvocationSignatureConfig) string {
	return invocationreturnpolicy.InvocationSignatureHash(invocationSignatureToInternal(signature))
}

func NormalizeInvocationValueMode(valueMode string) string {
	return invocationreturnpolicy.NormalizeInvocationValueMode(valueMode)
}

func NamedArgumentInputsFromAnyMap(values map[string]any) ([]NamedArgumentInput, error) {
	inputs, err := invocationreturnpolicy.NamedArgumentInputsFromAnyMap(values)
	if err != nil {
		return nil, err
	}
	return namedArgumentInputsFromInternal(inputs), nil
}

// QualifyInvocationArgumentError adds the invoked Factory name to
// customer-visible ArgumentError diagnostics when the transport boundary knows it.
func QualifyInvocationArgumentError(err error, factoryName string) error {
	var argumentErr *ArgumentError
	if errors.As(err, &argumentErr) {
		return mapInvocationReturnPolicyError(
			invocationreturnpolicy.QualifyInvocationArgumentError(
				argumentErrorToInternal(argumentErr),
				factoryName,
			),
		)
	}
	return mapInvocationReturnPolicyError(
		invocationreturnpolicy.QualifyInvocationArgumentError(err, factoryName),
	)
}

func NormalizeArguments(input NormalizeArgumentsInput) (NormalizedArguments, error) {
	result, err := invocationreturnpolicy.NormalizeArguments(normalizeArgumentsInputToInternal(input))
	if err != nil {
		return NormalizedArguments{}, mapInvocationReturnPolicyError(err)
	}
	return normalizedArgumentsFromInternal(result), nil
}

func ResolvePrimaryResult(input PrimaryResultSelectionInput) (PrimaryResultSelection, error) {
	selection, err := invocationreturnpolicy.ResolvePrimaryResult(primaryResultSelectionInputToInternal(input))
	if err != nil {
		return PrimaryResultSelection{}, mapInvocationReturnPolicyError(err)
	}
	return primaryResultSelectionFromInternal(selection), nil
}

func ClassifyInvocationControlState(
	sessionID string,
	snapshotFactoryState string,
	input PrimaryResultSelectionInput,
) (*PrimaryResultError, bool) {
	result, ok := invocationreturnpolicy.ClassifyInvocationControlState(
		sessionID,
		snapshotFactoryState,
		primaryResultSelectionInputToInternal(input),
	)
	if !ok {
		return nil, false
	}
	return primaryResultErrorFromInternal(result), true
}

func ClassifyMissingPrimaryResult(input PrimaryResultSelectionInput) (*PrimaryResultError, bool) {
	result, ok := invocationreturnpolicy.ClassifyMissingPrimaryResult(primaryResultSelectionInputToInternal(input))
	if !ok {
		return nil, false
	}
	return primaryResultErrorFromInternal(result), true
}

func ClassifyMissingPrimaryResultWorkItem(
	requestID string,
	invocationReturn *InvocationReturnConfig,
	item FactoryWorkItem,
	sessionID string,
) *PrimaryResultError {
	return primaryResultErrorFromInternal(invocationreturnpolicy.ClassifyMissingPrimaryResultWorkItem(
		requestID,
		invocationReturnToInternal(invocationReturn),
		workItemToInternal(item),
		sessionID,
	))
}

func ClassifyFailedInvocation(
	sessionID string,
	input PrimaryResultSelectionInput,
) (*PrimaryResultError, bool) {
	result, ok := invocationreturnpolicy.ClassifyFailedInvocation(
		sessionID,
		primaryResultSelectionInputToInternal(input),
	)
	if !ok {
		return nil, false
	}
	return primaryResultErrorFromInternal(result), true
}

func NewInvocationInputPreparation() InvocationInputPreparation {
	return invocationInputPreparationAdapter{}
}

type invocationInputPreparationAdapter struct{}

func (invocationInputPreparationAdapter) PrepareInvocationInput(
	ctx context.Context,
	request InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	prepared, err := invocationreturnpolicy.NewInvocationInputPreparation().PrepareInvocationInput(
		ctx,
		invocationInputPreparationRequestToInternal(request),
	)
	if err != nil {
		return PreparedInvocationInput{}, mapInvocationReturnPolicyError(err)
	}
	return preparedInvocationInputFromInternal(prepared), nil
}

func NewInvocationPolicyService() Service {
	return invocationPolicyServiceAdapter{inner: invocationreturnpolicy.NewPolicyService()}
}

type invocationPolicyServiceAdapter struct {
	inner *invocationreturnpolicy.PolicyService
}

func (invocationPolicyServiceAdapter) SubmitWorkRequestForSession(
	context.Context,
	string,
	WorkRequest,
) (WorkRequestSubmitResult, error) {
	return WorkRequestSubmitResult{}, fmt.Errorf("Work invocation policy service does not support admission")
}

func (invocationPolicyServiceAdapter) PrepareWorkRequest(
	context.Context,
	WorkRequestPreparation,
) (WorkRequest, error) {
	return WorkRequest{}, fmt.Errorf("Work invocation policy service does not support admission prep")
}

func (invocationPolicyServiceAdapter) MoveWorkForSession(
	context.Context,
	string,
	string,
	string,
	string,
) (OperatorMoveResult, error) {
	return OperatorMoveResult{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyServiceAdapter) ListWork(context.Context, string, ListOptions) (ListResult, error) {
	return ListResult{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyServiceAdapter) GetWork(context.Context, string, string) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyServiceAdapter) MoveWorkAndRead(
	context.Context,
	string,
	string,
	string,
	string,
) (ReadModel, error) {
	return ReadModel{}, fmt.Errorf("Work invocation policy service does not support state access")
}

func (invocationPolicyServiceAdapter) StageContent(context.Context, StageContentRequest) (StageContentResult, error) {
	return StageContentResult{}, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyServiceAdapter) PrepareContent(context.Context, []StagedSubmissionItem) ([]WorkContentPart, error) {
	return nil, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyServiceAdapter) ResolveContent(context.Context, string) (ResolvedStagedContent, error) {
	return ResolvedStagedContent{}, fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyServiceAdapter) CleanupContent(context.Context, string) error {
	return fmt.Errorf("Work invocation policy service does not support content staging")
}

func (invocationPolicyServiceAdapter) MaterializeContentURL(context.Context, string) (string, ContentCleanup, error) {
	return "", nil, fmt.Errorf("Work invocation policy service does not support content materialization")
}

func (a invocationPolicyServiceAdapter) PrepareInvocationInput(
	ctx context.Context,
	request InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	prepared, err := a.inner.PrepareInvocationInput(ctx, invocationInputPreparationRequestToInternal(request))
	if err != nil {
		return PreparedInvocationInput{}, mapInvocationReturnPolicyError(err)
	}
	return preparedInvocationInputFromInternal(prepared), nil
}

func (a invocationPolicyServiceAdapter) ResolvePrimaryResult(
	ctx context.Context,
	input PrimaryResultSelectionInput,
) (PrimaryResultSelection, error) {
	selection, err := a.inner.ResolvePrimaryResult(ctx, primaryResultSelectionInputToInternal(input))
	if err != nil {
		return PrimaryResultSelection{}, mapInvocationReturnPolicyError(err)
	}
	return primaryResultSelectionFromInternal(selection), nil
}

func normalizedArgumentsToInternal(input NormalizedArguments) invocationreturnpolicy.NormalizedArguments {
	inner := invocationreturnpolicy.NormalizedArguments{
		Arguments:        make(map[string]invocationreturnpolicy.NormalizedArgument, len(input.Arguments)),
		UnknownNamedArgs: make(map[string][]string, len(input.UnknownNamedArgs)),
	}
	for name, argument := range input.Arguments {
		inner.Arguments[name] = invocationreturnpolicy.NormalizedArgument{
			Values:    cloneStringSlice(argument.Values),
			Sensitive: argument.Sensitive,
			Sources:   argumentSourcesToInternal(argument.Sources),
		}
	}
	for name, values := range input.UnknownNamedArgs {
		inner.UnknownNamedArgs[name] = cloneStringSlice(values)
	}
	if input.CompatibilityInput != nil {
		resolved := resolvedInputToInternal(*input.CompatibilityInput)
		inner.CompatibilityInput = &resolved
	}
	return inner
}

func argumentSourcesToInternal(sources []ArgumentSource) []invocationreturnpolicy.ArgumentSource {
	if len(sources) == 0 {
		return nil
	}
	converted := make([]invocationreturnpolicy.ArgumentSource, len(sources))
	for i, source := range sources {
		converted[i] = invocationreturnpolicy.ArgumentSource{
			Kind:   invocationreturnpolicy.ArgumentSourceKind(source.Kind),
			Name:   source.Name,
			Redact: source.Redact,
		}
	}
	return converted
}

func resolvedInputToInternal(input ResolvedInput) invocationreturnpolicy.ResolvedInput {
	return invocationreturnpolicy.ResolvedInput{
		Source:  invocationreturnpolicy.InputSourceLabel(input.Source),
		Text:    input.Text,
		Content: contentPartsToInternal(input.Content),
	}
}

func namedArgumentInputsFromInternal(inputs []invocationreturnpolicy.NamedArgumentInput) []NamedArgumentInput {
	if len(inputs) == 0 {
		return nil
	}
	converted := make([]NamedArgumentInput, len(inputs))
	for i, input := range inputs {
		converted[i] = NamedArgumentInput{
			Key:    input.Key,
			Values: cloneStringSlice(input.Values),
		}
	}
	return converted
}
