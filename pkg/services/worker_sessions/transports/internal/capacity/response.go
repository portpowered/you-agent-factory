// Package capacity decodes the bounded public START refusal shared by CLI and MCP.
package capacity

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

const Code = "WORKER_SESSION_DIRECT_CAPACITY_EXCEEDED"

// Decode accepts only a complete known envelope with unambiguous typed fields.
func Decode(body io.Reader) (workersessions.DirectCapacityFailure, bool) {
	if body == nil {
		return workersessions.DirectCapacityFailure{}, false
	}
	raw, err := io.ReadAll(io.LimitReader(body, 4097))
	if err != nil || len(raw) > 4096 {
		return workersessions.DirectCapacityFailure{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if !uniqueValue(decoder, 0) || decoder.Decode(new(any)) != io.EOF {
		return workersessions.DirectCapacityFailure{}, false
	}
	var response struct {
		Message string `json:"message"`
		Family  string `json:"family"`
		Code    string `json:"code"`
		Details struct {
			Host     string `json:"host"`
			Capacity int64  `json:"capacity"`
		} `json:"details"`
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&response) != nil || response.Code != Code || response.Family != "CONFLICT" || response.Message == "" || response.Details.Capacity < 1 {
		return workersessions.DirectCapacityFailure{}, false
	}
	host := response.Details.Host
	if host == "" || (host != "local" && (workersessions.DirectCapacityFailure{Host: host}).PublicHost() == "local") {
		return workersessions.DirectCapacityFailure{}, false
	}
	return workersessions.DirectCapacityFailure{Host: (workersessions.DirectCapacityFailure{Host: host}).PublicHost(), Capacity: response.Details.Capacity}, true
}

func uniqueValue(decoder *json.Decoder, depth int) bool {
	if depth > 3 {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	opening, compound := token.(json.Delim)
	if !compound {
		return true
	}
	if opening != '{' {
		return false
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		canonical := strings.ToLower(key)
		if err != nil || !ok || seen[canonical] || key != canonical {
			return false
		}
		seen[canonical] = true
		if !uniqueValue(decoder, depth+1) {
			return false
		}
	}
	closing, err := decoder.Token()
	return err == nil && closing == json.Delim('}')
}
