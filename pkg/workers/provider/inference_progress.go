package provider

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryresponseevents "github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	interfaceresponseevents "github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	agyadapter "github.com/portpowered/infinite-you/pkg/workers/provider/agy"
	codexprogress "github.com/portpowered/infinite-you/pkg/workers/provider/codex/progress"
	cursorprogress "github.com/portpowered/infinite-you/pkg/workers/provider/cursor/progress"
)

const (
	ProgressFragmentKind         = "PROGRESS_FRAGMENT"
	ResponseFragmentKind         = "RESPONSE_FRAGMENT"
	CompletedFragmentKind        = "STREAM_COMPLETED"
	FailedFragmentKind           = "STREAM_FAILED"
	NormalizedEventTypeUnknown   = "UNKNOWN"
	NormalizedEventTypeStarted   = "STARTED"
	NormalizedEventTypeProgress  = "PROGRESS"
	NormalizedEventTypeTextDelta = "TEXT_DELTA"
	NormalizedEventTypeFinalText = "FINAL_TEXT"
	NormalizedEventTypeFailed    = "FAILED"
	NormalizedEventTypeCanceled  = "CANCELED"
)

const (
	codexRetainedTextBytes       = codexprogress.ProgressRetainedTextBytes
	codexRetainedProgressBytes   = codexprogress.ProgressRetainedProgressBytes
	codexMetadataRunnerIDKey     = codexprogress.ProgressMetadataRunnerIDKey
	codexMetadataWorkIDKey       = codexprogress.ProgressMetadataWorkIDKey
	codexMetadataWorkstationKey  = codexprogress.ProgressMetadataWorkstationKey
	codexMetadataTextBytesKey    = codexprogress.ProgressMetadataTextBytesKey
	codexMetadataTruncatedKey    = codexprogress.ProgressMetadataTruncatedKey
	codexMetadataRawBytesKey     = codexprogress.ProgressMetadataRawBytesKey
	codexMetadataRawSHA256Key    = codexprogress.ProgressMetadataRawSHA256Key
	codexMetadataDiagnosticKey   = codexprogress.ProgressMetadataDiagnosticKey
	codexDiagnosticUnknownEvent  = codexprogress.ProgressDiagnosticUnknownEvent
	codexDiagnosticMalformedJSON = codexprogress.ProgressDiagnosticMalformedJSON
	codexDiagnosticIncompleteSSE = codexprogress.ProgressDiagnosticIncompleteSSE
)

func isCodexCommand(command string) bool {
	return codexprogress.IsCommand(command)
}

// InferenceProgressFragment is the provider-boundary shape for transient internal
// session progress that must not enter canonical factory event history.
type InferenceProgressFragment struct {
	DispatchID         string
	Kind               string
	Type               string
	Payload            string
	ProviderSessionRef *workerexecution.ProviderSessionMetadata
	ExternalEventType  string
	Metadata           map[string]string
	CanonicalDraft     any
	// CanonicalEventAlreadyPublished keeps a compatibility terminal marker
	// from projecting a second canonical failure after a native terminal draft.
	CanonicalEventAlreadyPublished bool
}

// CanonicalDraftFragment carries one provider-native canonical response draft
// to the session-owned publisher without flattening it into a legacy fragment.
func CanonicalDraftFragment(dispatchID string, draft any) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:     strings.TrimSpace(dispatchID),
		CanonicalDraft: draft,
	}
}

// InferenceProgressPublisher receives provider progress fragments for one live
// Factory Session internal response stream.
type InferenceProgressPublisher func(fragment InferenceProgressFragment)

// ProgressFragment builds one ordered progress fragment for a dispatch.
func ProgressFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ProgressFragmentKind,
		Type:               NormalizedEventTypeProgress,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// ResponseFragment builds one ordered response fragment for a dispatch.
func ResponseFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ResponseFragmentKind,
		Type:               NormalizedEventTypeTextDelta,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// CompletedFragment builds one terminal completion marker for a dispatch.
func CompletedFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               CompletedFragmentKind,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// FailedFragment builds one terminal failure marker for a dispatch.
func FailedFragment(dispatchID string, providerSession *workerexecution.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               FailedFragmentKind,
		Payload:            payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(providerSession),
	}
}

// StructuredResponseEvent carries one provider-neutral draft to the
// session-owned response-event publisher without converting it to a legacy
// response or progress fragment.
func StructuredResponseEvent(draft factoryresponseevents.Draft) InferenceProgressFragment {
	cloned := draft
	cloned.Payload = append(json.RawMessage(nil), draft.Payload...)
	return CanonicalDraftFragment(draft.DispatchID, cloned)
}

// InferenceProgressPublishingCommandRunner publishes internal response-stream
// fragments while provider subprocess stdout/stderr grow.
type InferenceProgressPublishingCommandRunner struct {
	Publisher InferenceProgressPublisher
	Logger    logging.Logger
}

// SupportsResponseStreaming reports that the runner observes subprocess output
// incrementally and can therefore consume native streaming protocols.
func (InferenceProgressPublishingCommandRunner) SupportsResponseStreaming() bool { return true }

// Run executes the provider subprocess and publishes incremental stdout/stderr
// fragments into the configured internal session response stream.
func (r InferenceProgressPublishingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Publisher == nil {
		return workerprocess.ExecCommandRunner{}.Run(ctx, req)
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	progressStream := newProgressStreamObserver(req, r.Publisher, r.Logger)
	observer := func(stream string, chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		if progressStream != nil && progressStream.observe(ctx, stream, chunk) {
			return
		}
		payload := string(chunk)
		switch stream {
		case workerprocess.OutputStreamStdout:
			r.Publisher(ResponseFragment(dispatchID, nil, payload))
		case workerprocess.OutputStreamStderr:
			r.Publisher(ProgressFragment(dispatchID, nil, payload))
		}
	}
	result, err := workerprocess.StreamingExecCommandRunner{
		Observer: observer,
		Logger:   logging.EnsureLogger(r.Logger),
	}.Run(ctx, req)
	if progressStream != nil {
		progressStream.flush(ctx, result, err)
	}
	return result, err
}

// NewInferenceProgressPublishingCommandRunner constructs a provider command
// runner that publishes internal response-stream fragments during subprocess IO.
func NewInferenceProgressPublishingCommandRunner(
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	if publisher == nil {
		return workerprocess.ExecCommandRunner{}
	}
	return InferenceProgressPublishingCommandRunner{
		Publisher: publisher,
		Logger:    logger,
	}
}

func (p *ScriptWrapProvider) executeAgy(
	ctx context.Context,
	req workerexecution.ProviderInferenceRequest,
	logger logging.Logger,
) (workerexecution.InferenceResponse, error) {
	factoryRoot := strings.TrimSpace(p.agyFactoryRoot)
	if factoryRoot == "" {
		return workerexecution.InferenceResponse{}, p.agyRequestValidationError(req, errors.New("Agy factory root is unavailable"))
	}
	providerAdapter := agyadapter.NewAdapter(factoryRoot)
	if p.agyAllocator != nil {
		var err error
		providerAdapter, err = agyadapter.NewAdapterWithAllocator(factoryRoot, p.agyAllocator)
		if err != nil {
			return workerexecution.InferenceResponse{}, p.agyRequestValidationError(req, err)
		}
	}
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		return workerexecution.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		return workerexecution.InferenceResponse{}, p.agyRequestValidationError(req, err)
	}
	started := time.Now()
	result, executeErr := adapter.Execute(ctx, registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command:  adapter.CommandContext{Request: req, SkipPermissions: p.SkipPermissions, MaterializeOptions: p.MaterializeOptions},
		Decoder:  adapter.DecoderContext{RunID: req.Dispatch.DispatchID, DispatchID: req.Dispatch.DispatchID},
		ObserveDraft: func(draft interfaceresponseevents.Draft) {
			if p.progressPublisher != nil {
				p.progressPublisher(CanonicalDraftFragment(draft.DispatchID, draft))
			}
		},
	})
	duration := time.Since(started)
	diagnostics := commandDiagnostics(result.Request, result.Command, duration, result.Outcome == adapter.CommandOutcomeCanceled)
	if result.Failure != nil {
		providerErr := providerErrorFromAdapterFailure(result.Failure, executeErr, diagnostics)
		logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return workerexecution.InferenceResponse{}, providerErr
	}
	if executeErr != nil {
		if orchestrated := agyadapter.ClassifyOrchestrationError(executeErr); orchestrated.Failure != nil {
			providerErr := providerErrorFromAdapterFailure(orchestrated.Failure, executeErr, diagnostics)
			logger.Error("provider failure normalized", providerFailureLogFields(req, providerErr, result.Command, duration)...)
			p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
			return workerexecution.InferenceResponse{}, providerErr
		}
		providerErr := normalizeProviderExecutionError(req.ModelProvider, result.Command, executeErr, result.Response.ProviderSession, diagnostics)
		p.publishOpenCodeFailure(req.Dispatch.DispatchID, providerErr, len(result.Drafts) > 0)
		return workerexecution.InferenceResponse{}, providerErr
	}
	response := result.Response
	response.Diagnostics = diagnostics
	logger.Info("inferencer: request completed",
		appendProviderSessionLogFields(providerLogFields(req, "output_len", len(response.Content)), response.ProviderSession)...)
	p.publishOpenCodeCompleted(req.Dispatch.DispatchID, response.ProviderSession, len(result.Drafts) > 0)
	return response, nil
}

func (p *ScriptWrapProvider) agyRequestValidationError(req workerexecution.ProviderInferenceRequest, err error) *ProviderError {
	providerErr := newProviderErrorWithDiagnostics(
		workerexecution.WorkFailureTypePermanentBadRequest,
		err.Error(),
		err,
		nil,
		workDiagnosticsForInferenceRequest(req),
	)
	p.publishFailureFragment(req.Dispatch.DispatchID, nil, providerErr)
	return providerErr
}

type progressStreamObserver interface {
	observe(ctx context.Context, stream string, chunk []byte) bool
	flush(ctx context.Context, result CommandResult, err error)
}

func progressStreamIdentity(command string) adapter.Identity {
	if cursorprogress.IsCommand(command) {
		return adapter.Identity(modelprovider.Cursor)
	}
	if codexprogress.IsCommand(command) {
		return adapter.Identity(modelprovider.Codex)
	}
	return adapter.NormalizeIdentity(adapter.Identity(command))
}

func newProgressStreamObserver(
	req CommandRequest,
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) progressStreamObserver {
	dispatchID := strings.TrimSpace(req.DispatchID)
	switch progressStreamIdentity(req.Command) {
	case adapter.Identity(modelprovider.Cursor):
		return &cursorProgressObserver{
			stream: cursorprogress.NewResponseEventStream(dispatchID, func(fragment cursorprogress.ProgressFragment) {
				publisher(inferenceProgressFragmentFromCursor(fragment))
			}, logger),
		}
	case adapter.Identity(modelprovider.Codex):
		return &codexProgressObserver{
			stream: codexprogress.NewProgressStream(req, func(fragment codexprogress.ProgressFragment) {
				publisher(inferenceProgressFragmentFromCodex(fragment))
			}),
		}
	default:
		return nil
	}
}

type cursorProgressObserver struct {
	stream *cursorprogress.ResponseEventStream
}

func (o *cursorProgressObserver) observe(ctx context.Context, stream string, chunk []byte) bool {
	o.stream.Observe(ctx, stream, chunk)
	return true
}

func (o *cursorProgressObserver) flush(ctx context.Context, result CommandResult, err error) {
	o.stream.Flush(ctx, cursorprogress.FlushReason(ctx, result.ExitCode, err))
}

type codexProgressObserver struct {
	stream *codexprogress.ProgressStream
}

func (o *codexProgressObserver) observe(_ context.Context, stream string, chunk []byte) bool {
	return o.stream.Observe(stream, chunk)
}

func (o *codexProgressObserver) flush(_ context.Context, _ CommandResult, _ error) {
	o.stream.Flush()
}

func inferenceProgressFragmentFromCursor(fragment cursorprogress.ProgressFragment) InferenceProgressFragment {
	if fragment.HasCanonicalDraft {
		return StructuredResponseEvent(fragment.CanonicalDraft)
	}
	return InferenceProgressFragment{
		DispatchID:        fragment.DispatchID,
		Kind:              fragment.Kind,
		Payload:           fragment.Payload,
		ExternalEventType: fragment.ExternalEventType,
	}
}

func inferenceProgressFragmentFromCodex(fragment codexprogress.ProgressFragment) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         fragment.DispatchID,
		Kind:               fragment.Kind,
		Type:               fragment.Type,
		Payload:            fragment.Payload,
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		ExternalEventType:  fragment.ExternalEventType,
		Metadata:           fragment.Metadata,
	}
}
