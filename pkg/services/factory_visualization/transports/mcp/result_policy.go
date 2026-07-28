package factoryvisualization

import "encoding/json"

// SuccessTextEncodingSerializedJSON documents serialized JSON tool payloads in text content.
const SuccessTextEncodingSerializedJSON = "serialized-json"

// EncodeSuccessCallToolResult builds the live MCP tools/call success envelope for one
// serialized tool-response payload. Protocol servers and tests use this shape to match
// the Sessions MCP success transport policy without mutating handlers.
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
