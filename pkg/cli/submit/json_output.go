package submit

import (
	"encoding/json"
	"io"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/cli/sessionpath"
)

// SubmitSuccessResult is the stable CLI JSON confirmation emitted after HTTP 201.
type SubmitSuccessResult struct {
	WorkID       *string `json:"workId"`
	Name         string  `json:"name"`
	WorkTypeName string  `json:"workTypeName"`
	TraceID      string  `json:"traceId"`
	SessionID    string  `json:"sessionId"`
	EndpointPath string  `json:"endpointPath"`
}

func writeJSONSubmitSuccess(
	w io.Writer,
	result factoryapi.SubmitWorkResponse,
	endpointPath string,
	fallbackName string,
	fallbackWorkType string,
	fallbackSessionID string,
) error {
	envelope := submitSuccessResultFromResponse(result, endpointPath, fallbackName, fallbackWorkType, fallbackSessionID)
	return json.NewEncoder(w).Encode(envelope)
}

func submitSuccessResultFromResponse(
	result factoryapi.SubmitWorkResponse,
	endpointPath string,
	fallbackName string,
	fallbackWorkType string,
	fallbackSessionID string,
) SubmitSuccessResult {
	name := submitResponseString(result.Name, fallbackName)
	workType := submitResponseString(result.WorkTypeName, fallbackWorkType)

	var workID *string
	if trimmed := submitResponseString(result.WorkId, ""); trimmed != "" {
		workID = &trimmed
	}

	return SubmitSuccessResult{
		WorkID:       workID,
		Name:         name,
		WorkTypeName: workType,
		TraceID:      result.TraceId,
		SessionID:    submitJSONSessionID(result, fallbackSessionID),
		EndpointPath: endpointPath,
	}
}

func submitJSONSessionID(result factoryapi.SubmitWorkResponse, cfgSessionID string) string {
	if id := submitResponseString(result.SessionId, ""); id != "" {
		return id
	}
	if trimmed := strings.TrimSpace(cfgSessionID); trimmed != "" {
		return trimmed
	}
	return sessionpath.DefaultFactorySessionID
}
