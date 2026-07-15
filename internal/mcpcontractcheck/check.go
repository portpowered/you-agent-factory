// Package mcpcontractcheck verifies parity between the authored MCP catalog,
// generated primary discovery, and the handwritten stable-ID handler registry.
// Runtime packages must not import this build-time verification package.
package mcpcontractcheck

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// ToolRecord is one canonical MCP tool identity and discovery contract.
type ToolRecord struct {
	ID          string
	Name        string
	Description string
	InputSchema any
	HandlerID   string
}

// HandlerBinding is one non-executable handwritten registry identity.
type HandlerBinding struct {
	ToolID    string
	HandlerID string
}

// AliasBinding is one compatibility-only name mapping supplied separately
// from canonical records.
type AliasBinding struct {
	Name          string
	CanonicalName string
}

// Inputs contains every explicit value consumed by the pure parity checker.
type Inputs struct {
	Catalog   []ToolRecord
	Discovery []ToolRecord
	Registry  []HandlerBinding
	Aliases   []AliasBinding
}

// Diagnostic records one deterministic stable-ID parity failure.
type Diagnostic struct {
	Code    string
	ToolID  string
	Surface string
	Message string
}

// Validate compares explicit boundary inputs without filesystem or runtime
// mutation. Diagnostics are returned in deterministic order.
func Validate(inputs Inputs) []Diagnostic {
	catalog, diagnostics := indexTools(inputs.Catalog, "catalog")
	discovery, discoveryDiagnostics := indexTools(inputs.Discovery, "discovery")
	diagnostics = append(diagnostics, discoveryDiagnostics...)
	registry, registryDiagnostics := indexBindings(inputs.Registry)
	diagnostics = append(diagnostics, registryDiagnostics...)

	for id, expected := range catalog {
		actual, ok := discovery[id]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.discovery.missing", ToolID: id, Surface: "discovery",
				Message: fmt.Sprintf("canonical tool %q is missing from generated primary discovery", id),
			})
		} else {
			diagnostics = append(diagnostics, metadataDiagnostics(expected, actual)...)
		}

		binding, ok := registry[id]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.missing", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("canonical tool %q is missing handler %q from the handwritten registry", id, expected.HandlerID),
			})
		} else if binding.HandlerID != expected.HandlerID {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.handler_mismatch", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("canonical tool %q binds handler %q; authored catalog expects %q", id, binding.HandlerID, expected.HandlerID),
			})
		}
	}

	for id := range discovery {
		if _, ok := catalog[id]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.discovery.extra", ToolID: id, Surface: "discovery",
				Message: fmt.Sprintf("generated primary discovery contains uncontracted tool %q", id),
			})
		}
	}
	for id := range registry {
		if _, ok := catalog[id]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.extra", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("handwritten registry contains uncontracted tool %q", id),
			})
		}
	}
	diagnostics = append(diagnostics, aliasSeparationDiagnostics(inputs.Aliases, catalog, discovery)...)

	sort.Slice(diagnostics, func(i, j int) bool {
		left, right := diagnostics[i], diagnostics[j]
		if left.ToolID != right.ToolID {
			return left.ToolID < right.ToolID
		}
		if left.Surface != right.Surface {
			return left.Surface < right.Surface
		}
		if left.Code != right.Code {
			return left.Code < right.Code
		}
		return left.Message < right.Message
	})
	return diagnostics
}

func indexTools(records []ToolRecord, surface string) (map[string]ToolRecord, []Diagnostic) {
	indexed := make(map[string]ToolRecord, len(records))
	var diagnostics []Diagnostic
	for _, record := range records {
		if _, exists := indexed[record.ID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp." + surface + ".duplicate_tool_id", ToolID: record.ID, Surface: surface,
				Message: fmt.Sprintf("stable tool ID %q appears more than once in %s", record.ID, surface),
			})
			continue
		}
		indexed[record.ID] = record
	}
	return indexed, diagnostics
}

func indexBindings(bindings []HandlerBinding) (map[string]HandlerBinding, []Diagnostic) {
	indexed := make(map[string]HandlerBinding, len(bindings))
	var diagnostics []Diagnostic
	for _, binding := range bindings {
		if _, exists := indexed[binding.ToolID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.duplicate_tool_id", ToolID: binding.ToolID, Surface: "registry",
				Message: fmt.Sprintf("stable tool ID %q appears more than once in registry", binding.ToolID),
			})
			continue
		}
		indexed[binding.ToolID] = binding
	}
	return indexed, diagnostics
}

func metadataDiagnostics(expected, actual ToolRecord) []Diagnostic {
	fields := []struct {
		name  string
		equal bool
	}{
		{name: "name", equal: expected.Name == actual.Name},
		{name: "description", equal: expected.Description == actual.Description},
		{name: "inputSchema", equal: schemasEqual(expected.InputSchema, actual.InputSchema)},
	}
	var diagnostics []Diagnostic
	for _, field := range fields {
		if field.equal {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code: "mcp.discovery.metadata_mismatch", ToolID: expected.ID, Surface: "discovery",
			Message: fmt.Sprintf("generated primary discovery for %q has stale %s metadata", expected.ID, field.name),
		})
	}
	return diagnostics
}

func schemasEqual(left, right any) bool {
	return reflect.DeepEqual(normalizeJSON(left), normalizeJSON(right))
}

func normalizeJSON(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	if json.Unmarshal(encoded, &normalized) != nil {
		return value
	}
	return normalized
}

func aliasSeparationDiagnostics(aliases []AliasBinding, catalog, discovery map[string]ToolRecord) []Diagnostic {
	var diagnostics []Diagnostic
	for _, alias := range aliases {
		for _, records := range []struct {
			name  string
			items map[string]ToolRecord
		}{{"catalog", catalog}, {"discovery", discovery}} {
			for id, record := range records.items {
				if record.Name != alias.Name {
					continue
				}
				diagnostics = append(diagnostics, Diagnostic{
					Code: "mcp.alias.canonical", ToolID: id, Surface: records.name,
					Message: fmt.Sprintf("compatibility alias %q must not appear in canonical %s", alias.Name, records.name),
				})
			}
		}
	}
	return diagnostics
}
