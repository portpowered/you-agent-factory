package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func validateForceConfig(config ControlConfig) error {
	invalid := config.Force && (config.Action != workersessions.ControlActionTerminate ||
		strings.TrimSpace(config.RequestID) == "" || strings.TrimSpace(config.ExpectedAttemptID) == "")
	invalid = invalid || (!config.Force && (config.RequestID != "" || config.ExpectedAttemptID != ""))
	if invalid {
		return newCLIError("WORKER_SESSION_CONTROL_INVALID", "terminate --force requires --request-id and --expected-attempt-id; identities require --force", nil)
	}
	return nil
}

func newControlHTTPRequest(config ControlConfig, endpoint string) (*http.Request, error) {
	var body io.Reader
	if config.Force {
		requestID, attemptID := strings.TrimSpace(config.RequestID), strings.TrimSpace(config.ExpectedAttemptID)
		payload, err := json.Marshal(factoryapi.TerminateWorkerSessionJSONRequestBody{
			Force: &config.Force, RequestId: &requestID, ExpectedAttemptId: &attemptID,
		})
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(config.Context, http.MethodPost, endpoint, body)
	if err == nil && config.Force {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, err
}
