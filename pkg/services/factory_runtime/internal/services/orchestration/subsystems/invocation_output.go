package subsystems

import (
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	packagedGoalExecuteWorkstationName = "execute-goal"
	packagedSubagentRunWorkstationName = "run-subagent"
	defaultAudioContentType            = "audio/wav"
)

// The following pure transformations are Runtime-owned application policy.
// They deliberately accept authored values and return marking content; no
// Definitions construction or effect port crosses the Runtime boundary.
func applyPackagedTTSInvocationMetadata(
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	inputColors []factorytoken.Color,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if token == nil || !shouldFormatTTSInvocationMetadata(workstation) || strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	traceID := ""
	if source := firstNonResourceInput(inputColors); source != nil {
		traceID = strings.TrimSpace(source.TraceID)
	}

	backendLabel := ""
	if workstation != nil && runtimeConfig != nil {
		if lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup); ok {
			if worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName)); ok && worker != nil {
				backendLabel = ttsBackendLabelFromWorker(worker)
			}
		}
	}

	metadataContent, err := ttsMetadataContentFromWorkerOutput(workerOutput, traceID, "", backendLabel)
	if err != nil {
		// Packaged TTS metadata is only shaped from successful audio output.
		return nil
	}

	token.Color.Content = metadataContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedGoalInvocationSummary(
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if token == nil || !shouldFormatInvocationSummary(workstation) || strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	summaryContent, err := summaryContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged goal invocation summary: %w", err)
	}

	token.Color.Content = summaryContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedSubagentInvocationResponse(
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if token == nil || !shouldFormatInvocationResponse(workstation) || strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	responseContent, err := responseContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged subagent invocation response: %w", err)
	}

	token.Color.Content = responseContent
	token.Color.Payload = nil
	return nil
}

func shouldFormatInvocationSummary(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstationHasAgentOutputRole(workstation, packagedGoalExecuteWorkstationName)
}

func shouldFormatInvocationResponse(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstationHasAgentOutputRole(workstation, packagedSubagentRunWorkstationName)
}

func shouldFormatTTSInvocationMetadata(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	return strings.TrimSpace(workstation.Name) == factorydefinitions.PackagedTTSInvokeWorkstationName &&
		factorydefinitions.IsInferenceRunWorkstationType(workstation.Type) &&
		strings.EqualFold(strings.TrimSpace(workstation.Operation), "TTS")
}

func ttsBackendLabelFromWorker(worker *factorydefinitions.FactoryWorkerConfig) string {
	model := factorydefinitions.DefaultTTSModelName
	backend := factorydefinitions.DefaultTTSBackendName
	if worker != nil {
		if trimmed := strings.TrimSpace(worker.Model); trimmed != "" {
			model = trimmed
		}
		if trimmed := strings.TrimSpace(worker.Command); trimmed != "" && trimmed != "omnivoice-llamacpp" {
			backend = trimmed
		}
	}
	return model + "/" + backend
}

func ttsMetadataContentFromWorkerOutput(
	output string,
	traceID string,
	sessionID string,
	backendLabel string,
) ([]work.WorkContentPart, error) {
	audioParts, err := audioPartsFromInferenceOutput(output)
	if err != nil {
		return nil, err
	}
	if len(audioParts) == 0 {
		return nil, fmt.Errorf("tts worker output did not include audio content")
	}

	audio := audioParts[0]
	artifactPath := strings.TrimSpace(audio.File)
	if artifactPath == "" {
		artifactPath = strings.TrimSpace(audio.URL)
	}
	if artifactPath == "" {
		return nil, fmt.Errorf("tts worker output did not include an artifact reference")
	}

	mediaType := strings.TrimSpace(audio.ContentType)
	if mediaType == "" {
		mediaType = defaultAudioContentType
	}
	if strings.TrimSpace(backendLabel) == "" {
		backendLabel = factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
	}

	encoded, err := json.Marshal(factorydefinitions.TTSInvocationMetadata{
		ArtifactPath: artifactPath,
		MediaType:    mediaType,
		Backend:      backendLabel,
		TraceID:      strings.TrimSpace(traceID),
		SessionID:    strings.TrimSpace(sessionID),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tts invocation metadata: %w", err)
	}
	return textContent(string(encoded)), nil
}

func summaryContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	summary := normalizeStoppedText(output, stopToken)
	if summary == "" {
		return nil, fmt.Errorf("goal worker output is empty after normalization")
	}
	return textContent(summary), nil
}

func responseContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	response := normalizeStoppedText(output, stopToken)
	if response == "" {
		return nil, fmt.Errorf("subagent worker output is empty after normalization")
	}
	return textContent(response), nil
}

func workstationHasAgentOutputRole(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	name string,
) bool {
	if workstation == nil || strings.TrimSpace(workstation.Name) != name {
		return false
	}
	switch factorydefinitions.EffectiveWorkstationTypeForCompatibility(factorydefinitions.Workstation{
		Name:           workstation.Name,
		Type:           workstation.Type,
		Kind:           workstation.Kind,
		WorkerTypeName: workstation.WorkerTypeName,
	}) {
	case factorydefinitions.WorkstationTypeModel, factorydefinitions.WorkstationTypeAgent:
		return true
	default:
		return false
	}
}

func normalizeStoppedText(output, stopToken string) string {
	trimmed := strings.TrimSpace(output)
	stopToken = strings.TrimSpace(stopToken)
	if stopToken == "" {
		return trimmed
	}
	if strings.HasSuffix(trimmed, stopToken) {
		return strings.TrimSpace(strings.TrimSuffix(trimmed, stopToken))
	}
	if index := strings.LastIndex(trimmed, "\n"+stopToken); index >= 0 {
		return strings.TrimSpace(trimmed[:index])
	}
	return trimmed
}

func audioPartsFromInferenceOutput(output string) ([]work.WorkContentPart, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil, nil
	}
	var parts []work.WorkContentPart
	if err := json.Unmarshal([]byte(trimmed), &parts); err != nil {
		var envelope struct {
			Content []work.WorkContentPart `json:"content"`
		}
		if envelopeErr := json.Unmarshal([]byte(trimmed), &envelope); envelopeErr != nil || envelope.Content == nil {
			return nil, fmt.Errorf("inference response is not valid WorkContent JSON for operation %q", "TTS")
		}
		parts = envelope.Content
	}
	audio := make([]work.WorkContentPart, 0, len(parts))
	for _, part := range work.SupportedContentParts(parts) {
		if part.Type.Normalized() == work.WorkContentPartTypeAudio {
			audio = append(audio, part)
		}
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("tts worker output did not include audio content")
	}
	return audio, nil
}

func textContent(text string) []work.WorkContentPart {
	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: text,
	}}
}

func workstationStopToken(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) string {
	if workstation == nil || runtimeConfig == nil {
		return ""
	}
	lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup)
	if !ok {
		return ""
	}
	worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName))
	if !ok || worker == nil {
		return ""
	}
	return strings.TrimSpace(worker.StopToken)
}
