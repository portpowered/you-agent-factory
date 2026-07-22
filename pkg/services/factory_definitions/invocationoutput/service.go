package invocationoutput

import (
	"encoding/json"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const (
	packagedGoalExecuteWorkstationName = "execute-goal"
	packagedSubagentRunWorkstationName = "run-subagent"
	defaultAudioContentType            = "audio/wav"
)

// Service implements packaged invocation-output shaping policy.
type Service struct{}

var _ factorydefinitions.InvocationOutputShapingService = Service{}

// NewService returns the canonical packaged invocation-output shaper.
func NewService() factorydefinitions.InvocationOutputShapingService {
	return Service{}
}

func (Service) ShouldFormatInvocationSummary(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return ShouldFormatInvocationSummary(workstation)
}

func (Service) SummaryContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	return SummaryContentFromWorkerOutput(output, stopToken)
}

func (Service) ShouldFormatInvocationResponse(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return ShouldFormatInvocationResponse(workstation)
}

func (Service) ResponseContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	return ResponseContentFromWorkerOutput(output, stopToken)
}

func (Service) ShouldFormatTTSInvocationMetadata(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return ShouldFormatTTSInvocationMetadata(workstation)
}

func (Service) TTSBackendLabelFromWorker(worker *factorydefinitions.FactoryWorkerConfig) string {
	return TTSBackendLabelFromWorker(worker)
}

func (Service) TTSMetadataContentFromWorkerOutput(
	output string,
	traceID string,
	sessionID string,
	backendLabel string,
) ([]work.WorkContentPart, error) {
	return TTSMetadataContentFromWorkerOutput(output, traceID, sessionID, backendLabel)
}

// ShouldFormatInvocationSummary reports whether workstation output should be
// shaped into packaged Goal summary content.
func ShouldFormatInvocationSummary(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstationHasAgentOutputRole(workstation, packagedGoalExecuteWorkstationName)
}

// SummaryContentFromWorkerOutput converts Goal worker output into canonical text.
func SummaryContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	summary := normalizeStoppedText(output, stopToken)
	if summary == "" {
		return nil, fmt.Errorf("goal worker output is empty after normalization")
	}
	return textContent(summary), nil
}

// ShouldFormatInvocationResponse reports whether workstation output should be
// shaped into packaged Subagent response content.
func ShouldFormatInvocationResponse(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	return workstationHasAgentOutputRole(workstation, packagedSubagentRunWorkstationName)
}

// ResponseContentFromWorkerOutput converts Subagent worker output into canonical text.
func ResponseContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	response := normalizeStoppedText(output, stopToken)
	if response == "" {
		return nil, fmt.Errorf("subagent worker output is empty after normalization")
	}
	return textContent(response), nil
}

// ShouldFormatTTSInvocationMetadata reports whether workstation output should
// be shaped into packaged TTS invocation metadata.
func ShouldFormatTTSInvocationMetadata(workstation *factorydefinitions.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	return strings.TrimSpace(workstation.Name) == factorydefinitions.PackagedTTSInvokeWorkstationName &&
		factorydefinitions.IsInferenceRunWorkstationType(workstation.Type) &&
		strings.EqualFold(strings.TrimSpace(workstation.Operation), "TTS")
}

// TTSBackendLabelFromWorker derives the packaged TTS backend identifier from a
// loaded worker definition.
func TTSBackendLabelFromWorker(worker *factorydefinitions.FactoryWorkerConfig) string {
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

// TTSMetadataContentFromWorkerOutput converts TTS audio output into canonical
// invocation metadata content.
func TTSMetadataContentFromWorkerOutput(
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
	case contracts.WorkstationTypeModel, contracts.WorkstationTypeAgent:
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
