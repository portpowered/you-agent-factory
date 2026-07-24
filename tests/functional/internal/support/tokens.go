package support

import (
	"encoding/json"
)

type publicInputColor struct {
	WorkID  string            `json:"work_id"`
	Payload json.RawMessage   `json:"payload"`
	Tags    map[string]string `json:"tags"`
}

type publicInputToken struct {
	Color publicInputColor `json:"color"`
}

// FirstInputWorkID returns the first dispatched input's Work ID without exposing
// a private workers.Token (or other engine snapshot) to callers.
func FirstInputWorkID(rawTokens any) string {
	color, ok := firstInputColor(rawTokens)
	if !ok {
		return ""
	}
	return color.WorkID
}

// FirstInputPayload returns the first dispatched input's payload bytes without
// exposing a private token object to callers.
func FirstInputPayload(rawTokens any) []byte {
	color, ok := firstInputColor(rawTokens)
	if !ok || len(color.Payload) == 0 {
		return nil
	}
	var asBytes []byte
	if err := json.Unmarshal(color.Payload, &asBytes); err == nil {
		return asBytes
	}
	return append([]byte(nil), color.Payload...)
}

// FirstInputTags returns a copy of the first dispatched input's tags without
// exposing a private token object to callers.
func FirstInputTags(rawTokens any) map[string]string {
	color, ok := firstInputColor(rawTokens)
	if !ok || len(color.Tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(color.Tags))
	for key, value := range color.Tags {
		out[key] = value
	}
	return out
}

func firstInputColor(rawTokens any) (publicInputColor, bool) {
	if rawTokens == nil {
		return publicInputColor{}, false
	}
	encoded, err := json.Marshal(rawTokens)
	if err != nil {
		return publicInputColor{}, false
	}
	var tokens []publicInputToken
	if err := json.Unmarshal(encoded, &tokens); err != nil {
		return publicInputColor{}, false
	}
	if len(tokens) == 0 {
		return publicInputColor{}, false
	}
	return tokens[0].Color, true
}
