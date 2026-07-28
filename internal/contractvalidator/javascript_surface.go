package contractvalidator

import (
	"fmt"
	"strconv"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func runtimeManifestSupportedSurfaceDiagnostics(document string, keys []string, pathByKey map[string]string, symbolKindByKey map[string]string) []Diagnostic {
	var diagnostics []Diagnostic

	for _, key := range keys {
		path := pathByKey[key]
		if path == "" {
			continue
		}
		symbolPath := "/symbols/" + escapeJSONPointerToken(key) + "/path"
		kind := symbolKindByKey[key]

		switch factoryruntime.JavaScriptClassifySurface(path, kind) {
		case factoryruntime.JavaScriptSurfaceForbiddenHostGlobal:
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.forbidden_global",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a forbidden host-only global", strconv.Quote(path)),
				document,
			))
		case factoryruntime.JavaScriptSurfaceComparisonProjectHelper:
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.unsupported_helper",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a comparison-project-only helper that is not part of the installed supported surface", strconv.Quote(path)),
				document,
			))
		case factoryruntime.JavaScriptSurfaceCallableAgentGlobal:
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.unsupported_helper",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a comparison-project-only callable agent global; installed runtime exposes agent as a namespace with agent.run", strconv.Quote(path)),
				document,
			))
		}
	}

	return diagnostics
}
