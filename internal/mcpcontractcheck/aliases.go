package mcpcontractcheck

import (
	"fmt"
	"sort"
)

func indexAliases(bindings []AliasBinding) (map[string]AliasBinding, map[string]AliasBinding, []Diagnostic) {
	bindings = append([]AliasBinding(nil), bindings...)
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].ID != bindings[j].ID {
			return bindings[i].ID < bindings[j].ID
		}
		if bindings[i].Name != bindings[j].Name {
			return bindings[i].Name < bindings[j].Name
		}
		return bindings[i].CanonicalToolID < bindings[j].CanonicalToolID
	})

	byID := make(map[string]AliasBinding, len(bindings))
	byName := make(map[string]AliasBinding, len(bindings))
	var diagnostics []Diagnostic
	for _, binding := range bindings {
		if binding.CanonicalToolID == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.alias.missing_target", ToolID: binding.ID, Surface: "aliases",
				Message: fmt.Sprintf("retained compatibility alias %q (%s) has no canonical stable tool ID; set lifecycle.successor.targetItemId in contracts/mcp/deprecated.json", binding.Name, binding.ID),
			})
		}
		if previous, exists := byID[binding.ID]; exists {
			diagnostics = append(diagnostics, conflictingAliasDiagnostic(binding, previous, "stable alias ID"))
		} else {
			byID[binding.ID] = binding
		}
		if previous, exists := byName[binding.Name]; exists {
			diagnostics = append(diagnostics, conflictingAliasDiagnostic(binding, previous, "compatibility alias name"))
		} else {
			byName[binding.Name] = binding
		}
	}
	return byID, byName, diagnostics
}

func conflictingAliasDiagnostic(binding, previous AliasBinding, identity string) Diagnostic {
	return Diagnostic{
		Code: "mcp.alias.conflicting_mapping", ToolID: binding.ID, Surface: "aliases",
		Message: fmt.Sprintf("%s %q has conflicting retained mappings to canonical stable tool IDs %q and %q; keep one alias inventory mapping", identity, aliasIdentityValue(binding, identity), previous.CanonicalToolID, binding.CanonicalToolID),
	}
}

func aliasIdentityValue(binding AliasBinding, identity string) string {
	if identity == "stable alias ID" {
		return binding.ID
	}
	return binding.Name
}

func indexRuntimeAliases(bindings []RuntimeAliasBinding) (map[string]RuntimeAliasBinding, []Diagnostic) {
	bindings = append([]RuntimeAliasBinding(nil), bindings...)
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Name != bindings[j].Name {
			return bindings[i].Name < bindings[j].Name
		}
		return bindings[i].CanonicalName < bindings[j].CanonicalName
	})

	indexed := make(map[string]RuntimeAliasBinding, len(bindings))
	var diagnostics []Diagnostic
	for _, binding := range bindings {
		if previous, exists := indexed[binding.Name]; exists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.alias.runtime_conflicting_mapping", ToolID: binding.Name, Surface: "runtime-aliases",
				Message: fmt.Sprintf("runtime compatibility alias %q maps to both %q and %q; keep one handwritten compatibility route", binding.Name, previous.CanonicalName, binding.CanonicalName),
			})
		} else {
			indexed[binding.Name] = binding
		}
	}
	return indexed, diagnostics
}

func aliasBoundaryDiagnostics(
	aliasesByID, aliasesByName map[string]AliasBinding,
	runtimeAliases map[string]RuntimeAliasBinding,
	catalog, discovery map[string]ToolRecord,
) []Diagnostic {
	var diagnostics []Diagnostic
	for _, alias := range aliasesByID {
		target, targetExists := catalog[alias.CanonicalToolID]
		if alias.CanonicalToolID != "" && !targetExists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.alias.unknown_target", ToolID: alias.ID, Surface: "aliases",
				Message: fmt.Sprintf("retained compatibility alias %q (%s) targets unknown canonical stable tool ID %q; repair contracts/mcp/deprecated.json", alias.Name, alias.ID, alias.CanonicalToolID),
			})
		}

		runtime, runtimeExists := runtimeAliases[alias.Name]
		if !runtimeExists {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.alias.runtime_missing", ToolID: alias.CanonicalToolID, Surface: "runtime-aliases",
				Message: fmt.Sprintf("retained compatibility alias %q (%s) has no handwritten runtime route to canonical stable tool ID %q; restore the compatibility alias mapping", alias.Name, alias.ID, alias.CanonicalToolID),
			})
		} else if targetExists && runtime.CanonicalName != target.Name {
			diagnostics = append(diagnostics, Diagnostic{
				Code: "mcp.alias.runtime_target_mismatch", ToolID: alias.CanonicalToolID, Surface: "runtime-aliases",
				Message: fmt.Sprintf("retained compatibility alias %q (%s) routes to %q; canonical stable tool ID %q is %q; repair the handwritten compatibility route", alias.Name, alias.ID, runtime.CanonicalName, alias.CanonicalToolID, target.Name),
			})
		}

		diagnostics = append(diagnostics, aliasCanonicalDiagnostics(alias, catalog, "catalog")...)
		diagnostics = append(diagnostics, aliasCanonicalDiagnostics(alias, discovery, "discovery")...)
	}

	for name, runtime := range runtimeAliases {
		if _, exists := aliasesByName[name]; exists {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code: "mcp.alias.runtime_uninventoried", ToolID: name, Surface: "runtime-aliases",
			Message: fmt.Sprintf("runtime compatibility alias %q routes to %q but is absent from contracts/mcp/deprecated.json; inventory the retained alias or remove the unapproved route", name, runtime.CanonicalName),
		})
	}
	return diagnostics
}

func aliasCanonicalDiagnostics(alias AliasBinding, records map[string]ToolRecord, surface string) []Diagnostic {
	var diagnostics []Diagnostic
	for recordID, record := range records {
		if recordID != alias.ID && record.Name != alias.Name {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code: "mcp.alias.canonical", ToolID: alias.CanonicalToolID, Surface: surface,
			Message: fmt.Sprintf("compatibility alias %q (%s) for canonical stable tool ID %q appears as canonical %s record %q; remove the alias from canonical %s and keep it only on the retained compatibility path", alias.Name, alias.ID, alias.CanonicalToolID, surface, recordID, surface),
		})
	}
	return diagnostics
}
