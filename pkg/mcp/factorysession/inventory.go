package factorysession

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	// ToolInventoryFormatVersion is the reviewed MCP tool identity baseline version.
	ToolInventoryFormatVersion = "1"
	// ToolInventoryProtocolVersion pins the MCP protocol revision for inventory docs.
	ToolInventoryProtocolVersion = "2024-11-05"
	// ToolInventoryBaselineRelativePath is the reviewed canonical MCP tool inventory fixture.
	ToolInventoryBaselineRelativePath = "contracts/testdata/baseline/mcp-tools.json"
)

// ToolInventory is a pure, read-only projection of canonical MCP tool identities.
type ToolInventory struct {
	FormatVersion   string               `json:"formatVersion"`
	ProtocolVersion string               `json:"protocolVersion"`
	Tools           []ToolInventoryEntry `json:"tools"`
}

// ToolInventoryEntry records one canonical tool identity without result contracts.
type ToolInventoryEntry struct {
	IDCandidate       string         `json:"idCandidate"`
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	InputSchema       map[string]any `json:"inputSchema"`
	HandlerRegistered bool           `json:"handlerRegistered"`
}

// ProjectToolInventory builds a sorted inventory document over DiscoverTools.
// Compatibility aliases are excluded. Input schemas are deep-copied and
// recursively key-canonicalized so serialization order stays stable.
func ProjectToolInventory() (ToolInventory, error) {
	return ProjectToolInventoryFromDiscovered(DiscoverTools())
}

// ProjectToolInventoryFromDiscovered builds one inventory document from an
// explicit canonical discovery slice. Production callers should use
// ProjectToolInventory.
func ProjectToolInventoryFromDiscovered(discovered []ToolDefinition) (ToolInventory, error) {
	entries := make([]ToolInventoryEntry, 0, len(discovered))
	for _, tool := range discovered {
		inputSchema, err := canonicalizeInputSchema(tool.InputSchema)
		if err != nil {
			return ToolInventory{}, err
		}
		entries = append(entries, ToolInventoryEntry{
			IDCandidate:       deriveToolIDCandidate(tool.Name),
			Name:              tool.Name,
			Description:       tool.Description,
			InputSchema:       inputSchema,
			HandlerRegistered: IsCanonicalToolHandlerRegistered(tool.Name),
		})
	}
	slices.SortFunc(entries, func(left, right ToolInventoryEntry) int {
		return strings.Compare(left.Name, right.Name)
	})
	return ToolInventory{
		FormatVersion:   ToolInventoryFormatVersion,
		ProtocolVersion: ToolInventoryProtocolVersion,
		Tools:           entries,
	}, nil
}

// MarshalToolInventoryJSON encodes one inventory document with stable map key order.
func MarshalToolInventoryJSON(inventory ToolInventory) ([]byte, error) {
	return json.Marshal(inventory)
}

// VerifyProjectedToolInventory projects the canonical inventory and fails when
// any discovered tool lacks a registered handler or a compatibility alias
// appears as a canonical inventory entry.
func VerifyProjectedToolInventory() error {
	inventory, err := ProjectToolInventory()
	if err != nil {
		return err
	}
	return VerifyToolInventory(inventory)
}

// VerifyToolInventory fails when any inventoried canonical tool lacks handler
// registration evidence or when a compatibility alias name appears even if the
// alias resolves to a live handler.
func VerifyToolInventory(inventory ToolInventory) error {
	aliasNames := compatibilityAliasNameSet()
	for _, tool := range inventory.Tools {
		if _, isAlias := aliasNames[tool.Name]; isAlias {
			return fmt.Errorf("compatibility alias %q must not appear in canonical inventory", tool.Name)
		}
		if !tool.HandlerRegistered || !IsCanonicalToolHandlerRegistered(tool.Name) {
			return fmt.Errorf("discovered canonical tool %q has no registered handler", tool.Name)
		}
	}
	return nil
}

func compatibilityAliasNameSet() map[string]struct{} {
	aliases := DiscoverCompatibilityAliases()
	names := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		names[alias.Name] = struct{}{}
	}
	return names
}

func deriveToolIDCandidate(name string) string {
	candidate := strings.TrimPrefix(name, "you.")
	return strings.ReplaceAll(candidate, "_", "-")
}

func canonicalizeInputSchema(schema map[string]any) (map[string]any, error) {
	if len(schema) == 0 {
		return nil, nil
	}
	canonical, err := canonicalizeSchemaValue(schema)
	if err != nil {
		return nil, err
	}
	out, ok := canonical.(map[string]any)
	if !ok {
		return nil, errInvalidInputSchemaType
	}
	return out, nil
}

var errInvalidInputSchemaType = &schemaCanonicalizationError{message: "input schema must be an object"}

type schemaCanonicalizationError struct {
	message string
}

func (e *schemaCanonicalizationError) Error() string {
	return e.message
}

func canonicalizeSchemaValue(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		out := make(map[string]any, len(typed))
		for _, key := range keys {
			canonical, err := canonicalizeSchemaValue(typed[key])
			if err != nil {
				return nil, err
			}
			out[key] = canonical
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			canonical, err := canonicalizeSchemaValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, canonical)
		}
		return out, nil
	default:
		return typed, nil
	}
}
