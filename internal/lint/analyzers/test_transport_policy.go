package analyzers

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Exact owner operations are forbidden only in transport tests. Detached
// service results remain allowed; compiler identities exclude shadowed names.
var transportTestPolicy = map[string]map[string]string{
	"pkg/services/factory_definitions": {
		"InternalModelProviderFromPublicWorkerModelProvider": "factory_definitions",
		"PublicWorkerModelProviderFromInternal":              "factory_definitions",
	},
	"pkg/services/factory_sessions": {
		"ApplySessionListScope":            "factory_sessions",
		"ProjectFactorySessionStopSummary": "factory_sessions",
		"ProjectWorkStopSummary":           "factory_sessions",
		"ValidateFactoryResponseEvent":     "factory_sessions",
	},
	"pkg/services/provider_sessions": {
		"CanonicalProvider":             "provider_sessions",
		"newTestProviderSessionService": "provider_sessions",
		"scriptedProviderSessionDetail": "provider_sessions",
		"testProviderSessionService":    "provider_sessions",
	},
	"pkg/services/provider_sessions/service": {
		"New":         "provider_sessions",
		"NewForRoots": "provider_sessions",
	},
	"pkg/services/provider_sessions/cursor": {
		"DefaultAgentStorageRoot":   "provider_sessions",
		"LoadDetails":               "provider_sessions",
		"NormalizeAgentStorageRoot": "provider_sessions",
	},
	"pkg/services/factory_runtime": {
		"CategoryForState":        "factory_runtime",
		"CollectPublicWorkTokens": "factory_runtime",
		"SplitPlaceID":            "factory_runtime",
	},
	"pkg/services/models": {
		"SupportedProviders": "models",
	},
	"pkg/services/work": {
		"NormalizeList":          "work",
		"PrepareInvocationInput": "work",
	},
	"pkg/services/workers": {
		"CanonicalProviderSessionProvider": "workers",
	},
	"pkg/transports/mapping": {
		"BuildWorkflowSessionLiveResult":           "factory_runtime",
		"BuildWorkflowSessionResult":               "factory_runtime",
		"BuildWorkflowSessionResultUpdatedPayload": "factory_runtime",
	},
}

func transportTestPolicyViolation(pass *analysis.Pass, id *ast.Ident, obj types.Object, unit string, called bool) *violation {
	if !under(strings.TrimSuffix(unit, "_test"), "pkg/transports") || !testBoundaryCallable(obj, called) {
		return nil
	}
	path := strings.TrimPrefix(obj.Pkg().Path(), modulePrefix)
	owner := transportTestPolicy[path][obj.Name()]
	if owner == "" {
		return nil
	}
	return transportTestPolicyFinding(pass, id, unit, path, owner)
}

func transportTestPolicyFinding(pass *analysis.Pass, id *ast.Ident, unit, path, owner string) *violation {
	return &violation{rule: "test-transport-owner-policy", importer: unit, importee: timingFile(unit, pass.Fset.Position(id.Pos()).Filename) + "#" + path + "." + id.Name, pos: id.Pos(), hint: "move policy assertions to pkg/services/" + owner + " and keep transport tests on injected public roles"}
}

// These legacy local helper declarations implement Provider Session policy
// inside transports. Declaration checks preserve their exact names and scope.
func transportTestPolicyDeclarations(pass *analysis.Pass, file *ast.File, unit string) []violation {
	var found []violation
	path := "pkg/services/provider_sessions"
	record := func(id *ast.Ident) {
		if owner := transportTestPolicy[path][id.Name]; owner != "" {
			found = append(found, *transportTestPolicyFinding(pass, id, unit, path, owner))
		}
	}
	for _, declaration := range file.Decls {
		switch d := declaration.(type) {
		case *ast.FuncDecl:
			record(d.Name)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if typ, ok := spec.(*ast.TypeSpec); ok {
					record(typ.Name)
				}
			}
		}
	}
	return found
}
