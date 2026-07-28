package factory

import (
	jscallbehavior "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/callbehavior"
	jscatalog "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/catalog"
	jssymbolidentity "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/tooling/javascript/symbolidentity"
)

// JavaScript catalog contract seams expose orchestration-internal tooling for
// repository contract validation without making those packages peer-importable.

type (
	JavaScriptCatalogSymbolPath              = jscatalog.CatalogSymbolPath
	JavaScriptCatalogPathCompletenessIssue   = jscatalog.PathCompletenessIssue
	JavaScriptCatalogCallBehaviorParityIssue = jscatalog.CallBehaviorParityIssue
	JavaScriptSymbolIdentityInventory        = jssymbolidentity.Inventory
	JavaScriptCallBehaviorInventory          = jscallbehavior.Inventory
	JavaScriptSurfaceClassification          = jssymbolidentity.SurfaceClassification
)

const (
	JavaScriptSurfaceSupported               = jssymbolidentity.SurfaceSupported
	JavaScriptSurfaceForbiddenHostGlobal     = jssymbolidentity.SurfaceForbiddenHostGlobal
	JavaScriptSurfaceComparisonProjectHelper = jssymbolidentity.SurfaceComparisonProjectHelper
	JavaScriptSurfaceCallableAgentGlobal     = jssymbolidentity.SurfaceCallableAgentGlobal
)

// JavaScriptCatalogSymbolPathsFromDocument extracts symbol paths from one
// resolved runtime manifest catalog document value.
func JavaScriptCatalogSymbolPathsFromDocument(value any) ([]JavaScriptCatalogSymbolPath, error) {
	return jscatalog.CatalogSymbolPathsFromDocument(value)
}

// JavaScriptCatalogPathCompletenessIssues compares catalog symbol paths to the
// reviewed identity baseline and installed call-behavior descriptor.
func JavaScriptCatalogPathCompletenessIssues(
	catalog []JavaScriptCatalogSymbolPath,
	identity JavaScriptSymbolIdentityInventory,
	callInventory JavaScriptCallBehaviorInventory,
) []JavaScriptCatalogPathCompletenessIssue {
	return jscatalog.CatalogPathCompletenessIssues(catalog, identity, callInventory)
}

// JavaScriptCatalogForbiddenSymbolIssues reports forbidden or unsupported
// catalog symbol paths.
func JavaScriptCatalogForbiddenSymbolIssues(
	catalog []JavaScriptCatalogSymbolPath,
	symbols map[string]any,
) []JavaScriptCatalogPathCompletenessIssue {
	return jscatalog.CatalogForbiddenSymbolIssues(catalog, symbols)
}

// JavaScriptCatalogCallBehaviorParityIssues compares representative catalog
// symbol call metadata against the installed call-behavior baseline.
func JavaScriptCatalogCallBehaviorParityIssues(
	document map[string]any,
	callInventory JavaScriptCallBehaviorInventory,
) ([]JavaScriptCatalogCallBehaviorParityIssue, error) {
	return jscatalog.CatalogCallBehaviorParityIssues(document, callInventory)
}

// JavaScriptProjectInstalledBindings builds the installed symbol-identity
// inventory for the JavaScript runtime surface.
func JavaScriptProjectInstalledBindings() JavaScriptSymbolIdentityInventory {
	return jssymbolidentity.ProjectInstalledBindings()
}

// JavaScriptVerifyProjectedInstalledBindings verifies the installed binding
// descriptor matches the authored catalog projection.
func JavaScriptVerifyProjectedInstalledBindings() error {
	return jssymbolidentity.VerifyProjectedInstalledBindings()
}

// JavaScriptProjectInstalledCallBehavior builds the installed call-behavior
// inventory for the JavaScript runtime surface.
func JavaScriptProjectInstalledCallBehavior() JavaScriptCallBehaviorInventory {
	return jscallbehavior.ProjectInstalledCallBehavior()
}

// JavaScriptClassifySurface centralizes forbidden and comparison-project-only
// symbol policy for authored catalog validation.
func JavaScriptClassifySurface(path, kind string) JavaScriptSurfaceClassification {
	return jssymbolidentity.ClassifySurface(path, kind)
}
