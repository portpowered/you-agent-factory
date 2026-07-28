package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
)

type InvocationInputSource = runconfig.InvocationInputSource

const (
	InvocationInputSourcePositional = runconfig.InvocationInputSourcePositional
	InvocationInputSourceStdin      = runconfig.InvocationInputSourceStdin
	InvocationInputSourceWorkFile   = runconfig.InvocationInputSourceWorkFile
)

// InvocationSignatureFromEffectiveSchema maps the transport-owned effective
// schema into Work's normalization contract. The effective schema remains the
// authority; the selected Factory's authored signature is not consulted again.
func InvocationSignatureFromEffectiveSchema(
	schema climanifest.EffectiveInputSchema,
) *work.InvocationSignatureConfig {
	if schema.FactoryInputMode != climanifest.EffectiveFactoryInputModeSignature {
		return nil
	}
	signature := &work.InvocationSignatureConfig{
		UnknownNamedArgumentPolicy: schema.UnknownNamedArgumentPolicy,
		Parameters:                 make([]work.InvocationParameterConfig, 0, len(schema.FactoryParameters)),
	}
	for _, parameter := range schema.FactoryParameters {
		mapped := work.InvocationParameterConfig{
			Name:          parameter.CanonicalName,
			ExternalName:  parameter.PreferredExternalName,
			Aliases:       append([]string(nil), parameter.Aliases...),
			Description:   parameter.Description,
			Required:      parameter.Required,
			Choices:       append([]string(nil), parameter.Choices...),
			DefaultValues: append([]string(nil), parameter.DefaultValues...),
			ValueMode:     parameter.ValueMode,
			TypeHint:      parameter.TypeHint,
			Sensitive:     parameter.Sensitive,
			Bindings:      append([]work.InvocationParameterBindingConfig(nil), parameter.Bindings...),
		}
		if parameter.DefaultValue != nil {
			mapped.DefaultValue = *parameter.DefaultValue
		}
		signature.Parameters = append(signature.Parameters, mapped)
	}
	return signature
}

// MapCompositionDiagnostics returns the first deterministic composition
// failure in the stable CLI error vocabulary.
func MapCompositionDiagnostics(diagnostics []climanifest.CompositionDiagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	return &InvocationError{
		Code:    diagnostics[0].Code,
		Message: diagnostics[0].Message,
	}
}

// MapInvocationInputError represents Work-owned preparation failures using the
// stable CLI error vocabulary.
func MapInvocationInputError(err error) error {
	var inputErr *work.InputError
	if errors.As(err, &inputErr) {
		switch inputErr.Code {
		case work.InputErrorCodeSourceConflict:
			return ambiguousInvocationInputError(inputErr)
		default:
			return &InvocationError{Code: string(inputErr.Code), Message: inputErr.Message}
		}
	}
	var argumentErr *work.ArgumentError
	if errors.As(err, &argumentErr) {
		return &InvocationError{Code: string(argumentErr.Code), Message: argumentErr.Message}
	}
	return err
}

// InvocationInputSourceFromWork maps a detached Work source label onto the CLI
// vocabulary used by diagnostics and RunConfig.
func InvocationInputSourceFromWork(source work.InputSourceLabel) InvocationInputSource {
	switch source {
	case work.InputSourcePositionalText:
		return InvocationInputSourcePositional
	case work.InputSourceStdinText:
		return InvocationInputSourceStdin
	default:
		return ""
	}
}

func ambiguousInvocationInputError(inputErr *work.InputError) error {
	sources := make([]InvocationInputSource, 0, len(inputErr.ConflictingSources))
	for _, label := range inputErr.ConflictingSources {
		switch label {
		case work.InputSourcePositionalText:
			sources = append(sources, InvocationInputSourcePositional)
		case work.InputSourceStdinText:
			sources = append(sources, InvocationInputSourceStdin)
		}
	}
	return &AmbiguousInvocationInputError{
		invocationErr: &InvocationError{
			Code:    string(inputErr.Code),
			Message: inputErr.Message,
		},
		Sources: sources,
	}
}

type AmbiguousInvocationInputError struct {
	invocationErr *InvocationError
	Sources       []InvocationInputSource
}

func (e *AmbiguousInvocationInputError) Error() string {
	if e == nil || e.invocationErr == nil {
		return ""
	}
	return e.invocationErr.Error()
}

func (e *AmbiguousInvocationInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.invocationErr
}

func openInvocation(
	ctx context.Context,
	cfg RunConfig,
	logger *zap.Logger,
	request *factoryapi.InvocationRequest,
	recordPath resolvedRunRecordPath,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	mockWorkersConfig *workers.MockWorkersConfig,
) (*Operation, error) {
	if invocation == nil {
		return nil, fmt.Errorf("construct factory invocation: operation is required")
	}
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) && presentation == nil {
		return nil, fmt.Errorf("construct factory invocation: response presentation operation is required")
	}
	return &Operation{
		cfg: cfg, logger: logger, invocationRequest: request,
		invocationTarget: invocationTarget(cfg, logger, mockWorkersConfig),
		invocation:       invocation, presentation: presentation,
		invocationMode: true, recordPath: recordPath,
	}, nil
}

func resolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	if strings.TrimSpace(cfg.WorkFile) != "" {
		return nil, false, nil
	}
	if factoryInvocationRoot(cfg) == "" {
		return nil, false, nil
	}
	if cfg.PreparedInvocationInput != nil {
		return invocationRequestFromPreparedInput(*cfg.PreparedInvocationInput)
	}
	if cfg.InvocationNormalizedArguments != nil {
		return invocationRequestFromNormalizedArguments(*cfg.InvocationNormalizedArguments), true, nil
	}
	if cfg.InvocationPositionalText == nil && cfg.InvocationStdinText == nil {
		return nil, false, nil
	}
	if cfg.InvocationPositionalText != nil && cfg.InvocationStdinText != nil {
		return nil, true, fmt.Errorf("prepared invocation input contains both positional and stdin text")
	}
	if cfg.InvocationPositionalText != nil {
		recordCLIInvocationResolved(cfg, work.InputSourcePositionalText)
		return invocationRequestFromText(*cfg.InvocationPositionalText), true, nil
	}
	recordCLIInvocationResolved(cfg, work.InputSourceStdinText)
	return invocationRequestFromText(*cfg.InvocationStdinText), true, nil
}

func factoryInvocationRoot(cfg RunConfig) string {
	if root := strings.TrimSpace(cfg.FactoryConfigPath); root != "" {
		return root
	}
	return strings.TrimSpace(cfg.Dir)
}

func invocationRequestFromResolvedInput(resolved work.ResolvedInput) *factoryapi.InvocationRequest {
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := *contentcontract.GeneratedPtrFromParts(resolved.Content)
	return &factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
}

func invocationRequestFromPreparedInput(prepared work.PreparedInvocationInput) (*factoryapi.InvocationRequest, bool, error) {
	if prepared.NormalizedArguments != nil {
		return invocationRequestFromNormalizedArguments(*prepared.NormalizedArguments), true, nil
	}
	if prepared.ResolvedInput != nil {
		return invocationRequestFromResolvedInput(*prepared.ResolvedInput), true, nil
	}
	return nil, false, nil
}

func invocationRequestFromText(text string) *factoryapi.InvocationRequest {
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := *contentcontract.GeneratedPtrFromParts([]work.WorkContentPart{{
		Type: work.WorkContentPartTypeText, Text: text,
	}})
	return &factoryapi.InvocationRequest{SourceKind: &sourceKind, Content: &content}
}

func invocationRequestFromNormalizedArguments(normalized work.NormalizedArguments) *factoryapi.InvocationRequest {
	args := make(map[string]any, len(normalized.Arguments))
	for name, argument := range normalized.Arguments {
		if len(argument.Values) == 1 {
			args[name] = argument.Values[0]
			continue
		}
		values := append([]string(nil), argument.Values...)
		args[name] = values
	}
	return &factoryapi.InvocationRequest{Args: &args}
}

func wrapInvocationInputError(err error) error {
	inputErr, ok := err.(*work.InputError)
	if !ok {
		return err
	}
	return invocationCLIError{
		Code:    string(inputErr.Code),
		Message: inputErr.Message,
	}
}

func runFactoryInvocation(
	ctx context.Context,
	cfg RunConfig,
	target factorysessions.InvocationTarget,
	request factoryapi.InvocationRequest,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
) error {
	if invocation == nil {
		return fmt.Errorf("run factory invocation: operation is required")
	}

	logPackagedTTSInvocationStart(cfg)

	invocationCfg := cfg
	invokeCtx := ctx
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) && cfg.Output != nil {
		var cancel context.CancelFunc
		invokeCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		invocationCfg.Output = responseStreamOutputCancelOnWriteError(cfg.Output, cancel)
	}

	streamRenderer, err := invocationFactoryEventRenderer(invocationCfg, presentation)
	if err != nil {
		return err
	}
	if streamRenderer != nil {
		defer streamRenderer.StopProgressRendering()
	}

	var consume factorysessions.FactoryEventConsumer
	if streamRenderer != nil {
		consume = streamRenderer.PresentFactoryEvents
	}
	invocationRequest := factorysessionmapping.InvocationRequestFromAPI(request)
	if cfg.PreparedInvocationInput != nil {
		invocationRequest.Args = nil
		invocationRequest.Content = nil
		invocationRequest.ContentProvided = false
		invocationRequest.PreparedInvocationInput = cfg.PreparedInvocationInput.Clone()
	}
	outcome, err := invocation.InvokeFactory(invokeCtx, target, invocationRequest, consume)
	if err != nil {
		return MapInvocationFailure(err)
	}
	result := outcome.Result
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(invocationCfg, result, streamRenderer)
	}
	return writeInvocationSuccess(invocationCfg, result, streamRenderer)
}

type responseStreamCancelOnWriteError struct {
	writer  io.Writer
	onError func()
	once    sync.Once
}

func responseStreamOutputCancelOnWriteError(writer io.Writer, onError context.CancelFunc) io.Writer {
	if writer == nil || onError == nil {
		return writer
	}
	return &responseStreamCancelOnWriteError{
		writer: writer,
		onError: func() {
			onError()
		},
	}
}

func (writer *responseStreamCancelOnWriteError) Write(payload []byte) (int, error) {
	written, err := writer.writer.Write(payload)
	if err != nil {
		writer.once.Do(writer.onError)
	}
	return written, err
}

func invocationTarget(
	cfg RunConfig,
	logger *zap.Logger,
	mockWorkersConfig *workers.MockWorkersConfig,
) factorysessions.InvocationTarget {
	return factorysessions.InvocationTarget{
		FactoryDir:        cfg.Dir,
		FactorySourcePath: cfg.FactoryConfigPath,
		RunnerID:          cfg.RunnerID,
		OperatorDefaults:  cfg.OperatorDefaults,
		ExecutionBaseDir:  cfg.ExecutionBaseDir,
		HomeDir:           cfg.HomeDir,
		Logger:            logger,
		Verbose:           cfg.Verbose,
		RecordPath:        cfg.RecordPath,
		ReplayPath:        cfg.ReplayPath,
		RuntimeLogDir:     cfg.RuntimeLogDir,
		RuntimeLogConfig: factoryruntime.RuntimeLogStorageConfig{
			MaxSize: cfg.RuntimeLogConfig.MaxSize, MaxBackups: cfg.RuntimeLogConfig.MaxBackups,
			MaxAge: cfg.RuntimeLogConfig.MaxAge, Compress: cfg.RuntimeLogConfig.Compress,
		},
		RuntimeMetricsDir: cfg.RuntimeMetricsDir,
		RuntimeMetricsConfig: factoryruntime.RuntimeMetricsStorageConfig{
			MaxSize: cfg.RuntimeMetricsConfig.MaxSize, MaxBackups: cfg.RuntimeMetricsConfig.MaxBackups,
			MaxAge: cfg.RuntimeMetricsConfig.MaxAge, Compress: cfg.RuntimeMetricsConfig.Compress,
		},
		ModelCacheDir:           cfg.ModelCacheDir,
		WorkflowID:              cfg.Workflow,
		MockWorkersConfig:       mockWorkersConfig,
		SkipPermissionsOverride: cfg.InvocationSkipPermissionsOverride,
		MetricsRecorder:         cfg.InvocationMetricsRecorder,
	}
}

type invocationCLIError struct {
	Code      string
	Message   string
	SessionID string
	WorkID    string
	WorkName  string
	WorkState string
}

func (e invocationCLIError) InvocationErrorCode() string {
	return e.Code
}

func (e invocationCLIError) InvocationErrorMessage() string {
	return e.responseMessage()
}

func (e invocationCLIError) Error() string {
	contextSuffix := e.contextSuffix()
	switch {
	case strings.TrimSpace(e.Code) == "":
		return strings.TrimSpace(e.Message) + contextSuffix
	case strings.TrimSpace(e.Message) == "":
		return strings.TrimSpace(e.Code) + contextSuffix
	default:
		return strings.TrimSpace(e.Code) + ": " + strings.TrimSpace(e.Message) + contextSuffix
	}
}

func (e invocationCLIError) contextSuffix() string {
	fields := make([]string, 0, 4)
	if sessionID := strings.TrimSpace(e.SessionID); sessionID != "" {
		fields = append(fields, "session="+sessionID)
	}
	if workID := strings.TrimSpace(e.WorkID); workID != "" {
		fields = append(fields, "workId="+workID)
	}
	if workName := strings.TrimSpace(e.WorkName); workName != "" {
		fields = append(fields, "workName="+workName)
	}
	if workState := strings.TrimSpace(e.WorkState); workState != "" {
		fields = append(fields, "workState="+workState)
	}
	if len(fields) == 0 {
		return ""
	}
	return " [" + strings.Join(fields, " ") + "]"
}

func (e invocationCLIError) responseMessage() string {
	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "clean invocation failed"
	}
	return message + e.contextSuffix()
}

func invocationResultFailure(result apisurface.FactoryInvocationResult) error {
	return invocationCLIError{
		Code:      strings.TrimSpace(result.ErrorCode),
		Message:   strings.TrimSpace(result.Message),
		SessionID: strings.TrimSpace(result.SessionID),
		WorkID:    strings.TrimSpace(result.WorkID),
		WorkName:  strings.TrimSpace(result.WorkName),
		WorkState: strings.TrimSpace(result.WorkState),
	}
}

func writeInvocationFailure(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer visualizationcli.FactoryEventRenderer,
) error {
	if streamRenderer != nil {
		if err := streamRenderer.WriteFinalInvocationResult(result); err != nil {
			return err
		}
	} else if cfg.JSONOutput {
		if err := writeInvocationJSON(cfg, result); err != nil {
			return err
		}
	}
	return invocationResultFailure(result)
}

func writeInvocationSuccess(
	cfg RunConfig,
	result apisurface.FactoryInvocationResult,
	streamRenderer visualizationcli.FactoryEventRenderer,
) error {
	if streamRenderer != nil {
		return streamRenderer.WriteFinalInvocationResult(result)
	}
	if cfg.JSONOutput {
		return writeInvocationJSON(cfg, result)
	}

	text, err := invocationPrimaryResultText(result.PrimaryResult)
	if err != nil {
		return err
	}
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("write invocation result: process output is required")
	}
	_, err = fmt.Fprint(output, text)
	return err
}

func writeInvocationJSON(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	output := cfg.Output
	if output == nil {
		return fmt.Errorf("write invocation JSON: process output is required")
	}
	encoded, err := json.Marshal(apisurface.InvocationResponseFromResult(result))
	if err != nil {
		return fmt.Errorf("marshal invocation response: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func invocationPrimaryResultText(parts []work.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("invocation primary result is empty")
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			return "", fmt.Errorf("invocation primary result is not plain text; use --json")
		}
		textParts = append(textParts, part.Text)
	}
	return strings.Join(textParts, "\n"), nil
}
