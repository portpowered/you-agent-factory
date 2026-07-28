package wire

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

// acpProviderIntegration is the composition adapter from the existing Workers
// invocation protocol into the singular Providers execution service.
type acpProviderIntegration struct {
	id        providers.ID
	providers providers.Service
}

func (i *acpProviderIntegration) Identity() inference.Identity { return inference.Identity(i.id) }
func (*acpProviderIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(inference.CapabilityPromptSubmission, inference.CapabilityImageInput, inference.CapabilitySessionResume, inference.CapabilityNativeStreaming, inference.CapabilityMessageDeltas, inference.CapabilityReasoningSummaries, inference.CapabilityToolLifecycle, inference.CapabilityFileChanges, inference.CapabilityPlans, inference.CapabilityUsage, inference.CapabilityStableItemIDs)
}
func (*acpProviderIntegration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(inference.ReadinessReady), nil
}
func (i *acpProviderIntegration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

func (i *acpProviderIntegration) Invoke(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
	execution := request.Execution()
	inputTokens := append([]any(nil), execution.InputTokens...)
	inputTokens = append(inputTokens, execution.Dispatch.InputTokens...)
	result, err := i.providers.Execute(ctx, providers.ExecuteRequest{Provider: i.id, AttemptID: request.InvocationID(), Model: request.Model(), SkipPermissions: execution.SkipPermissions, SystemPrompt: request.SystemPrompt(), UserMessage: request.UserMessage(), InputTokens: inputTokens, OutputSchema: request.OutputSchema(), WorkingDirectory: execution.WorkingDirectory, Worktree: execution.Worktree, ProcessEnvironment: execution.ProcessEnvironment, EnvVars: execution.EnvVars, WorkerType: execution.WorkerType, WorkstationName: execution.WorkstationType})
	sessionRef := ""
	if result.SessionRef != nil {
		sessionRef = result.SessionRef.ID
	}
	if writeErr := writeACPRunEvent(ctx, writer, request.InvocationID(), string(i.id), sessionRef, workers.PhaseStarted); writeErr != nil {
		return writeErr
	}
	if err != nil {
		var failure providers.ExecuteFailure
		errors.As(err, &failure)
		lifecycles := map[string]inference.EventDraft{}
		if failure.Diagnostics != nil {
			for _, progress := range failure.Diagnostics.Progress {
				if draft, ok := acpProgressDraft(request.InvocationID(), string(i.id), sessionRef, progress); ok {
					if writeErr := writeACPValidatedDraft(ctx, writer, draft, lifecycles); writeErr != nil {
						return writeErr
					}
				}
			}
		}
		if writeErr := closeACPLifecycles(ctx, writer, lifecycles); writeErr != nil {
			return writeErr
		}
		if failure.Diagnostics != nil {
			if partial := strings.TrimSpace(failure.Diagnostics.Metadata["partial_content"]); partial != "" {
				if draft, ok := acpPartialMessageDraft(request.InvocationID(), string(i.id), sessionRef, partial); ok {
					if writeErr := writeACPValidatedDraft(ctx, writer, draft, lifecycles); writeErr != nil {
						return writeErr
					}
				}
			}
		}
		if writeErr := writeACPErrorEvent(ctx, writer, request.InvocationID(), string(i.id), sessionRef, failure.Message); writeErr != nil {
			return writeErr
		}
		kind := inference.FailureUnknown
		switch failure.Kind {
		case providers.ExecuteFailureKindAuthentication:
			kind = inference.FailureAuthentication
		case providers.ExecuteFailureKindInvalidRequest:
			kind = inference.FailureInvalidRequest
		case providers.ExecuteFailureKindMisconfigured:
			kind = inference.FailureDependency
		case providers.ExecuteFailureKindDependency:
			kind = inference.FailureDependency
		case providers.ExecuteFailureKindCanceled:
			kind = inference.FailureCanceled
		case providers.ExecuteFailureKindTimeout:
			kind = inference.FailureTimeout
		}
		diagnostics := map[string]string{}
		if failure.Kind == providers.ExecuteFailureKindMisconfigured {
			diagnostics["work-failure-type"] = string(workers.WorkFailureTypeMisconfigured)
		}
		if failure.Kind == providers.ExecuteFailureKindDependency && strings.Contains(strings.ToLower(failure.Message), "executable") {
			diagnostics["work-failure-type"] = string(workers.WorkFailureTypeMissingExecutable)
		}
		if writeErr := writeACPRunEvent(ctx, writer, request.InvocationID(), string(i.id), sessionRef, workers.PhaseFailed); writeErr != nil {
			return writeErr
		}
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{Kind: kind, Message: failure.Message, Diagnostics: diagnostics})))
	}
	lifecycles := map[string]inference.EventDraft{}
	if result.Diagnostics != nil {
		for _, progress := range result.Diagnostics.Progress {
			if draft, ok := acpProgressDraft(request.InvocationID(), string(i.id), sessionRef, progress); ok {
				if err := writeACPValidatedDraft(ctx, writer, draft, lifecycles); err != nil {
					return err
				}
			}
		}
	}
	if err := closeACPLifecycles(ctx, writer, lifecycles); err != nil {
		return err
	}
	if draft, ok := acpCompletedMessageDraft(request.InvocationID(), string(i.id), sessionRef, result.Content); ok {
		if err := writeACPValidatedDraft(ctx, writer, draft, lifecycles); err != nil {
			return err
		}
	}
	if err := writeACPRunEvent(ctx, writer, request.InvocationID(), string(i.id), sessionRef, workers.PhaseCompleted); err != nil {
		return err
	}
	session := acpInferenceSession(result.SessionRef)
	return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{Content: result.Content, ProviderSession: session})))
}

func acpInferenceSession(ref *providers.SessionRef) *inference.ProviderSession {
	if ref == nil {
		return nil
	}
	session := inference.NewProviderSession(ref.Provider.String(), ref.Kind, ref.ID, nil)
	return &session
}

func acpProgressDraft(runID, provider, sessionRef string, p providers.ExecuteProgress) (inference.EventDraft, bool) {
	kind := p.Metadata["kind"]
	item := p.Metadata["item_id"]
	var eventKind workers.Kind
	phase := workers.PhaseUpdated
	representation := workers.RepresentationNotification
	fidelity := workers.FidelityNormalized
	var payload any
	switch kind {
	case "message":
		eventKind = workers.KindMessage
		switch p.Phase {
		case "started":
			phase = workers.PhaseStarted
			fidelity = workers.FidelityLifecycleOnly
			payload = workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText}}}
		case "completed":
			phase = workers.PhaseCompleted
			representation = workers.RepresentationSnapshot
			payload = workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: p.Detail}}}
		default:
			phase = workers.PhaseDelta
			representation = workers.RepresentationDelta
			payload = workers.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: p.Detail}
		}
	case "reasoning":
		eventKind = workers.KindReasoning
		if p.Phase == "started" {
			phase = workers.PhaseStarted
			fidelity = workers.FidelityLifecycleOnly
			payload = workers.ReasoningPayload{}
		} else if p.Phase == "completed" {
			phase = workers.PhaseCompleted
			representation = workers.RepresentationSnapshot
			payload = workers.ReasoningPayload{Summary: p.Detail}
		} else {
			phase = workers.PhaseDelta
			representation = workers.RepresentationDelta
			payload = workers.ReasoningPayload{SummaryDelta: p.Detail}
		}
	case "tool":
		eventKind = workers.KindTool
		if p.Phase == "started" {
			phase = workers.PhaseStarted
		} else if p.Metadata["status"] == "completed" {
			phase = workers.PhaseCompleted
		}
		tool := workers.ToolPayload{ToolCallID: item, ToolName: p.Detail, Status: p.Metadata["status"]}
		if raw := p.Metadata["raw_input"]; json.Valid([]byte(raw)) {
			tool.ArgumentsSummary = json.RawMessage(raw)
		}
		if raw := p.Metadata["raw_output"]; json.Valid([]byte(raw)) {
			tool.ResultSummary = json.RawMessage(raw)
		}
		payload = tool
	case "plan":
		eventKind = workers.KindPlan
		var entries []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		}
		_ = json.Unmarshal([]byte(p.Metadata["entries"]), &entries)
		steps := make([]workers.PlanStep, 0, len(entries))
		for index, entry := range entries {
			steps = append(steps, workers.PlanStep{ID: "step-" + strconv.Itoa(index+1), Description: entry.Content, Status: entry.Status})
		}
		payload = workers.PlanPayload{Steps: steps}
	case "usage":
		eventKind = workers.KindUsage
		used, _ := strconv.ParseInt(p.Metadata["used_tokens"], 10, 64)
		payload = workers.UsagePayload{TotalTokens: used}
	case "session":
		eventKind = workers.KindSession
		phase = workers.PhaseStarted
		payload = workers.SessionPayload{Status: "started"}
	case "file_change":
		eventKind = workers.KindFileChange
		payload = workers.FileChangePayload{Path: p.Metadata["path"], Operation: p.Metadata["operation"]}
	default:
		return inference.EventDraft{}, false
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return inference.EventDraft{}, false
	}
	draft, err := inference.NewEventDraft(inference.EventDraftInput{RunID: runID, Kind: eventKind, Phase: phase, ItemID: item, ProviderSessionRef: sessionRef, Payload: data, Provenance: workers.Provenance{Provider: provider, Delivery: workers.DeliveryNativeStream, Representation: representation, Fidelity: fidelity, NativeEventType: p.Metadata["native_type"]}})
	return draft, err == nil
}

func writeACPRunEvent(ctx context.Context, writer inference.ResponseWriter, runID, provider, sessionRef string, phase workers.Phase) error {
	payload, _ := json.Marshal(workers.RunPayload{Status: strings.ToLower(string(phase))})
	draft, err := inference.NewEventDraft(inference.EventDraftInput{RunID: runID, Kind: workers.KindRun, Phase: phase, ProviderSessionRef: sessionRef, Payload: payload, Provenance: workers.Provenance{Provider: provider, Delivery: workers.DeliverySynthesized, Representation: workers.RepresentationNotification, Fidelity: workers.FidelityLifecycleOnly, NativeEventType: "session/prompt"}})
	if err != nil {
		return err
	}
	return writer.WriteEvent(ctx, draft)
}

func writeACPValidatedDraft(ctx context.Context, writer inference.ResponseWriter, draft inference.EventDraft, lifecycles map[string]inference.EventDraft) error {
	d := draft.Draft()
	key := string(d.Kind) + ":" + d.ItemID
	if d.Kind == workers.KindSession {
		key = string(d.Kind) + ":" + d.ProviderSessionRef
	}
	if (d.Phase == workers.PhaseDelta || d.Phase == workers.PhaseCompleted || d.Phase == workers.PhaseFailed || d.Phase == workers.PhaseCanceled) && lifecycles[key].Draft().Phase == "" {
		started, err := cloneACPPhase(draft, workers.PhaseStarted, workers.FidelityLifecycleOnly, workers.RepresentationNotification, "acp/synthetic_start")
		if err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, started); err != nil {
			return err
		}
		lifecycles[key] = started
	}
	if err := writer.WriteEvent(ctx, draft); err != nil {
		return err
	}
	if d.Phase == workers.PhaseStarted {
		lifecycles[key] = draft
	}
	if d.Phase == workers.PhaseCompleted || d.Phase == workers.PhaseFailed || d.Phase == workers.PhaseCanceled {
		delete(lifecycles, key)
	}
	return nil
}

func closeACPLifecycles(ctx context.Context, writer inference.ResponseWriter, lifecycles map[string]inference.EventDraft) error {
	keys := make([]string, 0, len(lifecycles))
	for key := range lifecycles {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		leftDraft, rightDraft := lifecycles[keys[left]].Draft(), lifecycles[keys[right]].Draft()
		leftOrder, rightOrder := acpLifecycleCompletionOrder(leftDraft.Kind), acpLifecycleCompletionOrder(rightDraft.Kind)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return keys[left] < keys[right]
	})
	for _, key := range keys {
		draft := lifecycles[key]
		if draft.Draft().Kind == workers.KindMessage {
			continue
		}
		completed, err := cloneACPPhase(draft, workers.PhaseCompleted, workers.FidelityLifecycleOnly, workers.RepresentationNotification, "session/prompt")
		if err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, completed); err != nil {
			return err
		}
		delete(lifecycles, key)
	}
	return nil
}

func acpLifecycleCompletionOrder(kind workers.Kind) int {
	switch kind {
	case workers.KindReasoning:
		return 0
	case workers.KindTool:
		return 1
	case workers.KindSession:
		return 2
	default:
		return 3
	}
}

func cloneACPPhase(source inference.EventDraft, phase workers.Phase, fidelity workers.Fidelity, representation workers.Representation, nativeType string) (inference.EventDraft, error) {
	d := source.Draft()
	d.Provenance.Fidelity, d.Provenance.Representation, d.Provenance.NativeEventType = fidelity, representation, nativeType
	if phase == workers.PhaseStarted {
		var payload any
		switch d.Kind {
		case workers.KindMessage:
			payload = workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText}}}
		case workers.KindReasoning:
			payload = workers.ReasoningPayload{}
		}
		if payload != nil {
			d.Payload, _ = json.Marshal(payload)
		}
	}
	return inference.NewEventDraft(inference.EventDraftInput{RunID: d.RunID, Kind: d.Kind, Phase: phase, Provenance: d.Provenance, Payload: d.Payload, TurnID: d.TurnID, ItemID: d.ItemID, ParentItemID: d.ParentItemID, ProviderSessionRef: d.ProviderSessionRef})
}

func acpCompletedMessageDraft(runID, provider, sessionRef, content string) (inference.EventDraft, bool) {
	p := providers.ExecuteProgress{Phase: "completed", Detail: content, Metadata: map[string]string{"kind": "message", "item_id": "assistant-message", "native_type": "session/prompt"}}
	return acpProgressDraft(runID, provider, sessionRef, p)
}

func acpPartialMessageDraft(runID, provider, sessionRef, content string) (inference.EventDraft, bool) {
	payload, err := json.Marshal(workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: content}}, Partial: true})
	if err != nil {
		return inference.EventDraft{}, false
	}
	draft, err := inference.NewEventDraft(inference.EventDraftInput{RunID: runID, Kind: workers.KindMessage, Phase: workers.PhaseCompleted, ItemID: "assistant-message", ProviderSessionRef: sessionRef, Payload: payload, Provenance: workers.Provenance{Provider: provider, Delivery: workers.DeliverySynthesized, Representation: workers.RepresentationSnapshot, Fidelity: workers.FidelityNormalized, NativeEventType: "session/prompt"}})
	return draft, err == nil
}

func writeACPErrorEvent(ctx context.Context, writer inference.ResponseWriter, runID, provider, sessionRef, message string) error {
	payload, _ := json.Marshal(workers.ErrorPayload{Code: "ACP_PROMPT_FAILED", Message: message})
	draft, err := inference.NewEventDraft(inference.EventDraftInput{RunID: runID, Kind: workers.KindError, Phase: workers.PhaseFailed, ItemID: "acp-error", ProviderSessionRef: sessionRef, Payload: payload, Provenance: workers.Provenance{Provider: provider, Delivery: workers.DeliverySynthesized, Representation: workers.RepresentationNotification, Fidelity: workers.FidelityNormalized, NativeEventType: "session/prompt"}})
	if err != nil {
		return err
	}
	return writer.WriteEvent(ctx, draft)
}

func acpManifest(id string) providerregistry.Manifest {
	return providerregistry.Manifest{ID: id, Aliases: []string{}, Description: inference.LocalizedValue{Type: "LOCALIZABLE_ASSET", Value: "Agent Client Protocol provider"}, DisplayName: inference.LocalizedValue{Type: "LOCALIZABLE_ASSET", Value: id}, Documentation: []inference.DocumentationLink{{Kind: "homepage", URL: "https://agentclientprotocol.com/"}}, Discovery: inference.DiscoveryPrerequisites{EndpointKinds: []string{"stdio"}}, ImplementationAvailability: inference.ImplementationExternallySupplied, TechnicalSupportLevel: inference.SupportExperimental, MaximumExecutionCapabilities: inference.ExecutionCapabilities{PromptSubmission: true, ImageInput: true, SessionResume: true, WorkingDirectory: true, Worktree: true}, MaximumResponseFidelityCapabilities: inference.ResponseFidelityCapabilities{NativeStreaming: true, MessageDeltas: true, ReasoningSummaries: true, ToolLifecycle: true, FileChanges: true, Plans: true, Usage: true, StableItemIDs: true}}
}

func acpRegistryRegistrations(service providers.Service, ids ...string) []providerregistry.Registration {
	result := make([]providerregistry.Registration, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		integration := &acpProviderIntegration{id: providers.ID(id), providers: service}
		result = append(result, providerregistry.ExternalRegistration(acpManifest(id), integration))
	}
	return result
}

var _ inference.Integration = (*acpProviderIntegration)(nil)
