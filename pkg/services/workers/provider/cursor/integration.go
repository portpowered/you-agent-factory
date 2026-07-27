package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// IntegrationDependencies are Cursor's injected subprocess and temporary-file
// effects plus the platform facts needed for Windows prompt materialization.
type IntegrationDependencies struct {
	CommandRunner   workerprocess.CommandRunner
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// Integration owns Cursor invocation through the neutral response protocol.
type Integration struct {
	dependencies IntegrationDependencies
}

// NewIntegration constructs Cursor's registry-ready integration.
func NewIntegration(dependencies ...IntegrationDependencies) *Integration {
	integration := &Integration{}
	if len(dependencies) > 0 {
		integration.dependencies = dependencies[0]
	}
	return integration
}

func (*Integration) Identity() inference.Identity {
	return inference.Identity("cursor")
}

// MaximumCapabilities mirrors Cursor's accepted catalog manifest.
func (*Integration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilitySessionResume,
		inference.CapabilityNativeStreaming,
		inference.CapabilityMessageSnapshots,
		inference.CapabilityToolLifecycle,
		inference.CapabilityToolOutputDeltas,
		inference.CapabilityUsage,
		inference.CapabilityStableItemIDs,
	)
}

func (*Integration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}

func (i *Integration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

// Invoke publishes decoded Cursor stream records in subprocess order and
// closes the writer once with the authoritative parsed completion.
func (i *Integration) Invoke(
	ctx context.Context,
	request inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	requestedSession := cursorRequestedSession(request)
	providerAdapter := i.newAdapter(requestedSession)
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		return err
	}
	publication := &cursorPublication{ctx: ctx, writer: writer}
	started, err := cursorRunEvent(request.InvocationID(), workerexecution.PhaseStarted)
	if err != nil {
		return err
	}
	if err := publication.writeEvent(started); err != nil {
		return err
	}
	result, executeErr := adapter.Execute(ctx, registry, cursorStreamingRunner{
		runner: i.dependencies.CommandRunner,
	}, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{
			Request:         cursorRequestFromInvocation(request),
			SkipPermissions: request.Execution().SkipPermissions,
		},
		Decoder: adapter.DecoderContext{
			RunID:      request.InvocationID(),
			DispatchID: request.InvocationID(),
		},
		ObserveDraft: publication.write,
	})
	if err := publication.err(); err != nil {
		return err
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ctx.Err()
	}
	if result.Failure != nil {
		return writer.Close(ctx, inference.FailedCompletion(cursorInferenceFailure(*result.Failure)))
	}
	if executeErr != nil {
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureUnknown,
			Message: cursorUnknownFailureMessage,
		})))
	}
	completed, err := cursorRunEvent(request.InvocationID(), workerexecution.PhaseCompleted)
	if err != nil {
		return err
	}
	if err := publication.writeEvent(completed); err != nil {
		return err
	}
	return writer.Close(ctx, inference.SuccessfulCompletion(cursorInferenceResponse(result.Response)))
}

func (i *Integration) newAdapter(
	requestedSession *workerexecution.ProviderSessionMetadata,
) *Adapter {
	result := NewAdapter(AdapterDependencies{
		OperatingSystem: i.dependencies.OperatingSystem,
		TemporaryDir:    i.dependencies.TemporaryDir,
		TemporaryFiles:  i.dependencies.TemporaryFiles,
	})
	result.requestedSession = cloneCursorProviderSession(requestedSession)
	return result
}

type cursorPublication struct {
	mu     sync.Mutex
	ctx    context.Context
	writer inference.ResponseWriter
	first  error
}

func (p *cursorPublication) write(draft workerexecution.Draft) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.first != nil {
		return
	}
	event, err := cursorInferenceEvent(draft)
	if err == nil {
		err = p.writer.WriteEvent(p.ctx, event)
	}
	if err != nil {
		p.first = err
	}
}

func (p *cursorPublication) writeEvent(event inference.EventDraft) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.first != nil {
		return p.first
	}
	if err := p.writer.WriteEvent(p.ctx, event); err != nil {
		p.first = err
	}
	return p.first
}

func (p *cursorPublication) err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.first
}

func cursorRunEvent(runID string, phase workerexecution.Phase) (inference.EventDraft, error) {
	payload, err := json.Marshal(workerexecution.RunPayload{Status: string(phase)})
	if err != nil {
		return inference.EventDraft{}, fmt.Errorf("marshal Cursor run payload: %w", err)
	}
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID: runID, Kind: workerexecution.KindRun, Phase: phase, Payload: payload,
		Provenance: workerexecution.Provenance{
			Provider: "cursor", Delivery: workerexecution.DeliverySynthesized,
			Representation:  workerexecution.RepresentationNotification,
			Fidelity:        workerexecution.FidelityLifecycleOnly,
			NativeEventType: "command_lifecycle",
		},
	})
}

func cursorInferenceEvent(draft workerexecution.Draft) (inference.EventDraft, error) {
	return inference.NewEventDraft(inference.EventDraftInput{
		RunID:              draft.RunID,
		Kind:               draft.Kind,
		Phase:              draft.Phase,
		Provenance:         draft.Provenance,
		Payload:            draft.Payload,
		TurnID:             draft.TurnID,
		ItemID:             draft.ItemID,
		ParentItemID:       draft.ParentItemID,
		ProviderSessionRef: draft.ProviderSessionRef,
	})
}

func cursorInferenceResponse(response workerexecution.InferenceResponse) inference.Response {
	var metadata map[string]string
	if response.Diagnostics != nil && response.Diagnostics.Provider != nil {
		metadata = response.Diagnostics.Provider.ResponseMetadata
	}
	return inference.NewResponse(inference.ResponseInput{
		Content:         response.Content,
		ProviderSession: cursorInferenceSession(response.ProviderSession),
		Metadata:        metadata,
	})
}

func cursorInferenceSession(session *workerexecution.ProviderSessionMetadata) *inference.ProviderSession {
	if session == nil {
		return nil
	}
	result := inference.NewProviderSession(session.Provider, session.Kind, session.ID, nil)
	return &result
}

func cursorInferenceFailure(facts adapter.FailureFacts) inference.Failure {
	return inference.NewFailure(inference.FailureInput{
		Kind:            cursorInferenceFailureKind(facts.Type),
		Message:         facts.Message,
		Retryable:       facts.Retry.Retryable,
		ProviderSession: cursorInferenceSession(facts.ProviderSession),
	})
}

func cursorInferenceFailureKind(failureType workerexecution.WorkFailureType) inference.FailureKind {
	switch failureType {
	case workerexecution.WorkFailureTypeTimeout:
		return inference.FailureTimeout
	case workerexecution.WorkFailureTypeThrottled:
		return inference.FailureThrottled
	case workerexecution.WorkFailureTypeAuthFailure:
		return inference.FailureAuthentication
	case workerexecution.WorkFailureTypePermanentBadRequest:
		return inference.FailureInvalidRequest
	case workerexecution.WorkFailureTypeMisconfigured:
		return inference.FailureDependency
	default:
		return inference.FailureUnknown
	}
}

func cursorRequestFromInvocation(request inference.InvocationRequest) workerexecution.ProviderInferenceRequest {
	providerRequest := request.Execution()
	if providerRequest.Dispatch.DispatchID == "" {
		providerRequest.Dispatch.DispatchID = request.InvocationID()
	}
	providerRequest.ModelProvider = string(modelprovider.ProviderCursor)
	providerRequest.Model = request.Model()
	providerRequest.SystemPrompt = request.SystemPrompt()
	providerRequest.UserMessage = request.UserMessage()
	providerRequest.OutputSchema = request.OutputSchema()
	if session := cursorRequestedSession(request); session != nil {
		providerRequest.SessionID = session.ID
	}
	return providerRequest
}

func cursorRequestedSession(
	request inference.InvocationRequest,
) *workerexecution.ProviderSessionMetadata {
	if session := request.ProviderSession(); session != nil &&
		workerexecution.CanonicalProviderSessionProvider(session.Provider()) == "cursor" &&
		strings.TrimSpace(session.Kind()) == ProviderSessionKindSessionID {
		return canonicalProviderSession("cursor", session.ID())
	}
	return canonicalProviderSession("cursor", request.Execution().SessionID)
}

type cursorStreamingRunner struct {
	runner workerprocess.CommandRunner
}

func (r cursorStreamingRunner) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
	observe func(adapter.Observation) error,
) (workerprocess.CommandResult, error) {
	if r.runner == nil {
		return workerprocess.CommandResult{}, errors.New("Cursor command runner is required")
	}
	if streaming, ok := r.runner.(interface {
		RunStreaming(context.Context, workerprocess.CommandRequest, workerprocess.OutputChunkObserver) (workerprocess.CommandResult, error)
	}); ok {
		var observeMu sync.Mutex
		var observeErr error
		result, runErr := streaming.RunStreaming(ctx, request, func(stream string, chunk []byte) {
			observeMu.Lock()
			defer observeMu.Unlock()
			if observeErr == nil {
				observeErr = observe(adapter.Observation{Stream: adapter.OutputStream(stream), Chunk: chunk})
			}
		})
		return result, errors.Join(runErr, observeErr)
	}
	result, runErr := r.runner.Run(ctx, request)
	if len(result.Stdout) > 0 {
		runErr = errors.Join(runErr, observe(adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: result.Stdout}))
	}
	if len(result.Stderr) > 0 {
		runErr = errors.Join(runErr, observe(adapter.Observation{Stream: adapter.OutputStreamStderr, Chunk: result.Stderr}))
	}
	return result, runErr
}

func (a *Adapter) NewDecoder(_ context.Context, input adapter.DecoderContext) (adapter.Decoder, error) {
	return newResponseEventDecoderWithSession(input, a.requestedSession), nil
}

func (a *Adapter) ParseFinal(_ context.Context, input adapter.FinalParseContext) (adapter.FinalParseResult, error) {
	parsed, failure := parseInferenceResult(
		string(modelprovider.ProviderCursor),
		input.CommandResult.Stdout,
		a.requestedSession,
	)
	if failure != nil {
		return adapter.FinalParseResult{}, failure
	}
	return adapter.FinalParseResult{Response: workerexecution.InferenceResponse{
		Content:         parsed.Content,
		ProviderSession: parsed.ProviderSession,
		Diagnostics:     WithResponseMetadata(nil, parsed.ResponseMetadata),
	}}, nil
}

func (*Adapter) Capabilities(context.Context, adapter.CapabilityContext) (adapter.CapabilityResult, error) {
	return adapter.CapabilityResult{Capabilities: adapter.Capabilities{
		NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true,
		ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
	}}, nil
}

func (a *Adapter) ClassifyFailure(_ context.Context, input adapter.FailureContext) adapter.FailureResult {
	if input.CommandError == nil && input.CommandResult.ExitCode == 0 &&
		input.DecodeError == nil && input.FlushError == nil && input.ParseError == nil {
		return adapter.FailureResult{}
	}
	failure := ParseProviderFailure(FailureInput{
		Stdout: input.CommandResult.Stdout, Stderr: input.CommandResult.Stderr,
		ExitCode: input.CommandResult.ExitCode, FallbackReason: workerexecution.WorkFailureTypeUnknown,
	})
	if failure.ProviderSession == nil {
		failure.ProviderSession = latestCursorProviderSession(
			string(modelprovider.ProviderCursor),
			input.CommandResult.Stdout,
			a.requestedSession,
		)
	}
	family := workerexecution.WorkFailureFamilyTerminal
	retryable := cursorFailureRetryable(failure.Reason)
	if failure.Reason == workerexecution.WorkFailureTypeThrottled {
		family = workerexecution.WorkFailureFamilyThrottle
	} else if retryable {
		family = workerexecution.WorkFailureFamilyRetryable
	}
	return adapter.FailureResult{Failure: &adapter.FailureFacts{
		Family: family, Type: failure.Reason, Message: failure.Message,
		Retry:           adapter.RetryGuidance{Retryable: retryable},
		ProviderSession: failure.ProviderSession,
	}}
}

func latestCursorProviderSession(
	provider string,
	stdout []byte,
	requestedSession *workerexecution.ProviderSessionMetadata,
) *workerexecution.ProviderSessionMetadata {
	session := cloneCursorProviderSession(requestedSession)
	for _, line := range splitNonEmptyLines(stdout) {
		if observed := cursorProviderSessionFromStructuredLine(provider, line); observed != nil {
			session = observed
		}
	}
	return session
}

var _ adapter.Adapter = (*Adapter)(nil)
var _ adapter.StreamingCommandRunner = cursorStreamingRunner{}
var _ inference.Integration = (*Integration)(nil)
