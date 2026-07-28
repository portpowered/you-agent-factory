package http

import (
	"encoding/json"
	"errors"
	"io"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// MoveWorkRequestFromBody decodes one move-work request body.
func MoveWorkRequestFromBody(body io.Reader) (factoryapi.MoveWorkRequest, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	if len(payload) == 0 {
		return factoryapi.MoveWorkRequest{}, errors.New("request body is required")
	}

	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &rawFields); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	if err := requireOnlyFields(rawFields, "", "stateName", "requestId"); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}

	var req factoryapi.MoveWorkRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return factoryapi.MoveWorkRequest{}, err
	}
	return req, nil
}
