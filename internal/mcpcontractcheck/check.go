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
	ID              string
	Name            string
	CanonicalToolID string
}

// RuntimeAliasBinding is one handwritten compatibility route. It remains
// separate from the retained inventory so the checker can detect routing drift.
type RuntimeAliasBinding struct {
	Name          string
	CanonicalName string
}

// Inputs contains every explicit value consumed by the pure parity checker.
type Inputs struct {
	Catalog        []ToolRecord
	Discovery      []ToolRecord
	Registry       []HandlerBinding
	Aliases        []AliasBinding
	RuntimeAliases []RuntimeAliasBinding
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
	aliases, aliasNames, aliasDiagnostics := indexAliases(inputs.Aliases)
	diagnostics = append(diagnostics, aliasDiagnostics...)
	runtimeAliases, runtimeAliasDiagnostics := indexRuntimeAliases(inputs.RuntimeAliases)
	diagnostics = append(diagnostics, runtimeAliasDiagnostics...)
	if len(diagnostics) != 0 {
		return sortDiagnostics(diagnostics)
	}

	for id, expected := range catalog {
		actual, ok := discovery[id]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.discovery.missing", ToolID: id, Surface: "discovery",
				Message: fmt.Sprintf("canonical tool %q is missing from generated primary discovery; regenerate discovery from the authored catalog", id),
			})
		} else {
			diagnostics = append(diagnostics, metadataDiagnostics(expected, actual)...)
		}

		binding, ok := registry[id]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.missing", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("canonical tool %q is missing handler %q from the handwritten registry; register that handler binding", id, expected.HandlerID),
			})
		} else if binding.HandlerID != expected.HandlerID {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.handler_mismatch", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("canonical tool %q binds handler %q; authored catalog expects %q; update the authored catalog handler ID if intent changed, otherwise repair the handwritten registry binding", id, binding.HandlerID, expected.HandlerID),
			})
		}
	}

	for id := range discovery {
		if _, ok := catalog[id]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.discovery.extra", ToolID: id, Surface: "discovery",
				Message: fmt.Sprintf("generated primary discovery contains uncontracted tool %q; regenerate discovery from the authored catalog", id),
			})
		}
	}
	for id := range registry {
		if _, ok := catalog[id]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.extra", ToolID: id, Surface: "registry",
				Message: fmt.Sprintf("handwritten registry contains uncontracted tool %q; remove the binding or add the intended tool to the authored catalog", id),
			})
		}
	}
	diagnostics = append(diagnostics, aliasBoundaryDiagnostics(aliases, aliasNames, runtimeAliases, catalog, discovery)...)

	return sortDiagnostics(diagnostics)
}

func sortDiagnostics(diagnostics []Diagnostic) []Diagnostic {
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
	records = append([]ToolRecord(nil), records...)
	sort.Slice(records, func(i, j int) bool {
		if records[i].ID != records[j].ID {
			return records[i].ID < records[j].ID
		}
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].HandlerID < records[j].HandlerID
	})
	indexed := make(map[string]ToolRecord, len(records))
	names := make(map[string]string, len(records))
	handlerIDs := make(map[string]string, len(records))
	var diagnostics []Diagnostic
	for _, record := range records {
		if _, exists := indexed[record.ID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp." + surface + ".duplicate_tool_id", ToolID: record.ID, Surface: surface,
				Message: fmt.Sprintf("stable tool ID %q appears more than once in %s; assign one unique stable tool ID per canonical record", record.ID, surface),
			})
		} else {
			indexed[record.ID] = record
		}
		if priorID, exists := names[record.Name]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp." + surface + ".duplicate_canonical_name", ToolID: record.ID, Surface: surface,
				Message: fmt.Sprintf("canonical name %q is shared by stable tool IDs %q and %q in %s; assign a unique canonical name", record.Name, priorID, record.ID, surface),
			})
		} else {
			names[record.Name] = record.ID
		}
		if record.HandlerID == "" {
			continue
		}
		if priorID, exists := handlerIDs[record.HandlerID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp." + surface + ".duplicate_handler_id", ToolID: record.ID, Surface: surface,
				Message: fmt.Sprintf("stable handler ID %q is shared by stable tool IDs %q and %q in %s; assign one unique handler ID per canonical tool", record.HandlerID, priorID, record.ID, surface),
			})
		} else {
			handlerIDs[record.HandlerID] = record.ID
		}
	}
	return indexed, diagnostics
}

func indexBindings(bindings []HandlerBinding) (map[string]HandlerBinding, []Diagnostic) {
	bindings = append([]HandlerBinding(nil), bindings...)
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ToolID != bindings[j].ToolID {
			return bindings[i].ToolID < bindings[j].ToolID
		}
		return bindings[i].HandlerID < bindings[j].HandlerID
	})
	indexed := make(map[string]HandlerBinding, len(bindings))
	handlerIDs := make(map[string]string, len(bindings))
	var diagnostics []Diagnostic
	for _, binding := range bindings {
		if _, exists := indexed[binding.ToolID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.duplicate_tool_id", ToolID: binding.ToolID, Surface: "registry",
				Message: fmt.Sprintf("stable tool ID %q appears more than once in registry; keep exactly one handwritten binding per canonical tool", binding.ToolID),
			})
		} else {
			indexed[binding.ToolID] = binding
		}
		if binding.HandlerID == "" {
			continue
		}
		if priorID, exists := handlerIDs[binding.HandlerID]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.registry.duplicate_handler_id", ToolID: binding.ToolID, Surface: "registry",
				Message: fmt.Sprintf("stable handler ID %q is shared by stable tool IDs %q and %q in registry; bind each canonical tool to a unique stable handler ID", binding.HandlerID, priorID, binding.ToolID),
			})
		} else {
			handlerIDs[binding.HandlerID] = binding.ToolID
		}
	}
	return indexed, diagnostics
}

func metadataDiagnostics(expected, actual ToolRecord) []Diagnostic {
	fields := []struct {
		name    string
		equal   bool
		details string
	}{
		{name: "name", equal: expected.Name == actual.Name, details: fmt.Sprintf("got %q, want %q", actual.Name, expected.Name)},
		{name: "description", equal: expected.Description == actual.Description, details: fmt.Sprintf("got %q, want %q", actual.Description, expected.Description)},
		{name: "inputSchema", equal: schemasEqual(expected.InputSchema, actual.InputSchema)},
	}
	var diagnostics []Diagnostic
	for _, field := range fields {
		if field.equal {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code: "mcp.discovery.metadata_mismatch", ToolID: expected.ID, Surface: "discovery",
			Message: fmt.Sprintf("generated primary discovery for %q has stale %s metadata%s; update the authored catalog if intent changed, otherwise regenerate discovery from the authored catalog", expected.ID, field.name, formatMismatchDetails(field.details)),
		})
	}
	return diagnostics
}

func formatMismatchDetails(details string) string {
	if details == "" {
		return ""
	}
	return ": " + details
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
