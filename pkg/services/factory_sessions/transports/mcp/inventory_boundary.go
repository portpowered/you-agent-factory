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
	// FailureClassDomain labels typed ToolErrorEnvelope failures carried in tools/call results.
	FailureClassDomain = "domain"
	// FailureClassProtocol labels JSON-RPC protocol errors outside CallToolResult payloads.
	FailureClassProtocol = "protocol"
	// ProtocolTransportJSONRPCError documents top-level JSON-RPC error objects.
	ProtocolTransportJSONRPCError = "json-rpc-error-object"
)

// ResultPolicyInventory is a pure, read-only projection of MCP success and error transport policy.
type ResultPolicyInventory struct {
	FormatVersion          string                       `json:"formatVersion"`
	ProtocolVersion        string                       `json:"protocolVersion"`
	SuccessTransport       SuccessTransportPolicy       `json:"successTransport"`
	DomainErrorTransport   DomainErrorTransportPolicy   `json:"domainErrorTransport"`
	ProtocolErrorTransport ProtocolErrorTransportPolicy `json:"protocolErrorTransport"`
	Fixtures               []ResultPolicyFixture        `json:"fixtures"`
	DomainErrorFixtures    []DomainErrorFixture         `json:"domainErrorFixtures"`
	ProtocolErrorFixtures  []ProtocolErrorFixture       `json:"protocolErrorFixtures"`
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

// DomainErrorTransportPolicy records typed ToolErrorEnvelope transport via tools/call.
type DomainErrorTransportPolicy struct {
	FailureClass         string   `json:"failureClass"`
	ContentItemCount     int      `json:"contentItemCount"`
	ContentTypes         []string `json:"contentTypes"`
	TextEncoding         string   `json:"textEncoding"`
	IsError              bool     `json:"isError"`
	StableEnvelopeFields []string `json:"stableEnvelopeFields"`
}

// ProtocolErrorTransportPolicy records JSON-RPC protocol failures outside tools/call payloads.
type ProtocolErrorTransportPolicy struct {
	FailureClass string `json:"failureClass"`
	Transport    string `json:"transport"`
}

// ResultPolicyFixture records one representative tools/call success encoding.
type ResultPolicyFixture struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	ToolName       string          `json:"toolName"`
	ToolResponse   json.RawMessage `json:"toolResponse"`
	CallToolResult json.RawMessage `json:"callToolResult"`
}

// DomainErrorFixture records one representative typed domain-error tools/call encoding.
type DomainErrorFixture struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	ToolName       string          `json:"toolName"`
	ToolArguments  json.RawMessage `json:"toolArguments"`
	ToolResponse   json.RawMessage `json:"toolResponse"`
	CallToolResult json.RawMessage `json:"callToolResult"`
}

// ProtocolErrorFixture records one representative JSON-RPC protocol error response.
type ProtocolErrorFixture struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	RequestLine     string          `json:"requestLine"`
	JSONRPCResponse json.RawMessage `json:"jsonRpcResponse"`
}

// ProjectResultPolicyInventory builds the reviewed success and error transport policy inventory.
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
		Name:           "list_sessions_live_default",
		Description:    "Representative tools/call success for you.factory_session.list with default live scope.",
		ToolName:       ToolListSessions,
		ToolResponse:   toolResponse,
		CallToolResult: callToolResult,
	}}
	slices.SortFunc(fixtures, func(left, right ResultPolicyFixture) int {
		return strings.Compare(left.Name, right.Name)
	})

	domainErrorFixtures, err := projectDomainErrorFixtures()
	if err != nil {
		return ResultPolicyInventory{}, err
	}
	protocolErrorFixtures, err := projectProtocolErrorFixtures()
	if err != nil {
		return ResultPolicyInventory{}, err
	}

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
		DomainErrorTransport: DomainErrorTransportPolicy{
			FailureClass:         FailureClassDomain,
			ContentItemCount:     1,
			ContentTypes:         []string{"text"},
			TextEncoding:         SuccessTextEncodingSerializedJSON,
			IsError:              false,
			StableEnvelopeFields: append([]string(nil), sharedErrorStableFields...),
		},
		ProtocolErrorTransport: ProtocolErrorTransportPolicy{
			FailureClass: FailureClassProtocol,
			Transport:    ProtocolTransportJSONRPCError,
		},
		Fixtures:              fixtures,
		DomainErrorFixtures:   domainErrorFixtures,
		ProtocolErrorFixtures: protocolErrorFixtures,
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

// VerifyResultPolicyInventory fails when success, domain-error, or protocol-error
// transport policy or fixtures do not match the documented contract.
func VerifyResultPolicyInventory(inventory ResultPolicyInventory) error {
	if err := verifyDomainErrorTransportPolicy(inventory.DomainErrorTransport); err != nil {
		return err
	}
	if err := verifyProtocolErrorTransportPolicy(inventory.ProtocolErrorTransport); err != nil {
		return err
	}
	if err := verifySuccessTransportPolicy(inventory.SuccessTransport); err != nil {
		return err
	}
	if err := verifyResultPolicySuccessFixtures(inventory.Fixtures); err != nil {
		return err
	}
	if err := verifyDomainErrorFixtures(inventory.DomainErrorFixtures); err != nil {
		return err
	}
	if err := verifyProtocolErrorFixtures(inventory.ProtocolErrorFixtures); err != nil {
		return err
	}
	return nil
}

func verifySuccessTransportPolicy(policy SuccessTransportPolicy) error {
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
	return nil
}

func verifyResultPolicySuccessFixtures(fixtures []ResultPolicyFixture) error {
	seenFixtureNames := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
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

func verifyDomainErrorFixtures(fixtures []DomainErrorFixture) error {
	seenDomainFixtureNames := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
		if _, duplicate := seenDomainFixtureNames[fixture.Name]; duplicate {
			return fmt.Errorf("domain-error fixture %q appears more than once", fixture.Name)
		}
		seenDomainFixtureNames[fixture.Name] = struct{}{}
		if err := verifyDomainErrorFixture(fixture); err != nil {
			return err
		}
	}
	return nil
}

func verifyProtocolErrorFixtures(fixtures []ProtocolErrorFixture) error {
	seenProtocolFixtureNames := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
		if _, duplicate := seenProtocolFixtureNames[fixture.Name]; duplicate {
			return fmt.Errorf("protocol-error fixture %q appears more than once", fixture.Name)
		}
		seenProtocolFixtureNames[fixture.Name] = struct{}{}
		if err := verifyProtocolErrorFixture(fixture); err != nil {
			return err
		}
	}
	return nil
}

func verifyDomainErrorTransportPolicy(policy DomainErrorTransportPolicy) error {
	if policy.FailureClass != FailureClassDomain {
		return fmt.Errorf("domain error transport failureClass = %q, want %q", policy.FailureClass, FailureClassDomain)
	}
	if policy.ContentItemCount != 1 {
		return fmt.Errorf("domain error transport contentItemCount = %d, want 1", policy.ContentItemCount)
	}
	if len(policy.ContentTypes) != 1 || policy.ContentTypes[0] != "text" {
		return fmt.Errorf("domain error transport contentTypes = %#v, want [text]", policy.ContentTypes)
	}
	if policy.TextEncoding != SuccessTextEncodingSerializedJSON {
		return fmt.Errorf("domain error transport textEncoding = %q, want %q", policy.TextEncoding, SuccessTextEncodingSerializedJSON)
	}
	if policy.IsError {
		return fmt.Errorf("domain error transport isError = true, want false for typed ToolErrorEnvelope payloads")
	}
	if !slices.Equal(policy.StableEnvelopeFields, sharedErrorStableFields) {
		return fmt.Errorf("domain error transport stableEnvelopeFields = %#v, want %#v", policy.StableEnvelopeFields, sharedErrorStableFields)
	}
	return nil
}

func verifyProtocolErrorTransportPolicy(policy ProtocolErrorTransportPolicy) error {
	if policy.FailureClass != FailureClassProtocol {
		return fmt.Errorf("protocol error transport failureClass = %q, want %q", policy.FailureClass, FailureClassProtocol)
	}
	if policy.Transport != ProtocolTransportJSONRPCError {
		return fmt.Errorf("protocol error transport transport = %q, want %q", policy.Transport, ProtocolTransportJSONRPCError)
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
	if isError, present := decoded["isError"]; present && isError != false {
		return fmt.Errorf("result-policy fixture %q isError = %#v, want false or omitted", fixture.Name, isError)
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

func projectDomainErrorFixtures() ([]DomainErrorFixture, error) {
	toolArguments := json.RawMessage(`{"sessionId":"dur-sess-missing-999"}`)
	toolResponse, err := representativeSessionNotFoundDomainErrorToolResponse()
	if err != nil {
		return nil, err
	}
	callToolResult, err := MarshalSuccessCallToolResultJSON(toolResponse)
	if err != nil {
		return nil, err
	}
	fixtures := []DomainErrorFixture{{
		Name:           "get_session_not_found",
		Description:    "Representative typed domain error for you.factory_session.get with a missing session id.",
		ToolName:       ToolGetSession,
		ToolArguments:  toolArguments,
		ToolResponse:   toolResponse,
		CallToolResult: callToolResult,
	}}
	slices.SortFunc(fixtures, func(left, right DomainErrorFixture) int {
		return strings.Compare(left.Name, right.Name)
	})
	return fixtures, nil
}

func projectProtocolErrorFixtures() ([]ProtocolErrorFixture, error) {
	fixtures := []ProtocolErrorFixture{
		{
			Name:        "tools_call_missing_tool_name",
			Description: "SDK invalid-params JSON-RPC error for tools/call with a missing tool name.",
			RequestLine: `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`,
			JSONRPCResponse: json.RawMessage(
				`{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"unknown tool \"\""}}`,
			),
		},
		{
			Name:        "unknown_method",
			Description: "SDK JSON-RPC error for an unsupported MCP method.",
			RequestLine: `{"jsonrpc":"2.0","id":3,"method":"nope"}`,
			JSONRPCResponse: json.RawMessage(
				`{"jsonrpc":"2.0","id":3,"error":{"code":0,"message":"JSON RPC not handled: \"nope\" unsupported"}}`,
			),
		},
	}
	slices.SortFunc(fixtures, func(left, right ProtocolErrorFixture) int {
		return strings.Compare(left.Name, right.Name)
	})
	return fixtures, nil
}

func representativeSessionNotFoundDomainErrorToolResponse() (json.RawMessage, error) {
	envelope := sessionNotFoundErrorEnvelope("dur-sess-missing-999")
	response := ToolResponse[map[string]any]{Error: &envelope}
	return json.Marshal(response)
}

func verifyDomainErrorFixture(fixture DomainErrorFixture) error {
	if strings.TrimSpace(fixture.Name) == "" {
		return fmt.Errorf("domain-error fixture name is required")
	}
	if strings.TrimSpace(fixture.ToolName) == "" {
		return fmt.Errorf("domain-error fixture %q toolName is required", fixture.Name)
	}
	if len(fixture.ToolArguments) == 0 {
		return fmt.Errorf("domain-error fixture %q toolArguments is required", fixture.Name)
	}
	if len(fixture.ToolResponse) == 0 {
		return fmt.Errorf("domain-error fixture %q toolResponse is required", fixture.Name)
	}
	if len(fixture.CallToolResult) == 0 {
		return fmt.Errorf("domain-error fixture %q callToolResult is required", fixture.Name)
	}

	var toolResponse struct {
		Error *ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal(fixture.ToolResponse, &toolResponse); err != nil {
		return fmt.Errorf("domain-error fixture %q toolResponse: %w", fixture.Name, err)
	}
	if toolResponse.Error == nil {
		return fmt.Errorf("domain-error fixture %q toolResponse must include error envelope", fixture.Name)
	}
	if strings.TrimSpace(toolResponse.Error.Code) == "" {
		return fmt.Errorf("domain-error fixture %q error.code is required", fixture.Name)
	}
	if strings.TrimSpace(toolResponse.Error.Message) == "" {
		return fmt.Errorf("domain-error fixture %q error.message is required", fixture.Name)
	}

	expected, err := MarshalSuccessCallToolResultJSON(fixture.ToolResponse)
	if err != nil {
		return fmt.Errorf("domain-error fixture %q marshal expected callToolResult: %w", fixture.Name, err)
	}
	if string(fixture.CallToolResult) != string(expected) {
		return fmt.Errorf("domain-error fixture %q callToolResult does not match encoded toolResponse", fixture.Name)
	}

	var decoded map[string]any
	if err := json.Unmarshal(fixture.CallToolResult, &decoded); err != nil {
		return fmt.Errorf("domain-error fixture %q callToolResult: %w", fixture.Name, err)
	}
	if isError, present := decoded["isError"]; present && isError != false {
		return fmt.Errorf("domain-error fixture %q isError = %#v, want false or omitted", fixture.Name, isError)
	}
	return nil
}

func verifyProtocolErrorFixture(fixture ProtocolErrorFixture) error {
	if strings.TrimSpace(fixture.Name) == "" {
		return fmt.Errorf("protocol-error fixture name is required")
	}
	if strings.TrimSpace(fixture.RequestLine) == "" {
		return fmt.Errorf("protocol-error fixture %q requestLine is required", fixture.Name)
	}
	if len(fixture.JSONRPCResponse) == 0 {
		return fmt.Errorf("protocol-error fixture %q jsonRpcResponse is required", fixture.Name)
	}

	var response struct {
		JSONRPC string                 `json:"jsonrpc"`
		Error   *jsonRPCErrorInventory `json:"error"`
		Result  any                    `json:"result"`
	}
	if err := json.Unmarshal(fixture.JSONRPCResponse, &response); err != nil {
		return fmt.Errorf("protocol-error fixture %q jsonRpcResponse: %w", fixture.Name, err)
	}
	if response.JSONRPC != "2.0" {
		return fmt.Errorf("protocol-error fixture %q jsonrpc = %q, want 2.0", fixture.Name, response.JSONRPC)
	}
	if response.Error == nil {
		return fmt.Errorf("protocol-error fixture %q jsonRpcResponse must include error object", fixture.Name)
	}
	if response.Result != nil {
		return fmt.Errorf("protocol-error fixture %q jsonRpcResponse must not include result", fixture.Name)
	}
	if response.Error.Code != 0 && response.Error.Code != -32601 && response.Error.Code != -32602 {
		return fmt.Errorf("protocol-error fixture %q error.code = %d, want SDK generic, method-not-found, or invalid-params code", fixture.Name, response.Error.Code)
	}
	if strings.TrimSpace(response.Error.Message) == "" {
		return fmt.Errorf("protocol-error fixture %q error.message is required", fixture.Name)
	}
	return nil
}

type jsonRPCErrorInventory struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// VerifyProjectedMCPBoundaryInventories verifies the result-policy inventory.
func VerifyProjectedMCPBoundaryInventories() error {
	return VerifyProjectedResultPolicyInventory()
}
