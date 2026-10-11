package cli

import (
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// normalizePromptStart keeps provider policy on the selected host. The CLI
// supplies only the prompt, caller identities and explicit route overrides.
func normalizePromptStart(config InvokeConfig) (normalizedInvokeRequest, error) {
	prompt, err := resolveInvokeMessage(config)
	if err != nil {
		return normalizedInvokeRequest{}, err
	}
	if strings.TrimSpace(prompt) == "" {
		return normalizedInvokeRequest{}, newCLIError("WORKER_SESSION_START_INVALID", "provide a non-empty prompt or stdin", nil)
	}
	if config.TimeoutMillis != nil && (*config.TimeoutMillis < 1 || *config.TimeoutMillis > 9223372036854) {
		return normalizedInvokeRequest{}, newCLIError("WORKER_SESSION_START_INVALID", "--timeout-millis must be between 1 and 9223372036854", nil)
	}
	request := factoryapi.WorkerSessionStartRequest{
		RequestId: strings.TrimSpace(config.RequestID), WorkerSessionId: strings.TrimSpace(config.WorkerSessionID),
		Prompt: &prompt, Provider: startOptionalString(config.Provider), Model: startOptionalString(config.Model),
		ReasoningEffort: startOptionalString(config.ReasoningEffort), WorkingRoot: startOptionalString(config.WorkingRoot),
		TimeoutMillis: config.TimeoutMillis,
	}
	if request.RequestId == "" || request.WorkerSessionId == "" {
		if config.GenerateID == nil {
			return normalizedInvokeRequest{}, newCLIError("WORKER_SESSION_START_INVALID", "start identity generator is unavailable; supply both identities", nil)
		}
		if request.RequestId == "" {
			request.RequestId = config.GenerateID()
		}
		if request.WorkerSessionId == "" {
			request.WorkerSessionId = config.GenerateID()
		}
	}
	return normalizedInvokeRequest{API: request, Service: workersessions.StartRequest{
		RequestID: request.RequestId, ID: request.WorkerSessionId, Caller: config.Caller,
	}}, nil
}

func startOptionalString(value string) *string {
	if value = strings.TrimSpace(value); value == "" {
		return nil
	}
	return &value
}
