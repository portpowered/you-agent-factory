package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/batchload"
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
	presentations factorysessions.OpeningPresentationOwner,
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
		openingPresentations: presentations,
	}, nil
}

func resolveFactoryInvocationRequest(cfg RunConfig) (*factoryapi.InvocationRequest, bool, error) {
	return resolveFactoryInvocationRequestForRun(cfg, nil)
}

// resolveFactoryInvocationRequestForRun projects the selected run input onto
// the Sessions invocation contract. A selected, finite --work request is a
// strict one-item compatibility invocation; ordinary batch/service runs keep
// their startup Work file and do not enter this path.
func resolveFactoryInvocationRequestForRun(
	cfg RunConfig,
	prepareWorkTarget work.SingleWorkTargetPreparation,
) (*factoryapi.InvocationRequest, bool, error) {
	if strings.TrimSpace(cfg.WorkFile) != "" {
		if !cfg.CleanInvocation {
			return nil, false, nil
		}
		return invocationRequestFromWorkFile(cfg, prepareWorkTarget)
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

func invocationRequestFromWorkFile(
	cfg RunConfig,
	prepareWorkTarget work.SingleWorkTargetPreparation,
) (*factoryapi.InvocationRequest, bool, error) {
	if cfg.WorkRequestFileLoader == nil {
		return nil, true, fmt.Errorf("resolve --work invocation: Work Request file loader is required")
	}
	request, err := batchload.LoadFromFile(cfg.WorkRequestFileLoader, cfg.WorkFile)
	if err != nil {
		return nil, true, err
	}
	if prepareWorkTarget == nil {
		return nil, true, fmt.Errorf("resolve --work invocation: Work single-target preparation is required")
	}
	if _, err := prepareWorkTarget(request); err != nil {
		return nil, true, err
	}
	if len(request.Works) != 1 {
		return nil, true, fmt.Errorf("resolve --work invocation: request requires exactly one work item")
	}

	item := request.Works[0]
	content, err := invocationContentFromWorkItem(item)
	if err != nil {
		return nil, true, err
	}
	if len(content) == 0 {
		return nil, true, fmt.Errorf("resolve --work invocation: work content is required")
	}

	sourceKind := factoryapi.InvocationInputSourceKindText
	generatedContent := *contentcontract.GeneratedPtrFromParts(content)
	result := &factoryapi.InvocationRequest{
		Content:    &generatedContent,
		SourceKind: &sourceKind,
	}
	requestID := strings.TrimSpace(request.RequestID)
	if requestID == "" {
		requestID = strings.TrimSpace(item.RequestID)
	}
	if requestID == "" {
		// The Sessions contract has no WorkID carrier. Using an authored WorkID
		// as the idempotency key keeps repeated clean runs addressable without
		// reconstructing a runtime token or dispatch history in the CLI.
		requestID = strings.TrimSpace(item.WorkID)
	}
	if requestID != "" {
		result.RequestId = &requestID
	}
	return result, true, nil
}

func invocationContentFromWorkItem(item work.Work) ([]work.WorkContentPart, error) {
	if len(item.Content) > 0 {
		return work.CloneWorkContentParts(item.Content), nil
	}
	if item.Payload == nil {
		return nil, nil
	}
	if text, ok := item.Payload.(string); ok {
		return []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: text}}, nil
	}
	encoded, err := json.Marshal(item.Payload)
	if err != nil {
		return nil, fmt.Errorf("resolve --work invocation payload: %w", err)
	}
	return []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: encoded}}, nil
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

// hostedInvocationOperation keeps the already-opened application runtime at
// the CLI composition edge. InvocationTarget remains detached configuration;
// the hosted capability itself is an operation-valued result from application
// opening rather than a service table retained in the opening request.
type hostedInvocationOperation struct {
	delegate      InvocationOperation
	hosted        HostedInvocationOperation
	logger        *zap.Logger
	presentations factorysessions.OpeningPresentationOwner
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
) (factorysessions.FactoryInvocationOutcome, error) {
	if operation == nil || operation.delegate == nil {
		return factorysessions.FactoryInvocationOutcome{}, errors.New("hosted invocation operation is required")
	}
	hosted := operation.hosted
	if hosted == nil {
		return factorysessions.FactoryInvocationOutcome{}, errors.New("hosted invocation operation is incomplete")
	}
	sessionID := strings.TrimSpace(target.FactorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	if isJavaScriptHostedFactory, probeErr := hostedFactoryUsesJavaScriptOrchestrator(ctx, hosted, sessionID); probeErr != nil {
		return factorysessions.FactoryInvocationOutcome{}, probeErr
	} else if isJavaScriptHostedFactory {
		return operation.delegate.InvokeFactory(ctx, target, request)
	}
	var bridge interface {
		Finish(context.Context, factoryEventReader, factorysessions.FactoryInvocationOutcome) error
	}
	if target.EventScopeID != "" {
		if operation.presentations == nil {
			return factorysessions.FactoryInvocationOutcome{}, errors.New("invocation presentation owner is required")
		}
		var bridgeErr error
		bridge, bridgeErr = operation.presentations.StartFactoryEventBridge(ctx, hosted, target.EventScopeID)
		if bridgeErr != nil {
			return factorysessions.FactoryInvocationOutcome{}, bridgeErr
		}
	}
	invocationResult, invokeErr := hosted.InvokeFactorySession(
		ctx, sessionID, request,
	)
	outcome := factorysessions.FactoryInvocationOutcome{
		Result: factoryInvocationResultFromSessionInvocation(invocationResult),
	}
	if bridge == nil {
		return outcome, invokeErr
	}
	postResultErr := bridge.Finish(ctx, hosted, outcome)
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

func hostedFactoryUsesJavaScriptOrchestrator(
	ctx context.Context,
	hosted HostedInvocationOperation,
	sessionID string,
) (bool, error) {
	projectionReader, ok := hosted.(interface {
		GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	})
	if !ok {
		return false, nil
	}
	projection, projectionErr := projectionReader.GetFactorySession(ctx, sessionID)
	if projectionErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		return false, nil
	}
	return interfaces.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg), nil
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

func runFactoryInvocation(
	ctx context.Context,
	cfg RunConfig,
	target factorysessions.InvocationTarget,
	request factoryapi.InvocationRequest,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	presentations factorysessions.OpeningPresentationOwner,
) error {
	_, err := runFactoryInvocationWithResult(
		ctx, cfg, target, request, invocation, presentation, presentations,
	)
	return err
}

func runFactoryInvocationWithResult(
	ctx context.Context,
	cfg RunConfig,
	target factorysessions.InvocationTarget,
	request factoryapi.InvocationRequest,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	presentations factorysessions.OpeningPresentationOwner,
) (apisurface.FactoryInvocationResult, error) {
	if invocation == nil {
		return apisurface.FactoryInvocationResult{}, fmt.Errorf("run factory invocation: operation is required")
	}

	logPackagedTTSInvocationStart(cfg)

	invocationCfg := cfg
	invokeCtx := ctx
	var outputWriter *responseStreamCancelOnWriteError
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) && cfg.Output != nil {
		var cancel context.CancelFunc
		invokeCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		outputWriter = newResponseStreamCancelOnWriteError(cfg.Output, cancel)
		invocationCfg.Output = outputWriter
	}

	streamRenderer, err := invocationFactoryEventRenderer(invocationCfg, presentation)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, err
	}
	if streamRenderer != nil {
		defer streamRenderer.StopProgressRendering()
	}
	if streamRenderer != nil && presentations != nil {
		scopeID, registerErr := presentations.RegisterInvocationEvents(factorysessions.InvocationEventScope{
			FactorySessionID: cfg.FactorySessionID,
			Consume:          streamRenderer.PresentFactoryEvents,
		})
		if registerErr != nil {
			return apisurface.FactoryInvocationResult{}, fmt.Errorf("register invocation event presentation: %w", registerErr)
		}
		defer presentations.Close(scopeID)
		target.EventScopeID = scopeID
	}
	invocationRequest := factorysessionmapping.InvocationRequestFromAPI(request)
	if cfg.PreparedInvocationInput != nil {
		invocationRequest.Args = nil
		invocationRequest.Content = nil
		invocationRequest.ContentProvided = false
		invocationRequest.PreparedInvocationInput = cfg.PreparedInvocationInput.Clone()
	}
	outcome, err := invocation.InvokeFactory(invokeCtx, target, invocationRequest)
	result := outcome.Result
	if result.Status == "" {
		return apisurface.FactoryInvocationResult{}, runFactoryInvocationWithoutTerminalResult(err, outputWriter, streamRenderer)
	}
	// A terminal result was determined even though err is non-nil: err is a
	// post-result failure (for example runtime teardown or resource cleanup)
	// that races the invocation's own completion. The public terminal record
	// must still be written for the outcome the invocation actually reached;
	// err is preserved below so the CLI still reports failure and exit-code
	// semantics for the cleanup error are not lost.
	writeErr := writeFactoryInvocationOutcome(invocationCfg, result, streamRenderer)
	return result, finishFactoryInvocation(err, writeErr, outputWriter, result)
}

func runFactoryInvocationWithoutTerminalResult(
	err error,
	outputWriter *responseStreamCancelOnWriteError,
	streamRenderer interface{ StopProgressRendering() },
) error {
	// A lossless response stream writes Factory Events on its own drain
	// goroutine. A writer failure cancels invokeCtx immediately, so the
	// invocation can return before that goroutine has recorded the failure.
	// Drain the stream before classifying an undetermined outcome so the
	// caller receives the writer failure instead of a generic cancellation.
	if outputWriter != nil && streamRenderer != nil {
		streamRenderer.StopProgressRendering()
		if writeErr := outputWriter.Err(); writeErr != nil {
			return MapInvocationFailure(writeErr)
		}
	}
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

type responseStreamCancelOnWriteError struct {
	writer   io.Writer
	onError  func()
	writeErr atomic.Pointer[InvocationError]
}

func responseStreamOutputCancelOnWriteError(writer io.Writer, onError context.CancelFunc) io.Writer {
	if writer == nil || onError == nil {
		return writer
	}
	return newResponseStreamCancelOnWriteError(writer, onError)
}

func newResponseStreamCancelOnWriteError(writer io.Writer, onError context.CancelFunc) *responseStreamCancelOnWriteError {
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
		writeErr := &InvocationError{
			Code:    InvocationErrorCodeFailed,
			Message: err.Error(),
			Cause:   err,
		}
		if writer.writeErr.CompareAndSwap(nil, writeErr) {
			writer.onError()
		}
	}
	return written, err
}

func (writer *responseStreamCancelOnWriteError) Err() error {
	if writer == nil {
		return nil
	}
	recorded := writer.writeErr.Load()
	if recorded == nil {
		return nil
	}
	return recorded
}

func invocationTarget(
	cfg RunConfig,
	mockWorkersConfig *workers.MockWorkersConfig,
) factorysessions.InvocationTarget {
	return factorysessions.InvocationTarget{
		FactorySessionID:      cfg.FactorySessionID,
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
		ResumePath:            cfg.ResumePath,
		CanonicalSessionID:    cfg.CanonicalSessionID,
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
		InvocationArguments:     work.CloneInvocationArguments(cfg.InvocationArguments),
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

func waitForRemoteInvocationResult(
	ctx context.Context,
	cfg RunConfig,
	server string,
	start factoryapi.FactorySessionExecutionResponse,
	requestID string,
	operation RemoteInvocationResultOperation,
) (apisurface.FactoryInvocationResult, error) {
	if ctx == nil {
		return apisurface.FactoryInvocationResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "wait for remote durable result: context is required",
		}
	}
	if operation == nil {
		return apisurface.FactoryInvocationResult{}, &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: "wait for remote durable result: operation is required",
		}
	}
	var invocationResult apisurface.FactoryInvocationResult
	_, err := factorysessionscli.Poll(
		ctx,
		remoteDurableResultPollInterval,
		func(readCtx context.Context) (factoryapi.FactorySessionResult, error) {
			return operation.GetFactorySessionResult(readCtx, RemoteInvocationResultRequest{
				Server:      server,
				SessionID:   start.SessionId,
				Diagnostics: cfg.Diagnostics,
				Verbose:     cfg.Verbose,
			})
		},
		func(result factoryapi.FactorySessionResult) (bool, error) {
			mapped, ready, poll, mapErr := remoteInvocationResultFromDurable(
				result,
				start.SessionId,
				requestID,
			)
			if mapErr != nil {
				return false, mapErr
			}
			if ready {
				invocationResult = mapped
				return true, nil
			}
			if !poll {
				return false, &InvocationError{
					Code:    RemoteDurableResponseInvalidCode,
					Message: "remote durable result ended without a terminal classification",
				}
			}
			return false, nil
		},
	)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			endpoint := safeRemoteEndpoint(server)
			return apisurface.FactoryInvocationResult{}, &InvocationError{
				Code: RemoteDurableResultCode,
				Message: fmt.Sprintf(
					"remote durable result wait canceled at %s: %v",
					endpoint,
					err,
				),
				Cause: err,
			}
		}
		return apisurface.FactoryInvocationResult{}, err
	}
	return invocationResult, nil
}

func remoteInvocationResultFromDurable(
	result factoryapi.FactorySessionResult,
	expectedSessionID string,
	requestID string,
) (apisurface.FactoryInvocationResult, bool, bool, error) {
	expectedSessionID = strings.TrimSpace(expectedSessionID)
	actualSessionID := strings.TrimSpace(result.SessionId)
	if expectedSessionID == "" || actualSessionID == "" || actualSessionID != expectedSessionID {
		return apisurface.FactoryInvocationResult{}, false, false, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote durable result returned a session identity different from the accepted session",
		}
	}
	if strings.TrimSpace(string(result.ResultStatus)) == "" {
		return apisurface.FactoryInvocationResult{}, false, false, &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: fmt.Sprintf("remote durable result for session %s has no result status", actualSessionID),
		}
	}

	parts := contentcontract.PartsFromGenerated(result.PrimaryResult)
	status, code, message, terminalFailure := remoteDurableFailureClassification(result)
	if terminalFailure {
		return remoteDurableInvocationFailure(
			requestID,
			actualSessionID,
			status,
			code,
			message,
			parts,
		), true, false, nil
	}

	if result.ResultStatus == factoryapi.FactorySessionResultStatusFinal {
		if len(parts) == 0 {
			return remoteDurableInvocationFailure(
				requestID,
				actualSessionID,
				interfaces.InvocationTerminalStatusFailed,
				string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),

				remoteDurableResultMessage(result, "primary result could not be resolved"),
				nil,
			), true, false, nil
		}
		return apisurface.FactoryInvocationResult{
			RequestID:     strings.TrimSpace(requestID),
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: parts,
			SessionID:     actualSessionID,
		}, true, false, nil
	}

	if remoteDurableResultShouldPoll(result) {
		return apisurface.FactoryInvocationResult{}, false, true, nil
	}

	return remoteDurableInvocationFailure(
		requestID,
		actualSessionID,
		interfaces.InvocationTerminalStatusFailed,
		string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
		remoteDurableResultMessage(result, "primary result could not be resolved"),
		parts,
	), true, false, nil
}

func remoteDurableInvocationFailure(
	requestID string,
	sessionID string,
	status interfaces.InvocationTerminalStatus,
	code string,
	message string,
	parts []work.WorkContentPart,
) apisurface.FactoryInvocationResult {
	return apisurface.FactoryInvocationResult{
		RequestID:     strings.TrimSpace(requestID),
		Status:        status,
		PrimaryResult: parts,
		ErrorCode:     strings.TrimSpace(code),
		Message:       strings.TrimSpace(message),
		SessionID:     strings.TrimSpace(sessionID),
	}
}

func remoteDurableFailureClassification(
	result factoryapi.FactorySessionResult,
) (interfaces.InvocationTerminalStatus, string, string, bool) {
	lifecycle := remoteDurableLifecycleStatus(result.SessionStatus)
	switch lifecycle {
	case string(factoryapi.FactorySessionDurableLifecycleStatusAwaitingApproval):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONNEEDSHUMAN),
			remoteDurableResultMessage(result, "factory session is awaiting human approval"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusPaused):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPAUSED),
			remoteDurableResultMessage(result, "factory session is paused; resume the session to continue waiting for the primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusTimedOut):
		return interfaces.InvocationTerminalStatusTimedOut,
			string(factoryapi.INVOCATIONTIMEDOUT),
			remoteDurableResultMessage(result, "invocation timed out while waiting for primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusCanceled):
		return interfaces.InvocationTerminalStatusCanceled,
			string(factoryapi.INVOCATIONCANCELED),
			remoteDurableResultMessage(result, "invocation was canceled while waiting for primary result"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusInterrupted), string(factoryapi.FactorySessionDurableLifecycleStatusTerminated):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONINTERRUPTED),
			remoteDurableResultMessage(result, "invocation was interrupted before the primary result was available"),
			true
	case string(factoryapi.FactorySessionDurableLifecycleStatusFailed):
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONRUNTIMEFAILURE),
			remoteDurableResultMessage(result, "remote Factory Session failed before the primary result was available"),
			true
	}

	for _, rawReason := range []string{remoteDurableAvailabilityReason(result), remoteDurableFailureReason(result)} {
		if status, code, fallback, ok := remoteDurableReasonClassification(rawReason); ok {
			return status, code, remoteDurableResultMessage(result, fallback), true
		}
	}

	if result.ResultStatus == factoryapi.FactorySessionResultStatusFailedWithPartial {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONRUNTIMEFAILURE),
			remoteDurableResultMessage(result, "remote Factory Session failed before the primary result was available"),
			true
	}
	if result.ResultStatus == factoryapi.FactorySessionResultStatusPartial && lifecycle == string(factoryapi.FactorySessionDurableLifecycleStatusSucceeded) {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
			remoteDurableResultMessage(result, "primary result could not be resolved"),
			true
	}
	if result.ResultStatus == factoryapi.FactorySessionResultStatusUnavailable && !remoteDurableResultShouldPoll(result) {
		return interfaces.InvocationTerminalStatusFailed,
			string(factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED),
			remoteDurableResultMessage(result, "primary result could not be resolved"),
			true
	}
	return "", "", "", false
}

func remoteDurableReasonClassification(
	rawReason string,
) (interfaces.InvocationTerminalStatus, string, string, bool) {
	reason := strings.ToUpper(strings.TrimSpace(rawReason))
	switch {
	case strings.Contains(reason, "BLOCKED"):
		return interfaces.InvocationTerminalStatusFailed, string(factoryapi.INVOCATIONBLOCKED), "invocation was blocked before the primary result was available", true
	case strings.Contains(reason, "NEEDS_HUMAN"), strings.Contains(reason, "NEEDS-HUMAN"), strings.Contains(reason, "APPROVAL"):
		return interfaces.InvocationTerminalStatusFailed, string(factoryapi.INVOCATIONNEEDSHUMAN), "factory session is awaiting human approval", true
	case strings.Contains(reason, "PAUSED"):
		return interfaces.InvocationTerminalStatusFailed, string(factoryapi.INVOCATIONPAUSED), "factory session is paused; resume the session to continue waiting for the primary result", true
	case strings.Contains(reason, "TIMEOUT"), strings.Contains(reason, "TIMED_OUT"):
		return interfaces.InvocationTerminalStatusTimedOut, string(factoryapi.INVOCATIONTIMEDOUT), "invocation timed out while waiting for primary result", true

	case strings.Contains(reason, "CANCELED"), strings.Contains(reason, "CANCELLED"):
		return interfaces.InvocationTerminalStatusCanceled, string(factoryapi.INVOCATIONCANCELED), "invocation was canceled while waiting for primary result", true
	case strings.Contains(reason, "INTERRUPT"), strings.Contains(reason, "TERMINAT"):
		return interfaces.InvocationTerminalStatusFailed, string(factoryapi.INVOCATIONINTERRUPTED), "invocation was interrupted before the primary result was available", true
	default:
		return "", "", "", false
	}
}

func remoteDurableResultShouldPoll(result factoryapi.FactorySessionResult) bool {
	if result.Availability != nil && result.Availability.Retryable != nil && !*result.Availability.Retryable {
		return false
	}
	lifecycle := remoteDurableLifecycleStatus(result.SessionStatus)
	switch lifecycle {
	case "", string(factoryapi.FactorySessionDurableLifecycleStatusQueued), string(factoryapi.FactorySessionDurableLifecycleStatusRunning), string(factoryapi.FactorySessionDurableLifecycleStatusResuming), string(factoryapi.FactorySessionDurableLifecycleStatusCanceling):
		return result.ResultStatus == factoryapi.FactorySessionResultStatusNotReady || result.ResultStatus == factoryapi.FactorySessionResultStatusPartial || result.ResultStatus == factoryapi.FactorySessionResultStatusUnavailable
	default:
		return false
	}
}

func remoteDurableResultMessage(result factoryapi.FactorySessionResult, fallback string) string {
	if result.FailureDetail != nil && strings.TrimSpace(result.FailureDetail.Message) != "" {
		return strings.TrimSpace(result.FailureDetail.Message)
	}
	if result.Availability != nil && result.Availability.Message != nil && strings.TrimSpace(*result.Availability.Message) != "" {
		return strings.TrimSpace(*result.Availability.Message)
	}
	return fallback
}

func remoteDurableAvailabilityReason(result factoryapi.FactorySessionResult) string {
	if result.Availability == nil || result.Availability.Reason == nil {
		return ""
	}
	return *result.Availability.Reason
}

func remoteDurableFailureReason(result factoryapi.FactorySessionResult) string {
	if result.FailureDetail == nil {
		return ""
	}
	return string(result.FailureDetail.Reason)
}

// RunRemoteInvocation starts one server-owned durable Factory Session through
// the selected remote adapter. It does not open local runtime state or use the
// live-session compatibility invocation route.
