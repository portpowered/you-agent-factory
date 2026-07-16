package factorysession

import (
	"encoding/json"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ResultRequestFromAPI maps one public durable result read request into the shared
// service contract.
func ResultRequestFromAPI(params factoryapi.GetFactorySessionResultsParams) (factorysessionexecution.ResultRequest, error) {
	req := factorysessionexecution.ResultRequest{}
	if params.Mode != nil {
		req.Mode = factorysessionexecution.ResultMode(*params.Mode)
	}
	if params.IncludeArtifacts != nil {
		req.IncludeArtifacts = *params.IncludeArtifacts
	}
	return factorysessionexecution.NormalizeResultRequest(req)
}

// EventReconnectRequestFromAPI maps one public event reconnect request into the
// shared service contract.
func EventReconnectRequestFromAPI(params factoryapi.GetEventsBySessionIdParams) (factorysessionexecution.EventReconnectRequest, error) {
	req := factorysessionexecution.EventReconnectRequest{}
	if params.AfterEventId != nil {
		req.AfterEventID = string(*params.AfterEventId)
	}
	if params.AfterSequence != nil {
		sequence := int(*params.AfterSequence)
		req.AfterSequence = &sequence
	}
	return factorysessionexecution.NormalizeEventReconnectRequest(req)
}

// ResultResponseToAPI maps one durable result projection to the public response shape.
func ResultResponseToAPI(result factorysessionexecution.ResultReadResult) factoryapi.FactorySessionResult {
	response := factoryapi.FactorySessionResult{
		SessionId:    result.SessionID,
		ResultStatus: factoryapi.FactorySessionResultStatus(result.ResultStatus),
	}
	if result.SessionStatus != "" {
		status := factoryapi.FactorySessionDurableLifecycleStatus(result.SessionStatus)
		response.SessionStatus = &status
	}
	if result.Mode != "" {
		mode := factoryapi.FactorySessionResultMode(result.Mode)
		response.Mode = &mode
	}
	if result.IncludeArtifacts {
		include := true
		response.IncludeArtifacts = &include
	}
	if len(result.PrimaryResult) > 0 {
		var content factoryapi.WorkContent
		if err := json.Unmarshal(result.PrimaryResult, &content); err == nil && len(content) > 0 {
			response.PrimaryResult = &content
		}
	}
	if ids := stringSlicePtr(result.ArtifactIDs); ids != nil {
		response.ArtifactIds = ids
	}
	if refs := artifactRefsToAPI(result.ArtifactRefs); refs != nil {
		response.ArtifactRefs = refs
	}
	if failure := failureSummaryToAPI(result.Failure); failure != nil {
		response.FailureDetail = failure
		if result.Failure.PartialResultAvailable {
			value := true
			response.PartialResultAvailable = &value
		}
	}
	if availability := resultAvailabilityToAPI(result.Availability); availability != nil {
		response.Availability = availability
	}
	return response
}

// ListDispatchesResponseToAPI maps one durable dispatch list projection to the public response shape.
func ListDispatchesResponseToAPI(result factorysessionexecution.ListDispatchesResult) factoryapi.ListFactorySessionDispatchesResponse {
	response := factoryapi.ListFactorySessionDispatchesResponse{
		SessionId:  result.SessionID,
		Dispatches: []factoryapi.FactorySessionDispatchSummary{},
	}
	if len(result.Dispatches) > 0 {
		dispatches := make([]factoryapi.FactorySessionDispatchSummary, 0, len(result.Dispatches))
		for _, dispatch := range result.Dispatches {
			dispatches = append(dispatches, dispatchSummaryToAPI(dispatch))
		}
		response.Dispatches = dispatches
	}
	return response
}

// DispatchDetailResponseToAPI maps one durable dispatch projection to the public response shape.
func DispatchDetailResponseToAPI(result factorysessionexecution.DispatchDetail) factoryapi.FactoryDispatch {
	response := factoryapi.FactoryDispatch{
		Id:               result.ID,
		SessionId:        result.SessionID,
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(result.OrchestratorKind)),
		DispatchKind:     factoryapi.FactoryDispatchKind(result.DispatchKind),
		Status:           factoryapi.FactoryDispatchStatus(result.Status),
	}
	if phase := strings.TrimSpace(result.Phase); phase != "" {
		response.Phase = &phase
	}
	if label := strings.TrimSpace(result.Label); label != "" {
		response.Label = &label
	}
	if result.Attempt > 0 {
		attempt := int32(result.Attempt)
		response.Attempt = &attempt
	}
	response.Retryable, response.FailureClassification = dispatchRetryDiagnosticsToAPI(
		result.Retryable,
		result.FailureClassification,
	)
	if runnerID := strings.TrimSpace(result.RunnerID); runnerID != "" {
		response.RunnerId = &runnerID
	}
	selection := resolvedWorkerSelectionToAPI(result.DispatchSummary)
	response.PresetId, response.ModelProvider = selection.presetID, selection.modelProvider
	response.Model, response.ReasoningEffort = selection.model, selection.reasoningEffort
	if provider := strings.TrimSpace(result.Provider); provider != "" {
		response.Provider = &provider
	}
	if refs := providerSessionRefsToAPI(result.ProviderSessionRefs); refs != nil {
		response.ProviderSessionRefs = refs
	}
	if ids := stringSlicePtr(result.ArtifactIDs); ids != nil {
		response.ArtifactIds = ids
	}
	if transitions := dispatchStatusTransitionsToAPI(result.StatusTransitions); transitions != nil {
		response.StatusTransitions = transitions
	}
	if usage := dispatchUsageToAPI(result.Usage); usage != nil {
		response.Usage = usage
	}
	if warnings := dispatchWarningsToAPI(result.Warnings); warnings != nil {
		response.Warnings = warnings
	}
	if failure := dispatchFailureToAPI(result.FailureDetail); failure != nil {
		response.FailureDetail = failure
	}
	if petri := dispatchPetriToAPI(result.Petri); petri != nil {
		response.Petri = petri
	}
	if javascript := dispatchJavaScriptToAPI(result.JavaScript); javascript != nil {
		response.Javascript = javascript
	}
	return response
}

// ListArtifactsResponseToAPI maps one durable artifact list projection to the public response shape.
func ListArtifactsResponseToAPI(result factorysessionexecution.ListArtifactsResult) factoryapi.ListFactorySessionArtifactsResponse {
	response := factoryapi.ListFactorySessionArtifactsResponse{
		SessionId: result.SessionID,
		Artifacts: []factoryapi.FactorySessionArtifactSummary{},
	}
	if len(result.Artifacts) > 0 {
		artifacts := make([]factoryapi.FactorySessionArtifactSummary, 0, len(result.Artifacts))
		for _, artifact := range result.Artifacts {
			artifacts = append(artifacts, artifactSummaryToAPI(artifact))
		}
		response.Artifacts = artifacts
	}
	return response
}

// ArtifactDetailResponseToAPI maps one durable artifact projection to the public response shape.
func ArtifactDetailResponseToAPI(result factorysessionexecution.ArtifactDetail) factoryapi.FactorySessionArtifactDetail {
	response := factoryapi.FactorySessionArtifactDetail{
		SessionId:  result.SessionID,
		Id:         result.ID,
		Kind:       factoryapi.FactoryArtifactKind(result.Kind),
		Visibility: factoryapi.FactoryArtifactVisibility(result.Visibility),
	}
	if label := strings.TrimSpace(result.Label); label != "" {
		response.Label = &label
	}
	if summary := strings.TrimSpace(result.Summary); summary != "" {
		response.Summary = &summary
	}
	if contentHash := strings.TrimSpace(result.ContentHash); contentHash != "" {
		response.ContentHash = &contentHash
	}
	if result.SizeBytes > 0 {
		size := result.SizeBytes
		response.SizeBytes = &size
	}
	if result.CreatedAt != nil {
		response.CreatedAt = result.CreatedAt
	}
	if dispatchID := strings.TrimSpace(result.DispatchID); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if auditMode := strings.TrimSpace(result.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		response.AuditMode = &mode
	}
	if counts := artifactRedactionCountsToAPI(result.RedactionCounts); counts != nil {
		response.RedactionCounts = counts
	}
	if len(result.Content) > 0 {
		var content factoryapi.WorkContent
		if err := json.Unmarshal(result.Content, &content); err == nil && len(content) > 0 {
			response.Content = &content
		}
	}
	if ref := artifactRetrievalRefToAPI(result.ContentRef); ref != nil {
		response.ContentRef = ref
	} else if ref := artifactRetrievalRefToAPI(result.RetrievalRef); ref != nil {
		response.ContentRef = ref
	}
	if metadata := artifactCaptureMetadataToAPI(result.CaptureMetadata); metadata != nil {
		response.CaptureMetadata = metadata
	}
	return response
}

// EventReadResponseToAPI maps replayed durable session events to generated API events.
func EventReadResponseToAPI(result factorysessionexecution.EventReadResult) []factoryapi.FactoryEvent {
	if len(result.Events) == 0 {
		return nil
	}
	events := make([]factoryapi.FactoryEvent, 0, len(result.Events))
	for _, raw := range result.Events {
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	return events
}

// FactoryEventStreamFromReadResult maps one durable event read result into the
// shared reconnect stream shape used by API, CLI, MCP, and UI transports.
func FactoryEventStreamFromReadResult(result factorysessionexecution.EventReadResult) *interfaces.FactoryEventStream {
	closed := make(chan interfaces.FactoryEvent)
	close(closed)
	stream := &interfaces.FactoryEventStream{
		Events: closed,
	}
	events := make([]interfaces.FactoryEvent, 0, len(result.Events))
	for _, raw := range result.Events {
		var event interfaces.FactoryEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			continue
		}
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		events = append(events, event)
	}
	if len(events) == 0 {
		return stream
	}
	stream.History = events
	return stream
}

func dispatchSummaryToAPI(dispatch factorysessionexecution.DispatchSummary) factoryapi.FactorySessionDispatchSummary {
	response := factoryapi.FactorySessionDispatchSummary{
		Id:           dispatch.ID,
		Status:       factoryapi.FactoryDispatchStatus(dispatch.Status),
		DispatchKind: factoryapi.FactoryDispatchKind(dispatch.DispatchKind),
	}
	if phase := strings.TrimSpace(dispatch.Phase); phase != "" {
		response.Phase = &phase
	}
	if label := strings.TrimSpace(dispatch.Label); label != "" {
		response.Label = &label
	}
	if dispatch.Attempt > 0 {
		attempt := int32(dispatch.Attempt)
		response.Attempt = &attempt
	}
	response.Retryable, response.FailureClassification = dispatchRetryDiagnosticsToAPI(
		dispatch.Retryable,
		dispatch.FailureClassification,
	)
	if runnerID := strings.TrimSpace(dispatch.RunnerID); runnerID != "" {
		response.RunnerId = &runnerID
	}
	selection := resolvedWorkerSelectionToAPI(dispatch)
	response.PresetId, response.ModelProvider = selection.presetID, selection.modelProvider
	response.Model, response.ReasoningEffort = selection.model, selection.reasoningEffort
	if provider := strings.TrimSpace(dispatch.Provider); provider != "" {
		response.Provider = &provider
	}
	if refs := providerSessionRefsToAPI(dispatch.ProviderSessionRefs); refs != nil {
		response.ProviderSessionRefs = refs
	}
	if ids := stringSlicePtr(dispatch.OutputArtifactIDs); ids != nil {
		response.OutputArtifactIds = ids
	}
	if usage := dispatchUsageToAPI(dispatch.Usage); usage != nil {
		response.Usage = usage
	}
	if warnings := dispatchWarningsToAPI(dispatch.Warnings); warnings != nil {
		response.Warnings = warnings
	}
	if failure := dispatchFailureToAPI(dispatch.FailureDetail); failure != nil {
		response.FailureDetail = failure
	}
	if javascript := dispatchJavaScriptToAPI(dispatch.JavaScript); javascript != nil {
		response.Javascript = javascript
	}
	return response
}

type resolvedWorkerSelectionAPI struct {
	presetID, modelProvider, model, reasoningEffort *string
}

func resolvedWorkerSelectionToAPI(dispatch factorysessionexecution.DispatchSummary) resolvedWorkerSelectionAPI {
	return resolvedWorkerSelectionAPI{
		presetID:        optionalTrimmedString(dispatch.PresetID),
		modelProvider:   optionalTrimmedString(dispatch.ModelProvider),
		model:           optionalTrimmedString(dispatch.Model),
		reasoningEffort: optionalTrimmedString(dispatch.ReasoningEffort),
	}
}

func optionalTrimmedString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func artifactSummaryToAPI(artifact factorysessionexecution.ArtifactSummary) factoryapi.FactorySessionArtifactSummary {
	response := factoryapi.FactorySessionArtifactSummary{
		Id:         artifact.ID,
		Kind:       factoryapi.FactoryArtifactKind(artifact.Kind),
		Visibility: factoryapi.FactoryArtifactVisibility(artifact.Visibility),
	}
	if label := strings.TrimSpace(artifact.Label); label != "" {
		response.Label = &label
	}
	if contentHash := strings.TrimSpace(artifact.ContentHash); contentHash != "" {
		response.ContentHash = &contentHash
	}
	if artifact.SizeBytes > 0 {
		size := artifact.SizeBytes
		response.SizeBytes = &size
	}
	if artifact.CreatedAt != nil {
		response.CreatedAt = artifact.CreatedAt
	}
	if dispatchID := strings.TrimSpace(artifact.DispatchID); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if auditMode := strings.TrimSpace(artifact.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		response.AuditMode = &mode
	}
	if counts := artifactRedactionCountsToAPI(artifact.RedactionCounts); counts != nil {
		response.RedactionCounts = counts
	}
	if ref := artifactRetrievalRefToAPI(artifact.RetrievalRef); ref != nil {
		response.RetrievalRef = ref
	}
	return response
}

func resultAvailabilityToAPI(availability *factorysessionexecution.ResultAvailabilityDetail) *factoryapi.FactorySessionResultAvailabilityDetail {
	if availability == nil {
		return nil
	}
	out := &factoryapi.FactorySessionResultAvailabilityDetail{}
	if reason := strings.TrimSpace(availability.Reason); reason != "" {
		out.Reason = &reason
	}
	if message := strings.TrimSpace(availability.Message); message != "" {
		out.Message = &message
	}
	if availability.Retryable {
		retryable := true
		out.Retryable = &retryable
	}
	if out.Reason == nil && out.Message == nil && out.Retryable == nil {
		return nil
	}
	return out
}

func dispatchUsageToAPI(usage *factorysessionexecution.DispatchUsage) *factoryapi.FactoryDispatchUsage {
	if usage == nil {
		return nil
	}
	out := &factoryapi.FactoryDispatchUsage{}
	if usage.InputTokens > 0 {
		value := usage.InputTokens
		out.InputTokens = &value
	}
	if usage.OutputTokens > 0 {
		value := usage.OutputTokens
		out.OutputTokens = &value
	}
	if usage.TotalTokens > 0 {
		value := usage.TotalTokens
		out.TotalTokens = &value
	}
	if usage.DurationMillis > 0 {
		value := usage.DurationMillis
		out.DurationMillis = &value
	}
	if usage.CostUSD > 0 {
		value := usage.CostUSD
		out.CostUsd = &value
	}
	if usage.RetryCount > 0 {
		value := usage.RetryCount
		out.RetryCount = &value
	}
	if out.InputTokens == nil && out.OutputTokens == nil && out.TotalTokens == nil &&
		out.DurationMillis == nil && out.CostUsd == nil && out.RetryCount == nil {
		return nil
	}
	return out
}

func dispatchStatusTransitionsToAPI(
	transitions []factorysessionexecution.DispatchStatus,
) *[]factoryapi.FactoryDispatchStatus {
	if len(transitions) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryDispatchStatus, 0, len(transitions))
	for _, transition := range transitions {
		status := strings.TrimSpace(string(transition))
		if status == "" {
			continue
		}
		out = append(out, factoryapi.FactoryDispatchStatus(status))
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func dispatchWarningsToAPI(warnings []factorysessionexecution.DispatchWarning) *[]factoryapi.FactoryDispatchWarning {
	if len(warnings) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryDispatchWarning, 0, len(warnings))
	for _, warning := range warnings {
		row := factoryapi.FactoryDispatchWarning{
			Code: warning.Code,
		}
		row.Message = strings.TrimSpace(warning.Message)
		out = append(out, row)
	}
	return &out
}

func dispatchFailureToAPI(failure *factorysessionexecution.DispatchFailureDetail) *factoryapi.FailureDetail {
	if failure == nil {
		return nil
	}
	reason := strings.TrimSpace(failure.Reason)
	message := strings.TrimSpace(failure.Message)
	if reason == "" || message == "" {
		return nil
	}
	return &factoryapi.FailureDetail{Reason: failureReasonToAPI(reason), Message: message}
}

func dispatchRetryDiagnosticsToAPI(retryable *bool, classification string) (*bool, *factoryapi.WorkFailureType) {
	var retryableValue *bool
	if retryable != nil {
		value := *retryable
		retryableValue = &value
	}
	classification = strings.TrimSpace(classification)
	if classification == "" {
		return retryableValue, nil
	}
	value := factoryapi.WorkFailureType(classification)
	return retryableValue, &value
}

func dispatchPetriToAPI(petri *factorysessionexecution.DispatchPetriProjection) *factoryapi.FactoryDispatchPetriProjection {
	if petri == nil {
		return nil
	}
	out := factoryapi.FactoryDispatchPetriProjection{
		TransitionId: petri.TransitionID,
	}
	if workstation := strings.TrimSpace(petri.WorkstationName); workstation != "" {
		out.WorkstationName = &workstation
	}
	if workerType := strings.TrimSpace(petri.WorkerType); workerType != "" {
		out.WorkerType = &workerType
	}
	return &out
}

func dispatchJavaScriptToAPI(javascript *factorysessionexecution.DispatchJavaScriptProjection) *factoryapi.FactoryDispatchJavaScriptProjection {
	if javascript == nil {
		return nil
	}
	out := factoryapi.FactoryDispatchJavaScriptProjection{
		TaskKind: factoryapi.FactoryDispatchJavaScriptTaskKind(javascript.TaskKind),
	}
	if label := strings.TrimSpace(javascript.TaskLabel); label != "" {
		out.TaskLabel = &label
	}
	if mode := strings.TrimSpace(javascript.ExecutionMode); mode != "" {
		out.ExecutionMode = &mode
	}
	return &out
}

func artifactRetrievalRefToAPI(ref *factorysessionexecution.ArtifactRetrievalRef) *factoryapi.FactorySessionArtifactRetrievalRef {
	if ref == nil || strings.TrimSpace(ref.Href) == "" {
		return nil
	}
	out := factoryapi.FactorySessionArtifactRetrievalRef{
		Href: ref.Href,
	}
	if method := strings.TrimSpace(ref.Method); method != "" {
		methodValue := factoryapi.FactorySessionArtifactRetrievalRefMethod(method)
		out.Method = &methodValue
	}
	return &out
}

func artifactCaptureMetadataToAPI(metadata map[string]any) *factoryapi.FactoryArtifactCaptureMetadata {
	if len(metadata) == 0 {
		return nil
	}
	projected := &factoryapi.FactoryArtifactCaptureMetadata{}
	hasValue := false
	if raw, ok := metadata["capturedAt"]; ok {
		switch value := raw.(type) {
		case time.Time:
			capturedAt := value.UTC()
			projected.CapturedAt = &capturedAt
			hasValue = true
		case string:
			if capturedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
				capturedAt = capturedAt.UTC()
				projected.CapturedAt = &capturedAt
				hasValue = true
			}
		}
	}
	if raw, ok := metadata["sourceDispatchId"]; ok {
		if dispatchID := strings.TrimSpace(stringValueFromAny(raw)); dispatchID != "" {
			projected.SourceDispatchId = &dispatchID
			hasValue = true
		}
	}
	if raw, ok := metadata["mimeType"]; ok {
		if mimeType := strings.TrimSpace(stringValueFromAny(raw)); mimeType != "" {
			projected.MimeType = &mimeType
			hasValue = true
		}
	}
	if !hasValue {
		return nil
	}
	return projected
}

func stringValueFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return ""
	}
}

func artifactRedactionCountsToAPI(counts *factorysessionexecution.ArtifactRedactionCounts) *factoryapi.FactoryArtifactRedactionCounts {
	if counts == nil {
		return nil
	}
	out := &factoryapi.FactoryArtifactRedactionCounts{}
	if counts.Paths > 0 {
		value := counts.Paths
		out.Paths = &value
	}
	if counts.Secrets > 0 {
		value := counts.Secrets
		out.Secrets = &value
	}
	if counts.Tokens > 0 {
		value := counts.Tokens
		out.Tokens = &value
	}
	if out.Paths == nil && out.Secrets == nil && out.Tokens == nil {
		return nil
	}
	return out
}

func stringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}
