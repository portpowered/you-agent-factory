package contractvalidator

import (
	"fmt"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/tooling/javascript/symbolidentity"
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

		switch symbolidentity.ClassifySurface(path, kind) {
		case symbolidentity.SurfaceForbiddenHostGlobal:
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.forbidden_global",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a forbidden host-only global", strconv.Quote(path)),
				document,
			))
		case symbolidentity.SurfaceComparisonProjectHelper:
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.unsupported_helper",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a comparison-project-only helper that is not part of the installed supported surface", strconv.Quote(path)),
				document,
			))
		case symbolidentity.SurfaceCallableAgentGlobal:
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
