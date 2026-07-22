package logicaltarget

import (
	"fmt"
	"path/filepath"
	"strings"

	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

// NormalizeBackendScopeID validates and trims a logical target backend scope.
func NormalizeBackendScopeID(scopeID string) (string, error) {
	return normalizeBackendScopeID(scopeID)
}

func normalizeBackendScopeID(scopeID string) (string, error) {
	trimmed := strings.TrimSpace(scopeID)
	if trimmed == "" {
		return "", requiredFieldError("backendScopeId", "backend scope id is required")
	}
	return trimmed, nil
}

// NormalizeFolderPath canonicalizes one logical target folder through injected
// filesystem effects.
func NormalizeFolderPathWithEffects(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
	folderPath string,
) (string, error) {
	if resolveSymlinks == nil {
		return "", invalidTargetError("folderPath", "logical target symlink resolver is required")
	}
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", requiredFieldError("folderPath", "factory session folder is required")
	}
	expanded, err := expandHome(trimmed, resolveHome)
	if err != nil {
		return "", invalidTargetError("folderPath", err.Error())
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", invalidTargetError("folderPath", fmt.Sprintf("resolve factory session folder %q: %v", folderPath, err))
	}
	resolved = filepath.Clean(resolved)
	if canonical, err := resolveSymlinks(resolved); err == nil {
		resolved = filepath.Clean(canonical)
	}
	return resolved, nil
}

func expandHome(path string, resolveHome factorysessions.HomeDirectoryResolver) (string, error) {
	if resolveHome == nil {
		return "", fmt.Errorf("Factory Session home-directory resolver is required")
	}
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		return path, nil
	}
	homeDir, err := resolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve user home for factory session folder %q: %w", path, err)
	}
	if path == "~" {
		return homeDir, nil
	}
	return filepath.Join(homeDir, path[2:]), nil
}

// IsDefaultSessionSelector reports whether selector addresses the default
// logical target.
func IsDefaultSessionSelector(selector string) bool {
	return isDefaultSessionSelector(selector)
}

func isDefaultSessionSelector(selector string) bool {
	switch strings.TrimSpace(selector) {
	case "", factorysessions.DefaultSessionID, string(factorysessions.TargetKindDefault):
		return true
	default:
		return false
	}
}

func NormalizeDefaultTargetWithEffects(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
) (CanonicalReference, error) {
	return NormalizeTargetRefWithEffects(resolveSymlinks, resolveHome, backendScopeID, folderPath, factorysessions.TargetRef{Kind: factorysessions.TargetKindDefault})
}

func NormalizeNamedTargetWithEffects(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	name string,
) (CanonicalReference, error) {
	return NormalizeTargetRefWithEffects(resolveSymlinks, resolveHome, backendScopeID, folderPath, factorysessions.TargetRef{Kind: factorysessions.TargetKindNamed, Name: name})
}

func NormalizeProviderTargetWithEffects(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	boundary ProviderBoundary,
) (CanonicalReference, error) {
	scopeID, err := NormalizeBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalReference{}, err
	}
	resolvedFolder, err := NormalizeFolderPathWithEffects(resolveSymlinks, resolveHome, folderPath)
	if err != nil {
		return CanonicalReference{}, err
	}
	normalizedBoundary, err := NormalizeProviderBoundaryValue(boundary)
	if err != nil {
		return CanonicalReference{}, err
	}
	return CanonicalReference{
		BackendScopeID: scopeID,
		FolderPath:     resolvedFolder,
		Kind:           KindProvider,
		Provider:       &normalizedBoundary,
	}, nil
}

func NormalizeProviderBoundaryValue(boundary ProviderBoundary) (ProviderBoundary, error) {
	provider := strings.ToLower(strings.TrimSpace(boundary.Provider))
	kind := strings.ToLower(strings.TrimSpace(boundary.Kind))
	scopeBoundary := strings.TrimSpace(boundary.Boundary)
	switch {
	case provider == "":
		return ProviderBoundary{}, requiredFieldError("provider", "provider-backed target requires provider")
	case kind == "":
		return ProviderBoundary{}, requiredFieldError("provider.kind", "provider-backed target requires provider kind")
	case scopeBoundary == "":
		return ProviderBoundary{}, requiredFieldError("provider.boundary", "provider-backed target requires stable provider boundary")
	case looksLikeSecret(scopeBoundary):
		return ProviderBoundary{}, invalidTargetError("provider.boundary", "provider boundary must not contain secret material")
	}
	return ProviderBoundary{Provider: provider, Kind: kind, Boundary: scopeBoundary}, nil
}

func NormalizeTargetRefWithEffects(
	resolveSymlinks factorysessions.LogicalTargetResolveSymlinks,
	resolveHome factorysessions.HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	ref factorysessions.TargetRef,
) (CanonicalReference, error) {
	scopeID, err := NormalizeBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalReference{}, err
	}
	resolvedFolder, err := NormalizeFolderPathWithEffects(resolveSymlinks, resolveHome, folderPath)
	if err != nil {
		return CanonicalReference{}, err
	}

	kind := factorysessions.TargetKind(strings.TrimSpace(string(ref.Kind)))
	name := strings.TrimSpace(ref.Name)
	switch {
	case kind == "" || kind == factorysessions.TargetKindDefault:
		if name != "" {
			return CanonicalReference{}, ambiguousTargetError("target", "default factory session target cannot include a name")
		}
		return CanonicalReference{BackendScopeID: scopeID, FolderPath: resolvedFolder, Kind: KindDefault}, nil
	case kind == factorysessions.TargetKindNamed:
		if name == "" {
			return CanonicalReference{}, requiredFieldError("target.name", "named factory session target requires a name")
		}
		segments, err := namedfactorypath.PathSegments(name)
		if err != nil {
			return CanonicalReference{}, invalidTargetError("target.name", err.Error())
		}
		canonicalName, err := namedfactorypath.NameFromPathSegments(segments)
		if err != nil {
			return CanonicalReference{}, invalidTargetError("target.name", err.Error())
		}
		return CanonicalReference{BackendScopeID: scopeID, FolderPath: resolvedFolder, Kind: KindNamed, NamedTarget: canonicalName}, nil
	default:
		return CanonicalReference{}, invalidTargetError("target.kind", fmt.Sprintf("unsupported factory session target kind %q", ref.Kind))
	}
}

func Equivalent(left, right CanonicalReference) bool {
	return equivalent(left, right)
}

func equivalent(left, right CanonicalReference) bool {
	return signature(left) == signature(right)
}

func signature(ref CanonicalReference) string {
	parts := []string{ref.BackendScopeID, ref.FolderPath, string(ref.Kind)}
	switch ref.Kind {
	case KindNamed:
		parts = append(parts, ref.NamedTarget)
	case KindProvider:
		if ref.Provider != nil {
			parts = append(parts, ref.Provider.Provider, ref.Provider.Kind, ref.Provider.Boundary)
		}
	}
	return strings.Join(parts, "|")
}

func looksLikeSecret(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"bearer ", "api_key", "apikey", "secret", "password", "token="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "xox")
}
