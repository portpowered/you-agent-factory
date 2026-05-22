package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

const (
	defaultOmniVoiceCommand       = "omnivoice-llamacpp"
	omniVoiceInvokeSubcommand     = "invoke"
	omniVoiceAudioContentType     = "audio/wav"
	omniVoiceModelSlotNameText    = "text"
	omniVoiceModelSlotNameAudio   = "audio"
	omniVoiceTokenizerNameSnippet = "tokenizer"
)

type omniVoiceLocalRuntime struct {
	runner workers.CommandRunner
}

type omniVoiceLocalHandle struct {
	runner        workers.CommandRunner
	command       string
	baseArgs      []string
	modelName     string
	cachePath     string
	revision      string
	modelPath     string
	tokenizerPath string
}

type omniVoiceInvocationPayload struct {
	Operation  string                                     `json:"operation"`
	ModelName  string                                     `json:"modelName"`
	Revision   string                                     `json:"revision,omitempty"`
	OutputFile string                                     `json:"outputFile"`
	Text       string                                     `json:"text"`
	Bindings   []interfaces.ResolvedModelOperationBinding `json:"bindings,omitempty"`
}

func newOmniVoiceLocalRuntime(runner workers.CommandRunner) localModelRuntime {
	if runner == nil {
		runner = workers.ExecCommandRunner{}
	}
	return &omniVoiceLocalRuntime{runner: runner}
}

func (r *omniVoiceLocalRuntime) Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool {
	if worker == nil {
		return false
	}
	return strings.TrimSpace(worker.ModelLocality) == interfaces.ModelLocalityLocal &&
		canonicalBackendName(resource.Backend) == "LLAMACPP" &&
		canonicalModelName(worker.Model) == canonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *omniVoiceLocalRuntime) Load(_ context.Context, request localModelLoadRequest) (localModelHandle, error) {
	if !r.Supports(request.Resource, request.Worker) {
		return nil, fmt.Errorf("unsupported local model runtime for model %q with backend %q", request.ModelName, request.Resource.Backend)
	}
	modelPath, tokenizerPath, err := omniVoiceCacheFiles(request.Files)
	if err != nil {
		return nil, err
	}
	command, args := omniVoiceCommandForWorker(request.Worker)
	return &omniVoiceLocalHandle{
		runner:        r.runner,
		command:       command,
		baseArgs:      args,
		modelName:     request.ModelName,
		cachePath:     request.CachePath,
		revision:      request.Revision,
		modelPath:     modelPath,
		tokenizerPath: tokenizerPath,
	}, nil
}

func (h *omniVoiceLocalHandle) Invoke(ctx context.Context, request localModelInvocationRequest) (interfaces.InferenceResponse, error) {
	if h == nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("local model handle is required")
	}
	operation := strings.TrimSpace(request.Request.ModelOperation)
	if operation != "TTS" {
		return interfaces.InferenceResponse{}, fmt.Errorf("local OMNIVOICE runtime only supports TTS, got %q", operation)
	}
	text, err := omniVoiceBoundText(request.Request.ModelBindings)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	outputFile, err := omniVoiceOutputPath(h.cachePath)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}

	payload := omniVoiceInvocationPayload{
		Operation:  operation,
		ModelName:  h.modelName,
		Revision:   h.revision,
		OutputFile: outputFile,
		Text:       text,
		Bindings:   interfaces.CloneResolvedModelOperationBindings(request.Request.ModelBindings),
	}
	stdin, err := json.Marshal(payload)
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("encode local OMNIVOICE invocation payload: %w", err)
	}

	result, err := h.runner.Run(ctx, omniVoiceCommandRequest(request.Request, h, outputFile, stdin))
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("run local OMNIVOICE runtime: %w", err)
	}
	if result.ExitCode != 0 {
		return interfaces.InferenceResponse{}, fmt.Errorf("local OMNIVOICE runtime exited with code %d: %s", result.ExitCode, omniVoiceCombinedOutput(result))
	}

	content, err := omniVoiceResponseContent(strings.TrimSpace(string(result.Stdout)), outputFile)
	if err != nil {
		return interfaces.InferenceResponse{}, err
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return interfaces.InferenceResponse{}, fmt.Errorf("encode local OMNIVOICE response content: %w", err)
	}
	return interfaces.InferenceResponse{Content: string(encoded)}, nil
}

func omniVoiceCacheFiles(files []string) (string, string, error) {
	var modelPath string
	var tokenizerPath string
	for _, file := range files {
		trimmed := strings.TrimSpace(file)
		if trimmed == "" {
			continue
		}
		info, err := os.Stat(trimmed)
		if err != nil {
			return "", "", fmt.Errorf("inspect local model asset %q: %w", trimmed, err)
		}
		if info.IsDir() {
			return "", "", fmt.Errorf("local model asset %q is a directory", trimmed)
		}
		name := strings.ToLower(filepath.Base(trimmed))
		if strings.Contains(name, omniVoiceTokenizerNameSnippet) {
			tokenizerPath = trimmed
			continue
		}
		if modelPath == "" {
			modelPath = trimmed
		}
	}
	if modelPath == "" {
		return "", "", fmt.Errorf("local OMNIVOICE model asset is required")
	}
	if tokenizerPath == "" {
		return "", "", fmt.Errorf("local OMNIVOICE tokenizer asset is required")
	}
	return modelPath, tokenizerPath, nil
}

func omniVoiceCommandForWorker(worker *interfaces.WorkerConfig) (string, []string) {
	if worker == nil {
		return defaultOmniVoiceCommand, nil
	}
	command := strings.TrimSpace(worker.Command)
	if command == "" {
		command = defaultOmniVoiceCommand
	}
	return command, append([]string(nil), worker.Args...)
}

func omniVoiceBoundText(bindings []interfaces.ResolvedModelOperationBinding) (string, error) {
	for _, binding := range bindings {
		if !strings.EqualFold(strings.TrimSpace(binding.Slot), omniVoiceModelSlotNameText) {
			continue
		}
		var parts []string
		for _, content := range binding.Content {
			if content.Type.Normalized() != interfaces.WorkContentPartTypeText {
				continue
			}
			text := strings.TrimSpace(content.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) == 0 {
			return "", fmt.Errorf("local OMNIVOICE runtime requires TEXT content for slot %q", omniVoiceModelSlotNameText)
		}
		return strings.Join(parts, "\n"), nil
	}
	return "", fmt.Errorf("local OMNIVOICE runtime requires resolved slot %q", omniVoiceModelSlotNameText)
}

func omniVoiceOutputPath(cachePath string) (string, error) {
	dir := strings.TrimSpace(cachePath)
	if dir == "" {
		dir = os.TempDir()
	}
	file, err := os.CreateTemp(dir, "omnivoice-*.wav")
	if err != nil {
		return "", fmt.Errorf("create local OMNIVOICE output file: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close local OMNIVOICE output file: %w", err)
	}
	return file.Name(), nil
}

func omniVoiceCommandRequest(request interfaces.RunnerExecutionRequest, handle *omniVoiceLocalHandle, outputFile string, stdin []byte) workers.CommandRequest {
	dispatch := interfaces.CloneWorkDispatch(request.Dispatch)
	args := append([]string(nil), handle.baseArgs...)
	args = append(args,
		omniVoiceInvokeSubcommand,
		"--model", handle.modelPath,
		"--tokenizer", handle.tokenizerPath,
		"--output", outputFile,
	)
	return workers.CommandRequest{
		Command:                  handle.command,
		Args:                     args,
		Stdin:                    stdin,
		DispatchID:               dispatch.DispatchID,
		TransitionID:             dispatch.TransitionID,
		WorkerType:               dispatch.WorkerType,
		WorkstationName:          dispatch.WorkstationName,
		ProjectID:                dispatch.ProjectID,
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: dispatch.PreviousChainingTraceIDs,
		Execution:                dispatch.Execution,
		InputTokens:              dispatch.InputTokens,
		InputBindings:            dispatch.InputBindings,
		WorkDir:                  strings.TrimSpace(request.WorkingDirectory),
	}
}

func omniVoiceCombinedOutput(result workers.CommandResult) string {
	parts := make([]string, 0, 2)
	if stdout := strings.TrimSpace(string(result.Stdout)); stdout != "" {
		parts = append(parts, "stdout: "+stdout)
	}
	if stderr := strings.TrimSpace(string(result.Stderr)); stderr != "" {
		parts = append(parts, "stderr: "+stderr)
	}
	if len(parts) == 0 {
		return "no command output"
	}
	return strings.Join(parts, "; ")
}

func omniVoiceResponseContent(stdout string, outputFile string) ([]interfaces.WorkContentPart, error) {
	content := make([]interfaces.WorkContentPart, 0, 1)
	if stdout != "" {
		var direct []interfaces.WorkContentPart
		if err := json.Unmarshal([]byte(stdout), &direct); err == nil {
			content = direct
		} else {
			var envelope struct {
				Content []interfaces.WorkContentPart `json:"content"`
			}
			if envelopeErr := json.Unmarshal([]byte(stdout), &envelope); envelopeErr != nil {
				return nil, fmt.Errorf("decode local OMNIVOICE response: %w", err)
			}
			content = envelope.Content
		}
	}
	if len(content) == 0 {
		content = []interfaces.WorkContentPart{{
			Type:        interfaces.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: omniVoiceAudioContentType,
		}}
	}
	audioFound := false
	for i := range content {
		if content[i].Type.Normalized() != interfaces.WorkContentPartTypeAudio {
			continue
		}
		audioFound = true
		if strings.TrimSpace(content[i].File) == "" {
			content[i].File = outputFile
		}
		if strings.TrimSpace(content[i].ContentType) == "" {
			content[i].ContentType = omniVoiceAudioContentType
		}
		if strings.TrimSpace(content[i].Slot) == "" {
			content[i].Slot = omniVoiceModelSlotNameAudio
		}
	}
	if !audioFound {
		content = append(content, interfaces.WorkContentPart{
			Type:        interfaces.WorkContentPartTypeAudio,
			Slot:        omniVoiceModelSlotNameAudio,
			File:        outputFile,
			ContentType: omniVoiceAudioContentType,
		})
	}
	if _, err := os.Stat(outputFile); err != nil {
		return nil, fmt.Errorf("inspect local OMNIVOICE output file %q: %w", outputFile, err)
	}
	return content, nil
}
