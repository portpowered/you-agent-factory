package run

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/factoryrun"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// PrepareFactoryPromptWorkFile validates the factory config, builds a canonical
// FACTORY_REQUEST_BATCH for the prompt, and writes it to a temporary JSON file.
func PrepareFactoryPromptWorkFile(factoryConfigPath, prompt string) (workFilePath string, err error) {
	trimmedPrompt := strings.TrimSpace(prompt)
	if trimmedPrompt == "" {
		return "", fmt.Errorf("prompt is required for you run --factory")
	}

	cfg, err := factoryrun.LoadFactoryConfigFromConfigFile(factoryConfigPath)
	if err != nil {
		return "", err
	}
	if err := factoryrun.ValidateFactoryForPromptRun(cfg); err != nil {
		return "", err
	}
	workTypeName, err := factoryrun.DefaultHandlingWorkTypeName(cfg)
	if err != nil {
		return "", err
	}

	request := factoryPromptWorkRequest(workTypeName, trimmedPrompt)
	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("marshal factory prompt work request: %w", err)
	}
	if err := requests.ValidateCanonicalWorkRequestJSON(data); err != nil {
		return "", fmt.Errorf("validate factory prompt work request: %w", err)
	}

	tempFile, err := os.CreateTemp("", "you-run-factory-prompt-*.json")
	if err != nil {
		return "", fmt.Errorf("create factory prompt work file: %w", err)
	}
	workFilePath = tempFile.Name()
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(workFilePath)
		return "", fmt.Errorf("write factory prompt work file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(workFilePath)
		return "", fmt.Errorf("close factory prompt work file: %w", err)
	}
	return workFilePath, nil
}

func factoryPromptWorkRequest(workTypeName, prompt string) interfaces.WorkRequest {
	requestID := requests.NewRequestID()
	workName := factoryPromptWorkName(requestID)
	return interfaces.WorkRequest{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       workName,
			WorkTypeID: workTypeName,
			Payload:    prompt,
		}},
	}
}

func factoryPromptWorkName(requestID string) string {
	suffix := strings.TrimPrefix(requestID, "request-")
	if suffix == "" || suffix == requestID {
		return "factory-prompt"
	}
	return "factory-prompt-" + suffix
}
