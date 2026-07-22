package logicaltarget

import (
	"os"
	"path/filepath"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// These owner-local helpers retain the concise historical test vocabulary
// without exposing a second production normalization entrypoint. Production
// consumers receive the exact service-root operation they need.
func NormalizeBackendScopeID(scopeID string) (string, error) {
	return factorysessions.NormalizeLogicalTargetBackendScopeID(scopeID)
}

func NormalizeFolderPath(folderPath string) (string, error) {
	return factorysessions.NormalizeLogicalTargetFolderPath(filepath.EvalSymlinks, os.UserHomeDir, folderPath)
}

func IsDefaultSessionSelector(selector string) bool {
	return factorysessions.IsDefaultLogicalTargetSelector(selector)
}

func NormalizeDefaultTarget(backendScopeID, folderPath string) (CanonicalReference, error) {
	return factorysessions.NormalizeDefaultLogicalTarget(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath)
}

func NormalizeNamedTarget(backendScopeID, folderPath, name string) (CanonicalReference, error) {
	return factorysessions.NormalizeNamedLogicalTarget(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, name)
}

func NormalizeProviderTarget(
	backendScopeID string,
	folderPath string,
	boundary ProviderBoundary,
) (CanonicalReference, error) {
	return factorysessions.NormalizeProviderLogicalTarget(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, boundary)
}

func NormalizeProviderBoundary(boundary ProviderBoundary) (ProviderBoundary, error) {
	return factorysessions.NormalizeLogicalTargetProviderBoundary(boundary)
}

func NormalizeTargetRef(
	backendScopeID string,
	folderPath string,
	ref factorysessions.TargetRef,
) (CanonicalReference, error) {
	return factorysessions.NormalizeLogicalTargetRef(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, ref)
}

func Equivalent(left, right CanonicalReference) bool {
	return factorysessions.EquivalentLogicalTargets(left, right)
}
