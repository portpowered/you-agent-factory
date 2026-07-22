package factorysessions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type LogicalTargetKind string

const (
	LogicalTargetKindDefault  LogicalTargetKind = "default"
	LogicalTargetKindNamed    LogicalTargetKind = "named"
	LogicalTargetKindProvider LogicalTargetKind = "provider"

	LogicalTargetReasonRequired        = "required"
	LogicalTargetReasonInvalidTarget   = "invalid_target"
	LogicalTargetReasonAmbiguousTarget = "ambiguous_target"
)

var (
	ErrLogicalTargetRequired  = errors.New("logical session target field is required")
	ErrLogicalTargetInvalid   = errors.New("logical session target reference is invalid")
	ErrLogicalTargetAmbiguous = errors.New("logical session target reference is ambiguous")
)

type LogicalTargetProviderBoundary struct {
	Provider string
	Kind     string
	Boundary string
}

// LogicalTargetResolveSymlinks is the exact filesystem effect used to
// canonicalize an existing Factory Session folder. Callers inject the effect;
// normalization never selects a host-filesystem implementation.
type LogicalTargetResolveSymlinks func(string) (string, error)

// LogicalTargetReferenceNormalizer is the exact service-root operation
// consumed by representation boundaries that need a canonical target.
type LogicalTargetReferenceNormalizer func(
	backendScopeID string,
	folderPath string,
	ref TargetRef,
) (CanonicalLogicalTargetReference, error)

type CanonicalLogicalTargetReference struct {
	BackendScopeID string
	FolderPath     string
	Kind           LogicalTargetKind
	NamedTarget    string
	Provider       *LogicalTargetProviderBoundary
}

type logicalTargetValidationError struct {
	reason string
	field  string
	err    error
}

func (e *logicalTargetValidationError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *logicalTargetValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func LogicalTargetValidationReason(err error) (reason string, field string, ok bool) {
	var validation *logicalTargetValidationError
	if !errors.As(err, &validation) || validation == nil {
		return "", "", false
	}
	return validation.reason, validation.field, true
}

func NewLogicalTargetValidationError(reason, field string, err error) error {
	if err == nil {
		return nil
	}
	return &logicalTargetValidationError{reason: reason, field: field, err: err}
}

func requiredLogicalTargetField(field, message string) error {
	if message == "" {
		message = fmt.Sprintf("%s is required", field)
	}
	return NewLogicalTargetValidationError(
		LogicalTargetReasonRequired,
		field,
		fmt.Errorf("%w: %s", ErrLogicalTargetRequired, message),
	)
}

func invalidLogicalTarget(field, message string) error {
	if message == "" {
		message = "logical session target reference is invalid"
	}
	return NewLogicalTargetValidationError(
		LogicalTargetReasonInvalidTarget,
		field,
		fmt.Errorf("%w: %s", ErrLogicalTargetInvalid, message),
	)
}

func ambiguousLogicalTarget(field, message string) error {
	if message == "" {
		message = "logical session target reference is ambiguous"
	}
	return NewLogicalTargetValidationError(
		LogicalTargetReasonAmbiguousTarget,
		field,
		fmt.Errorf("%w: %s", ErrLogicalTargetAmbiguous, message),
	)
}

func NormalizeLogicalTargetBackendScopeID(scopeID string) (string, error) {
	trimmed := strings.TrimSpace(scopeID)
	if trimmed == "" {
		return "", requiredLogicalTargetField("backendScopeId", "backend scope id is required")
	}
	return trimmed, nil
}

func NormalizeLogicalTargetFolderPath(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	folderPath string,
) (string, error) {
	if resolveSymlinks == nil {
		return "", invalidLogicalTarget("folderPath", "logical target symlink resolver is required")
	}
	trimmed := strings.TrimSpace(folderPath)
	if trimmed == "" {
		return "", requiredLogicalTargetField("folderPath", "factory session folder is required")
	}
	expanded, err := ExpandFolderHome(trimmed, resolveHome)
	if err != nil {
		return "", invalidLogicalTarget("folderPath", err.Error())
	}
	resolved, err := filepath.Abs(expanded)
	if err != nil {
		return "", invalidLogicalTarget(
			"folderPath",
			fmt.Sprintf("resolve factory session folder %q: %v", folderPath, err),
		)
	}
	resolved = filepath.Clean(resolved)
	if canonical, err := resolveSymlinks(resolved); err == nil {
		resolved = filepath.Clean(canonical)
	}
	return resolved, nil
}

func IsDefaultLogicalTargetSelector(selector string) bool {
	switch strings.TrimSpace(selector) {
	case "", DefaultSessionID, string(TargetKindDefault):
		return true
	default:
		return false
	}
}

func NormalizeDefaultLogicalTarget(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
) (CanonicalLogicalTargetReference, error) {
	return normalizeLogicalTarget(resolveSymlinks, resolveHome, backendScopeID, folderPath, TargetRef{Kind: TargetKindDefault})
}

func NormalizeNamedLogicalTarget(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	name string,
) (CanonicalLogicalTargetReference, error) {
	return normalizeLogicalTarget(resolveSymlinks, resolveHome, backendScopeID, folderPath, TargetRef{Kind: TargetKindNamed, Name: name})
}

func NormalizeProviderLogicalTarget(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	boundary LogicalTargetProviderBoundary,
) (CanonicalLogicalTargetReference, error) {
	scopeID, err := NormalizeLogicalTargetBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalLogicalTargetReference{}, err
	}
	resolvedFolder, err := NormalizeLogicalTargetFolderPath(resolveSymlinks, resolveHome, folderPath)
	if err != nil {
		return CanonicalLogicalTargetReference{}, err
	}
	normalizedBoundary, err := NormalizeLogicalTargetProviderBoundary(boundary)
	if err != nil {
		return CanonicalLogicalTargetReference{}, err
	}
	return CanonicalLogicalTargetReference{
		BackendScopeID: scopeID,
		FolderPath:     resolvedFolder,
		Kind:           LogicalTargetKindProvider,
		Provider:       &normalizedBoundary,
	}, nil
}

func NormalizeLogicalTargetProviderBoundary(
	boundary LogicalTargetProviderBoundary,
) (LogicalTargetProviderBoundary, error) {
	provider := strings.ToLower(strings.TrimSpace(boundary.Provider))
	kind := strings.ToLower(strings.TrimSpace(boundary.Kind))
	scopeBoundary := strings.TrimSpace(boundary.Boundary)
	switch {
	case provider == "":
		return LogicalTargetProviderBoundary{}, requiredLogicalTargetField("provider", "provider-backed target requires provider")
	case kind == "":
		return LogicalTargetProviderBoundary{}, requiredLogicalTargetField("provider.kind", "provider-backed target requires provider kind")
	case scopeBoundary == "":
		return LogicalTargetProviderBoundary{}, requiredLogicalTargetField(
			"provider.boundary",
			"provider-backed target requires stable provider boundary",
		)
	case logicalTargetLooksLikeSecret(scopeBoundary):
		return LogicalTargetProviderBoundary{}, invalidLogicalTarget(
			"provider.boundary",
			"provider boundary must not contain secret material",
		)
	}
	return LogicalTargetProviderBoundary{Provider: provider, Kind: kind, Boundary: scopeBoundary}, nil
}

func NormalizeLogicalTargetRef(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	ref TargetRef,
) (CanonicalLogicalTargetReference, error) {
	return normalizeLogicalTarget(resolveSymlinks, resolveHome, backendScopeID, folderPath, ref)
}

func normalizeLogicalTarget(
	resolveSymlinks LogicalTargetResolveSymlinks,
	resolveHome HomeDirectoryResolver,
	backendScopeID string,
	folderPath string,
	ref TargetRef,
) (CanonicalLogicalTargetReference, error) {
	scopeID, err := NormalizeLogicalTargetBackendScopeID(backendScopeID)
	if err != nil {
		return CanonicalLogicalTargetReference{}, err
	}
	resolvedFolder, err := NormalizeLogicalTargetFolderPath(resolveSymlinks, resolveHome, folderPath)
	if err != nil {
		return CanonicalLogicalTargetReference{}, err
	}

	kind := TargetKind(strings.TrimSpace(string(ref.Kind)))
	name := strings.TrimSpace(ref.Name)
	switch {
	case kind == "" || kind == TargetKindDefault:
		if name != "" {
			return CanonicalLogicalTargetReference{}, ambiguousLogicalTarget(
				"target",
				"default factory session target cannot include a name",
			)
		}
		return CanonicalLogicalTargetReference{
			BackendScopeID: scopeID,
			FolderPath:     resolvedFolder,
			Kind:           LogicalTargetKindDefault,
		}, nil
	case kind == TargetKindNamed:
		if name == "" {
			return CanonicalLogicalTargetReference{}, requiredLogicalTargetField(
				"target.name",
				"named factory session target requires a name",
			)
		}
		segments, err := namedfactorypath.PathSegments(name)
		if err != nil {
			return CanonicalLogicalTargetReference{}, invalidLogicalTarget("target.name", err.Error())
		}
		canonicalName, err := namedfactorypath.NameFromPathSegments(segments)
		if err != nil {
			return CanonicalLogicalTargetReference{}, invalidLogicalTarget("target.name", err.Error())
		}
		return CanonicalLogicalTargetReference{
			BackendScopeID: scopeID,
			FolderPath:     resolvedFolder,
			Kind:           LogicalTargetKindNamed,
			NamedTarget:    canonicalName,
		}, nil
	default:
		return CanonicalLogicalTargetReference{}, invalidLogicalTarget(
			"target.kind",
			fmt.Sprintf("unsupported factory session target kind %q", ref.Kind),
		)
	}
}

func EquivalentLogicalTargets(left, right CanonicalLogicalTargetReference) bool {
	return left.Signature() == right.Signature()
}

func (ref CanonicalLogicalTargetReference) Signature() string {
	parts := []string{ref.BackendScopeID, ref.FolderPath, string(ref.Kind)}
	switch ref.Kind {
	case LogicalTargetKindNamed:
		parts = append(parts, ref.NamedTarget)
	case LogicalTargetKindProvider:
		if ref.Provider != nil {
			parts = append(parts, ref.Provider.Provider, ref.Provider.Kind, ref.Provider.Boundary)
		}
	}
	return strings.Join(parts, "|")
}

func DeriveLogicalSessionKeyID(ref CanonicalLogicalTargetReference) string {
	sum := sha256.Sum256([]byte(ref.Signature()))
	return "lsk-" + hex.EncodeToString(sum[:16])
}

func IsLogicalSessionKeyID(value string) bool {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "lsk-") {
		return false
	}
	payload := strings.TrimPrefix(trimmed, "lsk-")
	if len(payload) != 32 {
		return false
	}
	_, err := hex.DecodeString(payload)
	return err == nil
}

func RuntimeLogicalTargetFromReference(ref CanonicalLogicalTargetReference) RuntimeLogicalTarget {
	target := RuntimeLogicalTarget{Kind: string(ref.Kind), FolderPath: ref.FolderPath}
	if ref.Kind == LogicalTargetKindNamed {
		namedTarget := ref.NamedTarget
		target.NamedTarget = &namedTarget
	}
	if ref.Kind == LogicalTargetKindProvider && ref.Provider != nil {
		target.ProviderBoundary = &RuntimeLogicalProviderBoundary{
			Provider: ref.Provider.Provider,
			Kind:     ref.Provider.Kind,
			Boundary: ref.Provider.Boundary,
		}
	}
	return target
}

func logicalTargetLooksLikeSecret(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"bearer ", "api_key", "apikey", "secret", "password", "token="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.HasPrefix(lower, "sk-") || strings.HasPrefix(lower, "xox")
}
