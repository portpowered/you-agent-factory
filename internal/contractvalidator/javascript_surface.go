package contractvalidator

import (
	"fmt"
	"strconv"
	"strings"
)

var forbiddenInstalledRootGlobals = []string{
	"context",
	"orchestrator",
}

var comparisonProjectHelperPaths = map[string]struct{}{
	"workflow.sleep": {},
	"agent.verify":   {},
	"agent.parallel": {},
}

func runtimeManifestSupportedSurfaceDiagnostics(document string, keys []string, pathByKey map[string]string, symbolKindByKey map[string]string) []Diagnostic {
	var diagnostics []Diagnostic

	for _, key := range keys {
		path := pathByKey[key]
		if path == "" {
			continue
		}
		symbolPath := "/symbols/" + escapeJSONPointerToken(key) + "/path"
		kind := symbolKindByKey[key]

		if isForbiddenInstalledRootGlobalPath(path) {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.forbidden_global",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a forbidden host-only global", strconv.Quote(path)),
				document,
			))
			continue
		}
		if isComparisonProjectHelperPath(path) {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.surface.unsupported_helper",
				symbolPath,
				fmt.Sprintf("symbol path %s documents a comparison-project-only helper that is not part of the installed supported surface", strconv.Quote(path)),
				document,
			))
			continue
		}
		if path == "agent" && kind == "function" {
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

func isForbiddenInstalledRootGlobalPath(path string) bool {
	for _, forbidden := range forbiddenInstalledRootGlobals {
		if path == forbidden || strings.HasPrefix(path, forbidden+".") {
			return true
		}
	}
	return false
}

func isComparisonProjectHelperPath(path string) bool {
	_, ok := comparisonProjectHelperPaths[path]
	return ok
}
