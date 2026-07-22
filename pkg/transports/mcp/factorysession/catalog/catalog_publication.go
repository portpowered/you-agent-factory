package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// AuthoredCatalogFormatVersion is the reviewed MCP tool-catalog document version.
	AuthoredCatalogFormatVersion = "1.0.0"
	// CatalogStagingInventoryFormatVersion is the reviewed identity-inventory version
	// staged to packages/api/generated/mcp/tools.json.
	CatalogStagingInventoryFormatVersion = "1"

	catalogTransportStdioJSONRPC  = "stdio-json-rpc"
	catalogExecutionModeToolsCall = "tools-call"
)

var (
	catalogForbiddenTransports = []string{
		"http",
		"http-json-rpc",
		"sse",
		"sse-json-rpc",
	}
	catalogForbiddenContentTypes = []string{"image", "audio", "resource"}
)

// MarshalCatalogDocumentJSON encodes one resolved MCP tool-catalog document with
// stable map key order, two-space indentation, and exactly one trailing newline.
func MarshalCatalogDocumentJSON(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal catalog document: %w", err)
	}
	var buffer bytes.Buffer
	buffer.Grow(len(payload) + 1)
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}

// VerifyCatalogByteStability ensures repeated canonical serialization is identical.
func VerifyCatalogByteStability(value any) error {
	first, err := MarshalCatalogDocumentJSON(value)
	if err != nil {
		return err
	}
	second, err := MarshalCatalogDocumentJSON(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(first, second) {
		return fmt.Errorf("catalog canonical serialization is not byte-stable across repeated projection")
	}
	return nil
}

// VerifyCatalogAliasExclusion rejects compatibility workflow-named tools from the
// authored catalog. Aliases remain only in contracts/mcp/deprecated.json.
func VerifyCatalogAliasExclusion(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog document is not an object")
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return fmt.Errorf("catalog document missing tools")
	}
	tools, ok := toolsValue.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog tools is not an object")
	}
	for key, raw := range tools {
		record, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("catalog tool %q is not an object", key)
		}
		if strings.HasPrefix(key, "mcp.tool.you.workflow.") {
			return fmt.Errorf("compatibility alias tool id %q must not appear in canonical catalog", key)
		}
		name, _ := record["name"].(string)
		if strings.HasPrefix(name, "you.workflow.") {
			return fmt.Errorf("compatibility alias %q must not appear in canonical catalog", name)
		}
		if id, _ := record["id"].(string); strings.HasPrefix(id, "mcp.tool.you.workflow.") {
			return fmt.Errorf("compatibility alias tool id %q must not appear in canonical catalog", id)
		}
	}
	return nil
}

// VerifyCatalogModalityPolicy ensures the authored catalog advertises only the
// existing text-only success/error modality and stdio JSON-RPC transport.
func VerifyCatalogModalityPolicy(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog document is not an object")
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return fmt.Errorf("catalog document missing tools")
	}
	tools, ok := toolsValue.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog tools is not an object")
	}
	for key, raw := range tools {
		record, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("catalog tool %q is not an object", key)
		}
		name, _ := record["name"].(string)
		if name == "" {
			name = key
		}
		if executionValue, ok := record["execution"].(map[string]any); ok {
			mode, _ := executionValue["mode"].(string)
			if mode != catalogExecutionModeToolsCall {
				return fmt.Errorf("catalog tool %q execution.mode = %q, want %q", name, mode, catalogExecutionModeToolsCall)
			}
		}
		if err := verifyCatalogTransports(record["transports"], name); err != nil {
			return err
		}
		resultValue, ok := record["result"].(map[string]any)
		if !ok {
			return fmt.Errorf("catalog tool %q missing result", name)
		}
		if err := verifyCatalogResultTransport(resultValue["transport"], name); err != nil {
			return err
		}
		if err := verifyCatalogCallToolResultExamples(resultValue["examples"], name); err != nil {
			return err
		}
	}
	return nil
}

// VerifyAuthoredCatalogStagingBoundary ensures the authored catalog document is
// not the staged identity-inventory projection published to packages/api/generated/mcp/tools.json.
func VerifyAuthoredCatalogStagingBoundary(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog document is not an object")
	}
	formatVersion, _ := root["formatVersion"].(string)
	if formatVersion != AuthoredCatalogFormatVersion {
		if formatVersion == CatalogStagingInventoryFormatVersion {
			return fmt.Errorf("catalog formatVersion = %q matches staged inventory version; authored catalog must remain distinct from packages/api/generated/mcp/tools.json", formatVersion)
		}
		return fmt.Errorf("catalog formatVersion = %q, want authored catalog %q", formatVersion, AuthoredCatalogFormatVersion)
	}
	toolsValue, ok := root["tools"]
	if !ok {
		return fmt.Errorf("catalog document missing tools")
	}
	if _, isArray := toolsValue.([]any); isArray {
		return fmt.Errorf("catalog tools must be an object keyed by stable tool id, not the staged inventory array shape")
	}
	if _, isObject := toolsValue.(map[string]any); !isObject {
		return fmt.Errorf("catalog tools is not an object")
	}
	if _, ok := root["sharedSchemas"]; !ok {
		return fmt.Errorf("authored catalog must include sharedSchemas and must not be copied from staged inventory output")
	}
	return nil
}

func verifyCatalogTransports(value any, toolName string) error {
	transports, err := stringSliceValue(value)
	if err != nil {
		return fmt.Errorf("catalog tool %q transports: %w", toolName, err)
	}
	if len(transports) != 1 || transports[0] != catalogTransportStdioJSONRPC {
		return fmt.Errorf("catalog tool %q transports = %#v, want [%q] only", toolName, transports, catalogTransportStdioJSONRPC)
	}
	for _, transport := range transports {
		for _, forbidden := range catalogForbiddenTransports {
			if strings.EqualFold(transport, forbidden) || strings.Contains(strings.ToLower(transport), forbidden) {
				return fmt.Errorf("catalog tool %q advertises unsupported transport %q", toolName, transport)
			}
		}
	}
	return nil
}

func verifyCatalogResultTransport(value any, toolName string) error {
	transport, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("catalog tool %q result.transport is not an object", toolName)
	}
	contentTypes, err := stringSliceValue(transport["contentTypes"])
	if err != nil {
		return fmt.Errorf("catalog tool %q result.transport.contentTypes: %w", toolName, err)
	}
	if len(contentTypes) != 1 || contentTypes[0] != "text" {
		return fmt.Errorf("catalog tool %q result.transport.contentTypes = %#v, want [text]", toolName, contentTypes)
	}
	for _, forbidden := range catalogForbiddenContentTypes {
		if slices.Contains(contentTypes, forbidden) {
			return fmt.Errorf("catalog tool %q advertises unsupported content type %q", toolName, forbidden)
		}
	}
	if allowsStructured, ok := transport["allowsStructuredContent"].(bool); ok && allowsStructured {
		return fmt.Errorf("catalog tool %q result.transport.allowsStructuredContent = true, want false", toolName)
	}
	if allowsOutputSchema, ok := transport["allowsOutputSchema"].(bool); ok && allowsOutputSchema {
		return fmt.Errorf("catalog tool %q result.transport.allowsOutputSchema = true, want false", toolName)
	}
	textEncoding, _ := transport["textEncoding"].(string)
	if textEncoding != "serialized-json" && textEncoding != "plain-text" {
		return fmt.Errorf("catalog tool %q result.transport.textEncoding = %q, want serialized-json or plain-text", toolName, textEncoding)
	}
	return nil
}

func verifyCatalogCallToolResultExamples(value any, toolName string) error {
	examples, ok := value.([]any)
	if !ok {
		return fmt.Errorf("catalog tool %q result.examples is not an array", toolName)
	}
	for index, raw := range examples {
		example, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("catalog tool %q result.examples[%d] is not an object", toolName, index)
		}
		if _, ok := example["structuredContent"]; ok {
			return fmt.Errorf("catalog tool %q result.examples[%d] must not include structuredContent", toolName, index)
		}
		if _, ok := example["outputSchema"]; ok {
			return fmt.Errorf("catalog tool %q result.examples[%d] must not include outputSchema", toolName, index)
		}
		content, ok := example["content"].([]any)
		if !ok || len(content) == 0 {
			return fmt.Errorf("catalog tool %q result.examples[%d] must include text content items", toolName, index)
		}
		for itemIndex, rawItem := range content {
			item, ok := rawItem.(map[string]any)
			if !ok {
				return fmt.Errorf("catalog tool %q result.examples[%d].content[%d] is not an object", toolName, index, itemIndex)
			}
			contentType, _ := item["type"].(string)
			if contentType != "text" {
				return fmt.Errorf("catalog tool %q result.examples[%d].content[%d].type = %q, want text", toolName, index, itemIndex, contentType)
			}
			for _, forbidden := range catalogForbiddenContentTypes {
				if contentType == forbidden {
					return fmt.Errorf("catalog tool %q result.examples[%d] advertises unsupported content type %q", toolName, index, forbidden)
				}
			}
		}
	}
	return nil
}

func stringSliceValue(value any) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for index, item := range typed {
			asString, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("index %d is %T, want string", index, item)
			}
			out = append(out, asString)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("is %T, want array", value)
	}
}
