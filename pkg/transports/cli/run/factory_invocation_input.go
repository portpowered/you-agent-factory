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
	"github.com/portpowered/infinite-you/pkg/services/models"
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
	InvocationInputSourceFile       = runconfig.InvocationInputSourceFile
	InvocationInputSourceNamed      = runconfig.InvocationInputSourceNamed
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
	case work.InputSourceFileText:
		return InvocationInputSourceFile
	case work.InputSourceNamedText:
		return InvocationInputSourceNamed
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
		case work.InputSourceFileText:
			sources = append(sources, InvocationInputSourceFile)
		case work.InputSourceNamedText:
			sources = append(sources, InvocationInputSourceNamed)
		default:
			sources = append(sources, InvocationInputSource(label))
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
		invocationTarget: invocationTarget(cfg, mockWorkersConfig),
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

// hostedInvocationOperation keeps the already-opened application runtime at
// the CLI composition edge. InvocationTarget remains detached configuration;
// the Factory Sessions invocation operation only receives that value contract
// and opens ephemeral runtimes for its normal path.
type hostedInvocationOperation struct {
	delegate InvocationOperation
	hosted   *factorysessions.HostedLiveInvocation
	logger   *zap.Logger
}

func (operation *hostedInvocationOperation) InvokeModel(
	ctx context.Context,
	target factorysessions.InvocationTarget,
	modelName string,
	request models.Request,
) (models.Result, error) {
	return operation.delegate.InvokeModel(ctx, target, modelName, request)
}

func (operation *hostedInvocationOperation) ResolveModelInvocationFactoryDir(dir string) (string, error) {
	return operation.delegate.ResolveModelInvocationFactoryDir(dir)
}

func (operation *hostedInvocationOperation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	return operation.delegate.ExportModelInvocationArtifact(sourcePath, destinationPath)
}

func (operation *hostedInvocationOperation) InvokeFactory(
	ctx context.Context,
	target factorysessions.InvocationTarget,
	request factorysessions.InvocationRequest,
	consume factorysessions.FactoryEventConsumer,
) (factorysessions.FactoryInvocationOutcome, error) {
	if operation == nil || operation.delegate == nil {
		return factorysessions.FactoryInvocationOutcome{}, errors.New("hosted invocation operation is required")
	}
	hosted := operation.hosted
	if hosted == nil || hosted.Sessions == nil || hosted.Invoker == nil {
		return factorysessions.FactoryInvocationOutcome{}, errors.New("hosted live invocation runtime is incomplete")
	}
	projection, projectionErr := hosted.Sessions.GetFactorySession(ctx, factorysessions.DefaultSessionID)
	if projectionErr == nil && interfaces.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		return operation.delegate.InvokeFactory(ctx, target, request, consume)
	}
	liveEvents, err := startHostedInvocationFactoryEvents(ctx, hosted.Sessions, consume)
	if err != nil {
		return factorysessions.FactoryInvocationOutcome{}, err
	}
	invocationResult, invokeErr := hosted.Invoker.InvokeFactorySession(
		ctx, factorysessions.DefaultSessionID, request,
	)
	outcome := factorysessions.FactoryInvocationOutcome{
		Result: factoryInvocationResultFromSessionInvocation(invocationResult),
	}
	if liveEvents == nil {
		return outcome, invokeErr
	}
	postResultErr := liveEvents.finish(ctx, hosted.Sessions, outcome.Result)
	if postResultErr != nil && outcome.Result.Status != "" {
		if operation.logger != nil {
			operation.logger.Warn(
				"invocation post-result step failed after terminal result was determined",
				zap.Error(postResultErr),
			)
		}
		return outcome, invokeErr
	}
	return outcome, errors.Join(invokeErr, postResultErr)
}

func factoryInvocationResultFromSessionInvocation(
	result factorysessions.InvocationResult,
) interfaces.FactoryInvocationResult {
	return interfaces.FactoryInvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status:        interfaces.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}
}

type hostedInvocationFactoryEvents struct {
	consume factorysessions.FactoryEventConsumer
	cancel  context.CancelFunc
	done    chan struct{}
	seen    map[string]struct{}
}

func startHostedInvocationFactoryEvents(
	ctx context.Context,
	reader factorysessions.Service,
	consume factorysessions.FactoryEventConsumer,
) (*hostedInvocationFactoryEvents, error) {
	if consume == nil {
		return nil, nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := reader.SubscribeFactoryEventsForSession(
		streamCtx, factorysessions.DefaultSessionID, nil,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe invocation Factory Events: %w", err)
	}
	if stream == nil || stream.Events == nil {
		cancel()
		return nil, errors.New("subscribe invocation Factory Events: stream is unavailable")
	}
	live := &hostedInvocationFactoryEvents{
		consume: consume, cancel: cancel, done: make(chan struct{}), seen: make(map[string]struct{}),
	}
	live.presentUnseen(stream.History)
	go func() {
		defer close(live.done)
		for event := range stream.Events {
			live.presentUnseen([]interfaces.FactoryEvent{event})
		}
	}()
	return live, nil
}

func (live *hostedInvocationFactoryEvents) finish(
	ctx context.Context,
	reader factorysessions.Service,
	result interfaces.FactoryInvocationResult,
) error {
	live.cancel()
	<-live.done
	stream, err := readHostedInvocationFactoryEvents(ctx, reader, result)
	if err != nil {
		return err
	}
	live.presentUnseen(stream)
	return nil
}

func (live *hostedInvocationFactoryEvents) presentUnseen(events []interfaces.FactoryEvent) {
	unseen := make([]interfaces.FactoryEvent, 0, len(events))
	for _, event := range events {
		if _, ok := live.seen[event.Id]; ok {
			continue
		}
		live.seen[event.Id] = struct{}{}
		unseen = append(unseen, event.Clone())
	}
	if len(unseen) > 0 {
		live.consume(unseen)
	}
}

func readHostedInvocationFactoryEvents(
	ctx context.Context,
	reader factorysessions.Service,
	result interfaces.FactoryInvocationResult,
) ([]interfaces.FactoryEvent, error) {
	if reader == nil {
		return nil, errors.New("Factory Session event reader is required")
	}
	var (
		stream *interfaces.FactoryEventStream
		err    error
	)
	if strings.TrimSpace(result.SessionID) != "" && result.SessionID != factorysessions.DefaultSessionID {
		stream, err = reader.ReadDurableFactorySessionEventStream(
			ctx, result.SessionID, factorysessions.EventReconnectRequest{},
		)
	} else {
		stream, err = reader.SubscribeFactoryEventsForSession(
			ctx, factorysessions.DefaultSessionID, nil,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("read invocation Factory Events: %w", err)
	}
	if stream == nil {
		return nil, errors.New("read invocation Factory Events: stream is unavailable")
	}
	events := make([]interfaces.FactoryEvent, len(stream.History))
	for index := range stream.History {
		events[index] = stream.History[index].Clone()
	}
	return events, nil
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
	result := outcome.Result
	if result.Status == "" {
		if err == nil {
			// InvokeFactory returned neither a determined terminal result nor
			// an error: without this explicit invariant failure, a nil err
			// maps to a nil CLI error (MapInvocationFailure(nil) == nil),
			// which would silently report success and omit the public
			// terminal record contract this invocation type owes its caller.
			err = fmt.Errorf("run factory invocation: invocation ended without a determined terminal result")
		}
		return MapInvocationFailure(err)
	}
	// A terminal result was determined even though err is non-nil: err is a
	// post-result failure (for example runtime teardown or resource cleanup)
	// that races the invocation's own completion. The public terminal record
	// must still be written for the outcome the invocation actually reached;
	// err is preserved below so the CLI still reports failure and exit-code
	// semantics for the cleanup error are not lost.
	var writeErr error
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		writeErr = writeInvocationFailure(invocationCfg, result, streamRenderer)
	} else {
		writeErr = writeInvocationSuccess(invocationCfg, result, streamRenderer)
	}
	if err != nil {
		if writeErr != nil {
			return errors.Join(MapInvocationFailure(err), writeErr)
		}
		return MapInvocationFailure(err)
	}
	return writeErr
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
	mockWorkersConfig *workers.MockWorkersConfig,
) factorysessions.InvocationTarget {
	return factorysessions.InvocationTarget{
		FactoryDir:            cfg.Dir,
		FactorySourcePath:     cfg.FactoryConfigPath,
		RunnerID:              cfg.RunnerID,
		WorkerReasoningEffort: cfg.WorkerReasoningEffort,
		Worktree:              cfg.Worktree,
		OperatorDefaults:      cfg.OperatorDefaults,
		ExecutionBaseDir:      cfg.ExecutionBaseDir,
		HomeDir:               cfg.HomeDir,
		Verbose:               cfg.Verbose,
		RecordPath:            cfg.RecordPath,
		ReplayPath:            cfg.ReplayPath,
		RuntimeLogDir:         cfg.RuntimeLogDir,
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
