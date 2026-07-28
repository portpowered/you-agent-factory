package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

const scriptWrapExecutorProvider = "script_wrap"

type providersExecutionRunner struct {
	next      workers.Runner
	providers providers.Service
	publish   workers.ProgressPublisher
}

func (r providersExecutionRunner) Execute(ctx context.Context, request workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
	providerID := providers.ID(strings.ToLower(strings.TrimSpace(request.ExecutorProvider)))
	if r.providers == nil || providerID == "" || providerID == scriptWrapExecutorProvider {
		return r.next.Execute(ctx, request)
	}
	stream, err := r.providers.ExecuteStream(ctx, providers.ExecuteRequest{
		ProviderID: providerID,
		Correlation: providers.Correlation{
			DispatchID:  request.Dispatch.DispatchID,
			Workstation: request.WorkstationType,
		},
		Instructions:     request.SystemPrompt,
		Prompt:           providerPromptParts(request),
		Model:            request.Model,
		WorkingDirectory: effectiveProviderWorkingDirectory(request),
		Environment:      providerEnvironment(request.ProcessEnvironment, request.EnvVars),
		SkipPermissions:  request.SkipPermissions,
	})
	if err != nil {
		return workers.RunnerExecutionResult{}, providersExecutionError(err)
	}
	defer stream.Close()
	for update := range stream.Updates {
		draft, mapErr := providerExecutionDraft(providerID, request.Dispatch.DispatchID, update)
		if mapErr != nil {
			return workers.RunnerExecutionResult{}, providersExecutionError(mapErr)
		}
		if r.publish != nil {
			r.publish(workers.ProgressFragment{DispatchID: request.Dispatch.DispatchID, CanonicalDraft: draft})
		}
	}
	outcome, ok := <-stream.Outcome
	if !ok {
		return workers.RunnerExecutionResult{}, providersExecutionError(errors.New("provider execution stream closed without an outcome"))
	}
	if outcome.Err != nil {
		return workers.RunnerExecutionResult{}, providersExecutionError(outcome.Err)
	}
	response := outcome.Response
	result := workers.RunnerExecutionResult{Content: response.Content}
	if response.Session != nil {
		result.ProviderSession = &workers.ProviderSessionMetadata{
			Provider: string(response.Session.ProviderID),
			Kind:     response.Session.Kind,
			ID:       response.Session.ID,
		}
	}
	if response.Diagnostics != nil {
		result.Diagnostics = &workers.WorkDiagnostics{
			Provider: &workers.ProviderDiagnostic{Provider: string(providerID), Model: request.Model},
			Metadata: cloneProviderMetadata(response.Diagnostics.Metadata),
		}
	}
	return result, nil
}

func providerPromptParts(request workers.RunnerExecutionRequest) []providers.ContentPart {
	parts := []providers.ContentPart{{Kind: providers.ContentKindText, Text: request.UserMessage}}
	seen := map[string]struct{}{}
	for _, raw := range request.InputTokens {
		token, ok := raw.(workers.Token)
		if !ok {
			continue
		}
		for _, content := range token.Color.Content {
			uri := strings.TrimSpace(content.URL)
			if uri == "" && strings.TrimSpace(content.File) != "" {
				path := content.File
				if !filepath.IsAbs(path) && request.WorkingDirectory != "" {
					path = filepath.Join(request.WorkingDirectory, path)
				}
				if absolute, err := filepath.Abs(path); err == nil {
					uri = (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
				}
			}
			if uri == "" {
				continue
			}
			if _, duplicate := seen[uri]; duplicate {
				continue
			}
			seen[uri] = struct{}{}
			name := firstProviderResourceName(content.Label, content.Slot, content.ArtifactID, filepath.Base(content.File), uri)
			parts = append(parts, providers.ContentPart{Kind: providers.ContentKindResourceLink, Name: name, URI: uri, MimeType: content.ContentType})
		}
	}
	return parts
}

func firstProviderResourceName(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "resource"
}

func providerExecutionDraft(providerID providers.ID, dispatchID string, update providers.ExecutionUpdate) (workers.Draft, error) {
	kind, phase, representation, payload, err := providerExecutionPayload(update)
	if err != nil {
		return workers.Draft{}, err
	}
	draft := workers.Draft{
		Kind: kind, Phase: phase, DispatchID: dispatchID, ItemID: update.ItemID,
		ProviderSessionRef: update.ProviderSessionID,
		Provenance: workers.Provenance{
			Provider: string(providerID), NativeEventType: update.NativeType,
			Delivery: workers.DeliveryNativeStream, Representation: representation,
			Fidelity: workers.FidelityNormalized,
		},
		Payload: payload,
	}
	if err := workers.ValidateDraft(draft); err != nil {
		return workers.Draft{}, fmt.Errorf("map provider execution update %d: %w", update.Sequence, err)
	}
	return draft, nil
}

func providerExecutionPayload(update providers.ExecutionUpdate) (workers.Kind, workers.Phase, workers.Representation, json.RawMessage, error) {
	var kind workers.Kind
	var phase workers.Phase
	representation := workers.RepresentationSnapshot
	var value any
	switch update.Kind {
	case providers.ExecutionUpdateMessage:
		kind = workers.KindMessage
		if update.Final {
			phase, representation = workers.PhaseCompleted, workers.RepresentationSnapshot
			value = workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: update.Text}}, Partial: update.Partial}
		} else {
			phase, representation = workers.PhaseDelta, workers.RepresentationDelta
			value = workers.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: update.Text}
		}
	case providers.ExecutionUpdateReasoning:
		kind, phase, representation = workers.KindReasoning, workers.PhaseDelta, workers.RepresentationDelta
		value = workers.ReasoningPayload{SummaryDelta: update.Text}
	case providers.ExecutionUpdateTool:
		kind, phase = workers.KindTool, providerToolPhase(update.Tool)
		tool := update.Tool
		if tool == nil {
			return "", "", "", nil, fmt.Errorf("provider tool update is missing tool data")
		}
		arguments, marshalErr := marshalOptionalProviderValue(tool.RawInput)
		if marshalErr != nil {
			return "", "", "", nil, marshalErr
		}
		result, marshalErr := marshalOptionalProviderValue(tool.RawOutput)
		if marshalErr != nil {
			return "", "", "", nil, marshalErr
		}
		value = workers.ToolPayload{ToolCallID: tool.ID, ToolName: providerToolName(tool), Status: tool.Status, ArgumentsSummary: arguments, ResultSummary: result}
	case providers.ExecutionUpdateFileChange:
		kind, phase = workers.KindFileChange, workers.PhaseUpdated
		if update.FileChange == nil {
			return "", "", "", nil, fmt.Errorf("provider file-change update is missing file data")
		}
		value = workers.FileChangePayload{Path: update.FileChange.Path, Operation: update.FileChange.Operation, Summary: update.FileChange.Summary}
	case providers.ExecutionUpdatePlan:
		kind, phase = workers.KindPlan, workers.PhaseUpdated
		steps := make([]workers.PlanStep, len(update.Plan))
		for index, entry := range update.Plan {
			steps[index] = workers.PlanStep{ID: entry.ID, Description: entry.Description, Status: entry.Status}
		}
		value = workers.PlanPayload{Steps: steps}
	case providers.ExecutionUpdateUsage:
		kind, phase = workers.KindUsage, workers.PhaseUpdated
		if update.Usage == nil {
			return "", "", "", nil, fmt.Errorf("provider usage update is missing usage data")
		}
		value = workers.UsagePayload{TotalTokens: update.Usage.UsedTokens}
	case providers.ExecutionUpdateSession:
		kind, phase = workers.KindSession, workers.PhaseStarted
		value = workers.SessionPayload{Status: "updated"}
	case providers.ExecutionUpdateError:
		kind, phase = workers.KindError, workers.PhaseFailed
		if update.Error == nil {
			return "", "", "", nil, fmt.Errorf("provider error update is missing error data")
		}
		value = workers.ErrorPayload{Code: update.Error.Code, Message: update.Error.Message, Retryable: update.Error.Retryable}
	default:
		return "", "", "", nil, fmt.Errorf("unsupported provider execution update kind %q", update.Kind)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("marshal provider execution update %q: %w", update.Kind, err)
	}
	return kind, phase, representation, payload, nil
}

func providerToolPhase(tool *providers.ToolUpdate) workers.Phase {
	if tool == nil {
		return workers.PhaseStarted
	}
	switch strings.ToLower(strings.TrimSpace(tool.Status)) {
	case "completed":
		return workers.PhaseCompleted
	case "failed":
		return workers.PhaseFailed
	default:
		return workers.PhaseStarted
	}
}

func providerToolName(tool *providers.ToolUpdate) string {
	if name := strings.TrimSpace(tool.Name); name != "" {
		return name
	}
	return "ACP tool"
}

func marshalOptionalProviderValue(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal ACP tool value: %w", err)
	}
	return payload, nil
}

func providersExecutionError(err error) error {
	failure := workers.WorkFailureTypeUnknown
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		failure = workers.WorkFailureTypeTimeout
	case errors.Is(err, providers.ErrAuthenticationRequired):
		failure = workers.WorkFailureTypeAuthFailure
	case errors.Is(err, providers.ErrUnavailableProvider):
		failure = workers.WorkFailureTypeMissingExecutable
	case errors.Is(err, providers.ErrUnknownProvider), errors.Is(err, providers.ErrUnsupportedExecutor), errors.Is(err, providers.ErrIncompatibleProtocol):
		failure = workers.WorkFailureTypeMisconfigured
	case errors.Is(err, providers.ErrInvalidRequest):
		failure = workers.WorkFailureTypePermanentBadRequest
	}
	return workerprovider.NewProviderError(failure, err.Error(), err)
}

func effectiveProviderWorkingDirectory(request workers.RunnerExecutionRequest) string {
	if request.WorkingDirectory != "" {
		return request.WorkingDirectory
	}
	return request.Worktree
}

func providerEnvironment(base []string, overlays map[string]string) []providers.EnvironmentEntry {
	values := make(map[string]string, len(base)+len(overlays))
	for _, entry := range base {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name != "" {
			values[name] = value
		}
	}
	for name, value := range overlays {
		if name != "" {
			values[name] = value
		}
	}
	keys := make([]string, 0, len(values))
	for name := range values {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	result := make([]providers.EnvironmentEntry, 0, len(keys))
	for _, name := range keys {
		result = append(result, providers.EnvironmentEntry{Name: name, Value: values[name]})
	}
	return result
}

func cloneProviderMetadata(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
