package recordingmcp

import "encoding/json"

const (
	// SuccessTextEncodingSerializedJSON documents serialized JSON tool payloads in text content.
	SuccessTextEncodingSerializedJSON = "serialized-json"
)

// EncodeSuccessCallToolResult builds the live MCP tools/call success envelope for one
// serialized tool-response payload. Callers use this shape to document current server
// transport without mutating handlers.
func EncodeSuccessCallToolResult(toolResponse json.RawMessage) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(toolResponse),
			},
		},
	}
}

// MarshalSuccessCallToolResultJSON encodes one success CallToolResult with stable key order.
func MarshalSuccessCallToolResultJSON(toolResponse json.RawMessage) ([]byte, error) {
	return json.Marshal(EncodeSuccessCallToolResult(toolResponse))
}
