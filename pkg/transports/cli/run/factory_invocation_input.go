package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
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

// MapInvocationInputError represents Work-owned preparation failures using the
// stable CLI error vocabulary.
func MapInvocationInputError(err error) error {
	inputErr, ok := err.(*work.InputError)
	if ok {
		switch inputErr.Code {
		case work.InputErrorCodeSourceConflict:
			return ambiguousInvocationInputError(inputErr)
		default:
			return &InvocationError{Code: string(inputErr.Code), Message: inputErr.Message}
		}
	}
	argumentErr, ok := err.(*work.ArgumentError)
	if ok {
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

	streamRenderer := invocationFactoryEventRenderer(cfg, presentation)
	if streamRenderer != nil {
		defer streamRenderer.stopProgressRendering()
	}

	var consume factorysessions.FactoryEventConsumer
	if streamRenderer != nil {
		consume = streamRenderer.PresentFactoryEvents
	}
	outcome, err := invocation.InvokeFactory(
		ctx, target, factorysessionmapping.InvocationRequestFromAPI(request), consume,
	)
	if err != nil {
		return MapInvocationFailure(err)
	}
	result := outcome.Result
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, streamRenderer)
	}
	return writeInvocationSuccess(cfg, result, streamRenderer)
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
	streamRenderer factoryEventRenderer,
) error {
	if streamRenderer != nil {
		if err := streamRenderer.writeFinalInvocationResult(result); err != nil {
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
	streamRenderer factoryEventRenderer,
) error {
	if streamRenderer != nil {
		return streamRenderer.writeFinalInvocationResult(result)
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

func factoryEventForPublicPresentation(event interfaces.FactoryEvent) (interfaces.FactoryEvent, bool) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.UseNumber()
	if len(event.Payload) == 0 || decoder.Decode(&payload) != nil {
		event.Payload = json.RawMessage(`{}`)
		return event, true
	}
	payload = redactPrivateFactoryEventPayload(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return interfaces.FactoryEvent{}, false
	}
	event.Payload = encoded
	return event, true
}

func redactPrivateFactoryEventPayload(value any) any {
	if list, ok := value.([]any); ok {
		for index, child := range list {
			list[index] = redactPrivateFactoryEventPayload(child)
		}
		return list
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if object["schemaVersion"] == string(factorysessions.ResponseEventSchemaVersionV1) {
		return map[string]any{}
	}
	for _, key := range []string{"diagnostics", "response", "providerSession", "provider_session", "providerSessionRef", "textDelta", "toolCallId", "toolCalls"} {
		delete(object, key)
	}
	for key, child := range object {
		object[key] = redactPrivateFactoryEventPayload(child)
	}
	return object
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
