package logicaltarget

import (
	"os"
	"path/filepath"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func NormalizeFolderPath(folderPath string) (string, error) {
	return NormalizeFolderPathWithEffects(filepath.EvalSymlinks, os.UserHomeDir, folderPath)
}

func NormalizeDefaultTarget(backendScopeID, folderPath string) (CanonicalReference, error) {
	return NormalizeTargetRefWithEffects(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
}

func NormalizeNamedTarget(backendScopeID, folderPath, name string) (CanonicalReference, error) {
	return NormalizeTargetRefWithEffects(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: name})
}

func NormalizeProviderTarget(
	backendScopeID string,
	folderPath string,
	boundary ProviderBoundary,
) (CanonicalReference, error) {
	return NormalizeProviderTargetWithEffects(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, boundary)
}

func NormalizeTargetRef(
	backendScopeID string,
	folderPath string,
	ref factorysessions.TargetRef,
) (CanonicalReference, error) {
	return NormalizeTargetRefWithEffects(filepath.EvalSymlinks, os.UserHomeDir, backendScopeID, folderPath, ref)
}

func IsDefaultSessionSelector(selector string) bool { return isDefaultSessionSelector(selector) }
