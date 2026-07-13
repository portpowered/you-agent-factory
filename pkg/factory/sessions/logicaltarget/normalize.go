package logicaltarget

import (
	"fmt"
	"path/filepath"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/sessions"
)

const defaultSessionSelector = factorysessions.DefaultSessionID

// NormalizeBackendScopeID trims and validates a backend scope identifier.
func NormalizeBackendScopeID(scopeID string) (string, error) {
	trimmed := strings.TrimSpace(scopeID)
	if trimmed == "" {
		return "", requiredFieldError("backendScopeId", "backend scope id is required")
	}
	return trimmed, nil
}

// NormalizeFolderPath resolves equivalent folder spellings to one canonical
// absolute path for the same workspace.
func NormalizeFolderPath(folderPath string) (string, error) {
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", requiredFieldError("folderPath", "factory session folder is required")
	}
	expanded, err := factorysessions.ExpandFolderHome(trimmed)
	if err != nil {
		return "", invalidTargetError("folderPath", err.Error())
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", invalidTargetError("folderPath", fmt.Sprintf("resolve factory session folder %q: %v", folderPath, err))
	}
	resolved = filepath.Clean(resolved)
	if canonical, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = filepath.Clean(canonical)
	}
	return resolved, nil
}

// IsDefaultSessionSelector reports whether selector is a default-route alias.
func IsDefaultSessionSelector(selector string) bool {
	switch strings.TrimSpace(selector) {
	case "", defaultSessionSelector, string(factorysessions.TargetKindDefault):
		return true
	default:
		return false
	}
}

// NormalizeDefaultTarget maps default-route aliases to the canonical default
// target within backendScopeID and folderPath.
func NormalizeDefaultTarget(backendScopeID, folderPath string) (CanonicalReference, error) {
	return normalizeTarget(backendScopeID, folderPath, factorysessions.TargetRef{
		Kind: factorysessions.TargetKindDefault,
	})
}

// NormalizeNamedTarget maps equivalent named-target spellings to one canonical
// named target for the same folder and backend scope.
func NormalizeNamedTarget(backendScopeID, folderPath, name string) (CanonicalReference, error) {
	return normalizeTarget(backendScopeID, folderPath, factorysessions.TargetRef{
		Kind: factorysessions.TargetKindNamed,
		Name: name,
	})
}

// NormalizeProviderTarget maps provider-backed target input to a canonical
// provider-scoped reference without leaking secrets into the normalized value.
func NormalizeProviderTarget(
	backendScopeID string,
	folderPath string,
	boundary ProviderBoundary,
) (CanonicalReference, error) {
	scopeID, err := NormalizeBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalReference{}, err
	}
	resolvedFolder, err := NormalizeFolderPath(folderPath)
	if err != nil {
		return CanonicalReference{}, err
	}
	normalizedBoundary, err := NormalizeProviderBoundary(boundary)
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

// NormalizeProviderBoundary trims provider scope fields and rejects empty or
// secret-like boundary values.
func NormalizeProviderBoundary(boundary ProviderBoundary) (ProviderBoundary, error) {
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
	return ProviderBoundary{
		Provider: provider,
		Kind:     kind,
		Boundary: scopeBoundary,
	}, nil
}

// NormalizeTargetRef normalizes a factory session target ref within one backend
// scope and folder path.
func NormalizeTargetRef(
	backendScopeID string,
	folderPath string,
	ref factorysessions.TargetRef,
) (CanonicalReference, error) {
	return normalizeTarget(backendScopeID, folderPath, ref)
}

func normalizeTarget(
	backendScopeID string,
	folderPath string,
	ref factorysessions.TargetRef,
) (CanonicalReference, error) {
	scopeID, err := NormalizeBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalReference{}, err
	}
	resolvedFolder, err := NormalizeFolderPath(folderPath)
	if err != nil {
		return CanonicalReference{}, err
	}

	kind := factorysessions.TargetKind(strings.TrimSpace(string(ref.Kind)))
	name := strings.TrimSpace(ref.Name)
	switch {
	case kind == "" || kind == factorysessions.TargetKindDefault:
		if name != "" {
			return CanonicalReference{}, ambiguousTargetError(
				"target",
				"default factory session target cannot include a name",
			)
		}
		return CanonicalReference{
			BackendScopeID: scopeID,
			FolderPath:     resolvedFolder,
			Kind:           KindDefault,
		}, nil
	case kind == factorysessions.TargetKindNamed:
		if name == "" {
			return CanonicalReference{}, requiredFieldError("target.name", "named factory session target requires a name")
		}
		canonicalName, err := canonicalNamedTargetName(name)
		if err != nil {
			return CanonicalReference{}, err
		}
		return CanonicalReference{
			BackendScopeID: scopeID,
			FolderPath:     resolvedFolder,
			Kind:           KindNamed,
			NamedTarget:    canonicalName,
		}, nil
	default:
		return CanonicalReference{}, invalidTargetError(
			"target.kind",
			fmt.Sprintf("unsupported factory session target kind %q", ref.Kind),
		)
	}
}

func canonicalNamedTargetName(name string) (string, error) {
	segment, err := factoryconfig.NamedFactoryNameToLayoutSegment(name)
	if err != nil {
		return "", invalidTargetError("target.name", err.Error())
	}
	return segment, nil
}

// Equivalent reports whether two canonical references identify the same logical
// session target.
func Equivalent(left, right CanonicalReference) bool {
	return left.Signature() == right.Signature()
}

// Signature returns a stable comparison key for canonical references.
func (ref CanonicalReference) Signature() string {
	parts := []string{
		ref.BackendScopeID,
		ref.FolderPath,
		string(ref.Kind),
	}
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
	secretMarkers := []string{
		"bearer ",
		"api_key",
		"apikey",
		"secret",
		"password",
		"token=",
	}
	for _, marker := range secretMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "xox") {
		return true
	}
	return false
}
