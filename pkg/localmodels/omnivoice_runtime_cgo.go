//go:build omnivoice_cgo && cgo

package localmodels

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	embedded "github.com/portpowered/infinite-you/pkg/localmodels/omnivoice"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// embeddedOmniVoiceRuntime adapts the OmniVoice-specific embedded runtime to
// the generic local-model manager contract.
type embeddedOmniVoiceRuntime struct{}

type embeddedOmniVoiceHandle struct{ model *embedded.Model }

func newOmniVoiceRuntime(_ workers.CommandRunner) Runtime { return &embeddedOmniVoiceRuntime{} }

func (r *embeddedOmniVoiceRuntime) Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool {
	return worker != nil && strings.TrimSpace(worker.ModelLocality) == interfaces.ModelLocalityLocal &&
		CanonicalBackendName(resource.Backend) == "LLAMACPP" &&
		canonicalModelName(worker.Model) == canonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *embeddedOmniVoiceRuntime) Load(_ context.Context, request LoadRequest) (Handle, error) {
	if !r.Supports(request.Resource, request.Worker) {
		return nil, fmt.Errorf("unsupported embedded OMNIVOICE runtime for model %q with backend %q", request.ModelName, request.Resource.Backend)
	}
	modelPath, codecPath, err := omniVoiceCacheFiles(request.Files)
	if err != nil {
		return nil, err
	}
	model, err := embedded.Open(modelPath, codecPath)
	if err != nil {
		return nil, err
	}
	handle := &embeddedOmniVoiceHandle{model: model}
	runtime.SetFinalizer(handle, (*embeddedOmniVoiceHandle).close)
	return handle, nil
}

func (h *embeddedOmniVoiceHandle) Invoke(ctx context.Context, request InvocationRequest) (interfaces.InferenceResponse, error) {
	if h == nil || h.model == nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("embedded OMNIVOICE runtime handle is required")
	}
	if err := ctx.Err(); err != nil {
		return interfaces.InferenceResponse{}, err
	}
	if operation := strings.TrimSpace(request.Request.ModelOperation); operation != "TTS" {
		return interfaces.InferenceResponse{}, fmt.Errorf("embedded OMNIVOICE runtime only supports TTS, got %q", operation)
	}
	outputPath, err := omniVoiceOutputPath("")
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	samples, sampleRate, err := h.model.Synthesize(request.Request.ModelBindings)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	if err := embedded.WriteWAV(outputPath, samples, sampleRate); err != nil {
		return interfaces.InferenceResponse{}, err
	}
	content, err := omniVoiceResponseContent("", outputPath)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("encode embedded OMNIVOICE response content: %w", err)
	}
	return interfaces.InferenceResponse{Content: string(encoded)}, nil
}

func (h *embeddedOmniVoiceHandle) close() {
	if h != nil && h.model != nil {
		h.model.Close()
		h.model = nil
	}
}
