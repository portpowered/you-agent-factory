package functionalscenarios

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/portpowered/infinite-you/internal/contractinventory"
)

const (
	FormatVersion = "functional-scenario-components/v1"

	InterfaceCLI  = "cli"
	InterfaceREST = "rest"
	InterfaceMCP  = "mcp"
	InterfaceSSE  = "sse"

	ClassificationRunnable    = "runnable"
	ClassificationGrouping    = "grouping"
	ClassificationOperation   = "operation"
	ClassificationTool        = "tool"
	ClassificationEventStream = "event-stream"
)

// Projection is the deterministic public-component set used by functional coverage review.
type Projection struct {
	FormatVersion string      `json:"formatVersion"`
	Components    []Component `json:"components"`
}

// Component records one canonical public interface identity.
type Component struct {
	StableID       string `json:"stableId"`
	Interface      string `json:"interface"`
	Identity       string `json:"identity"`
	Name           string `json:"name"`
	Classification string `json:"classification"`
}

type cliInventory struct {
	Commands map[string]cliCommand `json:"commands"`
}

type cliCommand struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Runnable bool   `json:"runnable"`
}

type mcpInventory struct {
	Tools map[string]mcpTool `json:"tools"`
}

type mcpTool struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Project normalizes the canonical CLI, OpenAPI, and MCP inventories. SSE
// components are derived from REST operations that declare text/event-stream.
func Project(cliData, openAPIData, mcpData []byte) (*Projection, error) {
	cliComponents, err := projectCLI(cliData)
	if err != nil {
		return nil, err
	}
	restComponents, sseComponents, err := projectOpenAPI(openAPIData)
	if err != nil {
		return nil, err
	}
	mcpComponents, err := projectMCP(mcpData)
	if err != nil {
		return nil, err
	}

	components := make([]Component, 0, len(cliComponents)+len(restComponents)+len(mcpComponents)+len(sseComponents))
	components = append(components, cliComponents...)
	components = append(components, restComponents...)
	components = append(components, mcpComponents...)
	components = append(components, sseComponents...)
	slices.SortFunc(components, func(left, right Component) int {
		return strings.Compare(left.StableID, right.StableID)
	})
	if err := validateUniqueStableIDs(components); err != nil {
		return nil, err
	}

	return &Projection{FormatVersion: FormatVersion, Components: components}, nil
}

func projectCLI(data []byte) ([]Component, error) {
	inventory := cliInventory{}
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, fmt.Errorf("project %s interface: decode canonical inventory: %w", InterfaceCLI, err)
	}
	components := make([]Component, 0, len(inventory.Commands))
	seen := make(map[string]string, len(inventory.Commands))
	for key, command := range inventory.Commands {
		identity := strings.TrimSpace(command.ID)
		if identity == "" {
			return nil, fmt.Errorf("project %s interface: missing canonical identity for command key %q", InterfaceCLI, key)
		}
		if first, ok := seen[identity]; ok {
			return nil, fmt.Errorf("project %s interface: duplicate canonical identity %q for command keys %q and %q", InterfaceCLI, identity, first, key)
		}
		seen[identity] = key
		name := strings.TrimSpace(command.Path)
		if name == "" {
			return nil, fmt.Errorf("project %s interface: identity %q has no canonical customer-facing path", InterfaceCLI, identity)
		}
		classification := ClassificationGrouping
		if command.Runnable {
			classification = ClassificationRunnable
		}
		components = append(components, newComponent(InterfaceCLI, identity, name, classification))
	}
	return components, nil
}

func projectOpenAPI(data []byte) ([]Component, []Component, error) {
	inventory, err := contractinventory.ExtractFromOpenAPIYAML(data)
	if err != nil {
		return nil, nil, fmt.Errorf("project %s interface: %w", InterfaceREST, err)
	}
	rest := make([]Component, 0, len(inventory.Operations))
	sse := make([]Component, 0)
	for _, operation := range inventory.Operations {
		rest = append(rest, newComponent(InterfaceREST, operation.OperationID, operation.OperationID, ClassificationOperation))
		if operationHasEventStream(operation) {
			sse = append(sse, newComponent(InterfaceSSE, operation.OperationID, operation.OperationID, ClassificationEventStream))
		}
	}
	return rest, sse, nil
}

func projectMCP(data []byte) ([]Component, error) {
	inventory := mcpInventory{}
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, fmt.Errorf("project %s interface: decode canonical inventory: %w", InterfaceMCP, err)
	}
	components := make([]Component, 0, len(inventory.Tools))
	seen := make(map[string]string, len(inventory.Tools))
	for key, tool := range inventory.Tools {
		identity := strings.TrimSpace(tool.ID)
		if identity == "" {
			return nil, fmt.Errorf("project %s interface: missing canonical identity for tool key %q", InterfaceMCP, key)
		}
		if first, ok := seen[identity]; ok {
			return nil, fmt.Errorf("project %s interface: duplicate canonical identity %q for tool keys %q and %q", InterfaceMCP, identity, first, key)
		}
		seen[identity] = key
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return nil, fmt.Errorf("project %s interface: identity %q has no canonical customer-facing name", InterfaceMCP, identity)
		}
		components = append(components, newComponent(InterfaceMCP, identity, name, ClassificationTool))
	}
	return components, nil
}

func newComponent(customerInterface, identity, name, classification string) Component {
	return Component{
		StableID:       customerInterface + "/" + identity,
		Interface:      customerInterface,
		Identity:       identity,
		Name:           name,
		Classification: classification,
	}
}

func operationHasEventStream(operation contractinventory.Operation) bool {
	for _, response := range operation.Responses {
		if slices.Contains(response.MediaTypes, "text/event-stream") {
			return true
		}
	}
	return false
}

func validateUniqueStableIDs(components []Component) error {
	for index := 1; index < len(components); index++ {
		if components[index-1].StableID == components[index].StableID {
			component := components[index]
			return fmt.Errorf("project %s interface: duplicate stable ID %q for canonical identity %q", component.Interface, component.StableID, component.Identity)
		}
	}
	return nil
}
