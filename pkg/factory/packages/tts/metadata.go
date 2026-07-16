package tts

import (
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	builtintts "github.com/portpowered/infinite-you/pkg/factory/packages/definitions/tts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerinference "github.com/portpowered/infinite-you/pkg/workers/inference"
)

// BuiltInFactoryJSON is the canonical runnable @you/tts definition owned by
// the factory packages family.
var BuiltInFactoryJSON = builtintts.BuiltInFactoryJSON

const defaultAudioContentType = "audio/wav"

const (
	// PackagedFactoryProject identifies the built-in @you/tts factory project id.
	PackagedFactoryProject = "builtin-tts"
	// PackagedInvokeWorkstationName is the MODEL_INVOKE workstation that runs TTS.
	PackagedInvokeWorkstationName = "execute-tts"
	// DefaultModelName is the default managed local TTS model for @you/tts.
	DefaultModelName = "OMNIVOICE_Q4_K_M"
	// DefaultBackendName is the default managed local TTS backend for @you/tts.
	DefaultBackendName = "LLAMACPP"
)

// InvocationMetadata is the default primary invocation result for successful @you/tts runs.
type InvocationMetadata struct {
	ArtifactPath string `json:"artifactPath"`
	MediaType    string `json:"mediaType"`
	Backend      string `json:"backend"`
	TraceID      string `json:"traceId,omitempty"`
	SessionID    string `json:"sessionId,omitempty"`
}

// ShouldFormatInvocationMetadata reports whether workstation output should be
// shaped into packaged TTS invocation metadata for terminal work content.
func ShouldFormatInvocationMetadata(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	if strings.TrimSpace(workstation.Name) != PackagedInvokeWorkstationName {
		return false
	}
	return interfaces.IsInferenceRunWorkstationType(workstation.Type) &&
		strings.EqualFold(strings.TrimSpace(workstation.Operation), "TTS")
}

// BackendLabelFromWorker derives the packaged TTS backend identifier from the
// loaded on-disk worker configuration. An empty or nil worker falls back to the
// packaged factory defaults.
func BackendLabelFromWorker(worker *workerconfig.Config) string {
	model := DefaultModelName
	backend := DefaultBackendName
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

// MetadataContentFromWorkerOutput parses a MODEL_INVOKE TTS worker output payload
// and returns canonical text work content for invocation primary-result selection.
// When backendLabel is empty, the packaged factory default backend label is used.
func MetadataContentFromWorkerOutput(output, traceID, sessionID, backendLabel string) ([]work.WorkContentPart, error) {
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
		backendLabel = defaultBackendLabel()
	}

	metadata := InvocationMetadata{
		ArtifactPath: artifactPath,
		MediaType:    mediaType,
		Backend:      backendLabel,
		TraceID:      strings.TrimSpace(traceID),
		SessionID:    strings.TrimSpace(sessionID),
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal tts invocation metadata: %w", err)
	}

	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: string(encoded),
	}}, nil
}

func audioPartsFromInferenceOutput(output string) ([]work.WorkContentPart, error) {
	parts, err := workerinference.WorkContentFromInferenceOutput(output, workerconfig.ModelOperation{
		Name: "TTS",
		Outputs: []workerconfig.ModelOperationSlot{{
			Name:         "audio",
			ContentTypes: []string{workerconfig.ModelOperationContentTypeAudio},
		}},
	})
	if err != nil {
		return nil, err
	}
	audio := audioPartsOnly(parts)
	if len(audio) == 0 {
		return nil, fmt.Errorf("tts worker output did not include audio content")
	}
	return audio, nil
}

func audioPartsOnly(parts []work.WorkContentPart) []work.WorkContentPart {
	audio := make([]work.WorkContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeAudio {
			audio = append(audio, part)
		}
	}
	return audio
}

func defaultBackendLabel() string {
	return DefaultModelName + "/" + DefaultBackendName
}
