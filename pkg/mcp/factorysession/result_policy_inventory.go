package factorysession

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// ResultPolicyInventoryBaselineRelativePath is the reviewed MCP CallToolResult
	// success transport policy inventory fixture.
	ResultPolicyInventoryBaselineRelativePath = "contracts/testdata/baseline/mcp-result-policy.json"
	// SuccessTextEncodingSerializedJSON documents serialized JSON tool payloads in text content.
	SuccessTextEncodingSerializedJSON = "serialized-json"
)

// ResultPolicyInventory is a pure, read-only projection of MCP success transport policy.
type ResultPolicyInventory struct {
	FormatVersion    string                   `json:"formatVersion"`
	ProtocolVersion  string                   `json:"protocolVersion"`
	SuccessTransport SuccessTransportPolicy   `json:"successTransport"`
	Fixtures         []ResultPolicyFixture    `json:"fixtures"`
}

// SuccessTransportPolicy records the live CallToolResult success envelope contract.
type SuccessTransportPolicy struct {
	ContentItemCount        int      `json:"contentItemCount"`
	ContentTypes            []string `json:"contentTypes"`
	TextEncoding            string   `json:"textEncoding"`
	IsError                 bool     `json:"isError"`
	HasStructuredContent    bool     `json:"hasStructuredContent"`
	UnsupportedContentTypes []string `json:"unsupportedContentTypes"`
	UnsupportedFields       []string `json:"unsupportedFields"`
}

// ResultPolicyFixture records one representative tools/call success encoding.
type ResultPolicyFixture struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	ToolName       string          `json:"toolName"`
	ToolResponse   json.RawMessage `json:"toolResponse"`
	CallToolResult json.RawMessage `json:"callToolResult"`
}

// ProjectResultPolicyInventory builds the reviewed success transport policy inventory.
func ProjectResultPolicyInventory() (ResultPolicyInventory, error) {
	toolResponse, err := representativeListSessionsSuccessToolResponse()
	if err != nil {
		return ResultPolicyInventory{}, err
	}
	callToolResult, err := MarshalSuccessCallToolResultJSON(toolResponse)
	if err != nil {
		return ResultPolicyInventory{}, err
	}
	fixtures := []ResultPolicyFixture{{
		Name:        "list_sessions_live_default",
		Description: "Representative tools/call success for you.factory_session.list with default live scope.",
		ToolName:    ToolListSessions,
		ToolResponse: toolResponse,
		CallToolResult: callToolResult,
	}}
	slices.SortFunc(fixtures, func(left, right ResultPolicyFixture) int {
		return strings.Compare(left.Name, right.Name)
	})
	return ResultPolicyInventory{
		FormatVersion:   ToolInventoryFormatVersion,
		ProtocolVersion: ToolInventoryProtocolVersion,
		SuccessTransport: SuccessTransportPolicy{
			ContentItemCount:        1,
			ContentTypes:            []string{"text"},
			TextEncoding:            SuccessTextEncodingSerializedJSON,
			IsError:                 false,
			HasStructuredContent:    false,
			UnsupportedContentTypes: []string{"image", "audio", "resource"},
			UnsupportedFields:       []string{"outputSchema", "structuredContent"},
		},
		Fixtures: fixtures,
	}, nil
}

// MarshalResultPolicyInventoryJSON encodes one result-policy inventory document.
func MarshalResultPolicyInventoryJSON(inventory ResultPolicyInventory) ([]byte, error) {
	return json.Marshal(inventory)
}

// EncodeSuccessCallToolResult builds the live MCP tools/call success envelope for one
// serialized tool-response payload. Inventory callers and tests use this shape to
// document current server transport without mutating handlers.
func EncodeSuccessCallToolResult(toolResponse json.RawMessage) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": string(toolResponse),
			},
		},
		"isError": false,
	}
}

// MarshalSuccessCallToolResultJSON encodes one success CallToolResult with stable key order.
func MarshalSuccessCallToolResultJSON(toolResponse json.RawMessage) ([]byte, error) {
	return json.Marshal(EncodeSuccessCallToolResult(toolResponse))
}

// VerifyProjectedResultPolicyInventory projects the result-policy inventory and fails
// when success transport policy or fixtures drift from the documented contract.
func VerifyProjectedResultPolicyInventory() error {
	inventory, err := ProjectResultPolicyInventory()
	if err != nil {
		return err
	}
	return VerifyResultPolicyInventory(inventory)
}

// VerifyResultPolicyInventory fails when success transport policy or fixtures do not
// match the documented text-only serialized-JSON contract.
func VerifyResultPolicyInventory(inventory ResultPolicyInventory) error {
	policy := inventory.SuccessTransport
	if policy.ContentItemCount != 1 {
		return fmt.Errorf("success transport contentItemCount = %d, want 1", policy.ContentItemCount)
	}
	if len(policy.ContentTypes) != 1 || policy.ContentTypes[0] != "text" {
		return fmt.Errorf("success transport contentTypes = %#v, want [text]", policy.ContentTypes)
	}
	if policy.TextEncoding != SuccessTextEncodingSerializedJSON {
		return fmt.Errorf("success transport textEncoding = %q, want %q", policy.TextEncoding, SuccessTextEncodingSerializedJSON)
	}
	if policy.IsError {
		return fmt.Errorf("success transport isError = true, want false")
	}
	if policy.HasStructuredContent {
		return fmt.Errorf("success transport hasStructuredContent = true, want false")
	}
	for _, unsupported := range []string{"image", "audio", "resource"} {
		if !slices.Contains(policy.UnsupportedContentTypes, unsupported) {
			return fmt.Errorf("success transport unsupportedContentTypes missing %q", unsupported)
		}
	}
	for _, unsupported := range []string{"outputSchema", "structuredContent"} {
		if !slices.Contains(policy.UnsupportedFields, unsupported) {
			return fmt.Errorf("success transport unsupportedFields missing %q", unsupported)
		}
	}

	seenFixtureNames := make(map[string]struct{}, len(inventory.Fixtures))
	for _, fixture := range inventory.Fixtures {
		if _, duplicate := seenFixtureNames[fixture.Name]; duplicate {
			return fmt.Errorf("result-policy fixture %q appears more than once", fixture.Name)
		}
		seenFixtureNames[fixture.Name] = struct{}{}
		if strings.TrimSpace(fixture.Name) == "" {
			return fmt.Errorf("result-policy fixture name is required")
		}
		if strings.TrimSpace(fixture.ToolName) == "" {
			return fmt.Errorf("result-policy fixture %q toolName is required", fixture.Name)
		}
		if len(fixture.ToolResponse) == 0 {
			return fmt.Errorf("result-policy fixture %q toolResponse is required", fixture.Name)
		}
		if len(fixture.CallToolResult) == 0 {
			return fmt.Errorf("result-policy fixture %q callToolResult is required", fixture.Name)
		}
		if err := verifySuccessCallToolResultFixture(fixture); err != nil {
			return err
		}
	}
	return nil
}

func verifySuccessCallToolResultFixture(fixture ResultPolicyFixture) error {
	expected, err := MarshalSuccessCallToolResultJSON(fixture.ToolResponse)
	if err != nil {
		return fmt.Errorf("result-policy fixture %q marshal expected callToolResult: %w", fixture.Name, err)
	}
	if string(fixture.CallToolResult) != string(expected) {
		return fmt.Errorf("result-policy fixture %q callToolResult does not match encoded toolResponse", fixture.Name)
	}

	var decoded map[string]any
	if err := json.Unmarshal(fixture.CallToolResult, &decoded); err != nil {
		return fmt.Errorf("result-policy fixture %q callToolResult: %w", fixture.Name, err)
	}
	if decoded["isError"] != false {
		return fmt.Errorf("result-policy fixture %q isError = %#v, want false", fixture.Name, decoded["isError"])
	}
	if _, ok := decoded["structuredContent"]; ok {
		return fmt.Errorf("result-policy fixture %q must not include structuredContent", fixture.Name)
	}
	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 1 {
		return fmt.Errorf("result-policy fixture %q content = %#v, want one item", fixture.Name, decoded["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		return fmt.Errorf("result-policy fixture %q content[0] type = %T, want object", fixture.Name, content[0])
	}
	if item["type"] != "text" {
		return fmt.Errorf("result-policy fixture %q content type = %#v, want text", fixture.Name, item["type"])
	}
	text, ok := item["text"].(string)
	if !ok || strings.TrimSpace(text) == "" {
		return fmt.Errorf("result-policy fixture %q content text is required", fixture.Name)
	}
	if text != string(fixture.ToolResponse) {
		return fmt.Errorf("result-policy fixture %q content text does not match toolResponse", fixture.Name)
	}
	return nil
}

func representativeListSessionsSuccessToolResponse() (json.RawMessage, error) {
	response := ToolResponse[map[string]any]{
		Result: &map[string]any{
			"scope":    "live",
			"sessions": []any{},
		},
	}
	return json.Marshal(response)
}
