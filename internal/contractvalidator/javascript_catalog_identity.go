package contractvalidator

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const authoredJavaScriptRuntimeCatalogPath = "contracts/javascript/runtime-api.json"

// JavaScriptRuntimeCatalogPathCompletenessDiagnostics applies authored-catalog path
// completeness checks against the identity baseline and installed binding descriptor.
func JavaScriptRuntimeCatalogPathCompletenessDiagnostics(document string, value any) []Diagnostic {
	if document != authoredJavaScriptRuntimeCatalogPath {
		return nil
	}

	paths, err := factoryruntime.JavaScriptCatalogSymbolPathsFromDocument(value)
	if err != nil {
		return []Diagnostic{newDiagnostic("catalog.path.parse", "/symbols", err.Error(), document)}
	}

	issues := factoryruntime.JavaScriptCatalogPathCompletenessIssues(
		paths,
		factoryruntime.JavaScriptProjectInstalledBindings(),
		factoryruntime.JavaScriptProjectInstalledCallBehavior(),
	)
	if len(issues) == 0 {
		return nil
	}

	diagnostics := make([]Diagnostic, 0, len(issues))
	for _, issue := range issues {
		diagnosticPath := "/symbols"
		switch issue.Code {
		case "javascript.path.extra", "javascript.path.duplicate":
			diagnosticPath = "/symbols/" + escapeJSONPointerToken(issue.SymbolKey) + "/path"
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

func javascriptCatalogPathCompletenessDiagnostics(document string, value any) []Diagnostic {
	return JavaScriptRuntimeCatalogPathCompletenessDiagnostics(document, value)
}
