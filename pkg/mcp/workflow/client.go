package workflow

import (
	"encoding/json"
	"fmt"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

// CallTool invokes one discovered workflow preview MCP tool by stable name.
func CallTool(name string, input json.RawMessage) (json.RawMessage, error) {
	switch name {
	case mcpfactorysession.ToolValidateSource:
		return callPreviewTool(input, ValidateTool)
	case mcpfactorysession.ToolStartPreview:
		return callPreviewTool(input, StartTool)
	default:
		return nil, fmt.Errorf("unsupported tool %q", name)
	}
}

func callPreviewTool(
	input json.RawMessage,
	handler func(factoryapi.FactoryPreviewRequest) (factoryapi.FactoryPreviewResult, error),
) (json.RawMessage, error) {
	var request factoryapi.FactoryPreviewRequest
	if err := json.Unmarshal(input, &request); err != nil {
		return nil, fmt.Errorf("decode preview tool input: %w", err)
	}
	response := mcpfactorysession.PreviewToolResponse(handler(request))
	return json.Marshal(response)
}
