package contractvalidator

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/callbehavior"
	jscatalog "github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/catalog"
)

// JavaScriptRuntimeCatalogCallBehaviorParityDiagnostics applies authored-catalog
// representative call-behavior parity checks against the installed baseline.
func JavaScriptRuntimeCatalogCallBehaviorParityDiagnostics(document string, value any) []Diagnostic {
	if document != authoredJavaScriptRuntimeCatalogPath {
		return nil
	}

	root, ok := value.(map[string]any)
	if !ok {
		return []Diagnostic{newDiagnostic(
			"catalog.call_behavior.parse",
			"/symbols",
			"catalog document is not an object",
			document,
		)}
	}

	issues, err := jscatalog.CatalogCallBehaviorParityIssues(root, callbehavior.ProjectInstalledCallBehavior())
	if err != nil {
		return []Diagnostic{newDiagnostic("catalog.call_behavior.parse", "/symbols", err.Error(), document)}
	}
	if len(issues) == 0 {
		return nil
	}

	diagnostics := make([]Diagnostic, 0, len(issues))
	for _, issue := range issues {
		diagnosticPath := "/symbols"
		if issue.SymbolKey != "" {
			diagnosticPath = "/symbols/" + escapeJSONPointerToken(issue.SymbolKey)
			if issue.Field != "" {
				diagnosticPath += "/" + escapeJSONPointerToken(issue.Field)
			}
		}
		diagnostics = append(diagnostics, newDiagnostic(
			issue.Code,
			diagnosticPath,
			issue.Message,
			document,
		))
	}
	return diagnostics
}

func javascriptCatalogCallBehaviorParityDiagnostics(document string, value any) []Diagnostic {
	return JavaScriptRuntimeCatalogCallBehaviorParityDiagnostics(document, value)
}
