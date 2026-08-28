package models

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
)

// ErrNotAvailable reports that a discovered local model exists but its
// required local assets are not present in the managed cache. It is distinct
// from ErrPullUnsupported and ErrSourceFetchFailed so peers can branch on
// typed pull outcomes through the root contract.
var ErrNotAvailable = errors.New("model not available")

// ErrPullUnsupported reports that the requested model does not support
// managed local asset pulls in the current runtime or platform. It is distinct
// from ErrNotAvailable and ErrSourceFetchFailed.
var ErrPullUnsupported = errors.New("model pull is not supported")

// ErrSourceFetchFailed reports that required managed runtime assets could not
// be fetched from the configured backend source. Classified pull failures may
// wrap this cause in PullError while carrying ManagedPullOutcome vocabulary.
var ErrSourceFetchFailed = errors.New("managed runtime source fetch failed")

var (
	// ErrAssetSourceMissing reports that no configured source can provide the
	// requested model assets.
	ErrAssetSourceMissing = errors.New("model asset source is missing")
	// ErrAssetSourceUnsupported reports that the configured source cannot be
	// used by the current Models implementation or platform.
	ErrAssetSourceUnsupported = errors.New("model asset source is unsupported")
	// ErrAssetUnavailable reports that requested assets are not currently
	// available for inspection or use.
	ErrAssetUnavailable = ErrNotAvailable
	// ErrAssetPreparationInterrupted reports that preparation stopped before
	// the requested assets reached an available state.
	ErrAssetPreparationInterrupted = errors.New("model asset preparation was interrupted")
	// ErrAssetIntegrityFailed reports that prepared assets failed verification.
	ErrAssetIntegrityFailed = errors.New("model asset integrity verification failed")
	// ErrAssetCancelled reports cancellation of a Models asset operation.
	ErrAssetCancelled = errors.New("model asset operation cancelled")
	// ErrAssetOffline reports that an offline preparation request could not
	// satisfy every requested artifact from the ordered local caches.
	ErrAssetOffline = errors.New("model assets are missing while offline")
	// ErrAssetBackendNotReady reports that a backend artifact could not be
	// proven reachable during the zero-body preflight. It is deliberately
	// distinct from a model-source failure so invocation transports can publish
	// the stable backend-not-ready classification.
	ErrAssetBackendNotReady = errors.New("model backend is not ready")
	// ErrAssetEstimateOverflow reports that the requested asset sizes cannot be
	// represented by the Models byte-total contract without wrapping int64.
	ErrAssetEstimateOverflow = errors.New("model asset estimate exceeds int64")
	// ErrModelCacheNotFound reports that the selected model has no managed
	// cache revision that can be removed.
	ErrModelCacheNotFound = errors.New("managed model cache not found")
	// ErrModelCacheInUse reports that a live model host or invocation lease
	// still holds the selected managed cache.
	ErrModelCacheInUse = errors.New("managed model cache is in use")
	// ErrModelCacheUnsafe reports that a managed cache path contains a link or
	// path shape that cannot be removed without following it outside the cache.
	ErrModelCacheUnsafe = errors.New("managed model cache path is unsafe")
	// ErrModelCacheRemovalFailed reports that removal could not be verified.
	ErrModelCacheRemovalFailed = errors.New("managed model cache removal failed")
)

// PullStage identifies the stage at which a managed local-model pull stopped.
// It is intentionally narrower than the legacy PULLED/FAILED outcome so a
// caller can distinguish a source problem from a failure after bytes arrived.
type PullStage string

const (
	PullStageSourceResolution      PullStage = "SOURCE_RESOLUTION"
	PullStageSourceFetch           PullStage = "SOURCE_FETCH"
	PullStageIntegrityVerification PullStage = "INTEGRITY_VERIFICATION"
	PullStageAssembly              PullStage = "ASSEMBLY"
	PullStageCacheInstallation     PullStage = "CACHE_INSTALLATION"
	PullStageReadinessEvaluation   PullStage = "READINESS_EVALUATION"
)

// PullStageError preserves the failed stage and the original cause across the
// Models boundary. Operation and artifact are logical labels; implementations
// must not put credentials, URLs, or private cache paths in them.
type PullStageError struct {
	Stage     PullStage
	ModelName string
	Operation string
	Artifact  string
	Cause     error
}

func (failure *PullStageError) Error() string {
	if failure == nil {
		return ""
	}
	message := fmt.Sprintf("managed runtime pull failed during %s", strings.ToLower(string(failure.Stage)))
	if operation := strings.TrimSpace(failure.Operation); operation != "" {
		message += ": " + operation
	}
	if artifact := strings.TrimSpace(failure.Artifact); artifact != "" {
		message += fmt.Sprintf(" for asset %q", artifact)
	}
	return message
}

func (failure *PullStageError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// PullDiagnostics contains safe, logical facts about one failed managed
// runtime pull. It deliberately excludes response bodies, credentials, and
// unrestricted local paths so callers can carry it to operator-facing
// diagnostics without exposing implementation details.
type PullDiagnostics struct {
	ModelName          string
	ResolvedRepository string
	Revision           string
	File               string
	Operation          string
	RequestURL         string
	UpstreamStatusCode int
}

// Normalize returns a detached, safe representation suitable for transport
// mapping or logging. Unknown URL query parameters and path-like values are
// discarded at this boundary.
func (diagnostics PullDiagnostics) Normalize() PullDiagnostics {
	diagnostics.ModelName = normalizePullDiagnosticModelName(diagnostics.ModelName)
	diagnostics.ResolvedRepository = normalizePullDiagnosticRepository(diagnostics.ResolvedRepository)
	diagnostics.Revision = normalizePullDiagnosticRevision(diagnostics.Revision)
	diagnostics.File = normalizePullDiagnosticFile(diagnostics.File)
	diagnostics.Operation = normalizePullDiagnosticText(diagnostics.Operation)
	diagnostics.RequestURL = normalizePullDiagnosticURL(diagnostics.RequestURL)
	if diagnostics.UpstreamStatusCode < 100 || diagnostics.UpstreamStatusCode > 599 {
		diagnostics.UpstreamStatusCode = 0
	}
	return diagnostics
}

func normalizePullDiagnosticModelName(value string) string {
	value = normalizePullDiagnosticSafeField(value)
	if value == "" || strings.ContainsRune(value, '\\') || strings.ContainsRune(value, '\x00') ||
		path.IsAbs(value) || filepath.IsAbs(value) || hasWindowsDrivePrefix(value) ||
		strings.HasPrefix(value, "../") || value == ".." {
		return ""
	}
	return value
}

// WithDefaults fills only missing facts and then normalizes the complete
// diagnostic. Defaults are logical labels supplied by a caller that owns the
// surrounding pull context.
func (diagnostics PullDiagnostics) WithDefaults(
	modelName, resolvedRepository, revision, file, operation string,
) PullDiagnostics {
	if strings.TrimSpace(diagnostics.ModelName) == "" {
		diagnostics.ModelName = modelName
	}
	if strings.TrimSpace(diagnostics.ResolvedRepository) == "" {
		diagnostics.ResolvedRepository = resolvedRepository
	}
	if strings.TrimSpace(diagnostics.Revision) == "" {
		diagnostics.Revision = revision
	}
	if strings.TrimSpace(diagnostics.File) == "" {
		diagnostics.File = file
	}
	if strings.TrimSpace(diagnostics.Operation) == "" {
		diagnostics.Operation = operation
	}
	return diagnostics.Normalize()
}

func (diagnostics PullDiagnostics) hasDetails() bool {
	return strings.TrimSpace(diagnostics.ModelName) != "" ||
		strings.TrimSpace(diagnostics.ResolvedRepository) != "" ||
		strings.TrimSpace(diagnostics.Revision) != "" ||
		strings.TrimSpace(diagnostics.File) != "" ||
		strings.TrimSpace(diagnostics.Operation) != "" ||
		strings.TrimSpace(diagnostics.RequestURL) != "" ||
		diagnostics.UpstreamStatusCode != 0
}

// HasDetails reports whether at least one safe diagnostic fact is present.
func (diagnostics PullDiagnostics) HasDetails() bool {
	return diagnostics.Normalize().hasDetails()
}

// ErrorText renders only the structured facts in a stable, line-free form.
func (diagnostics PullDiagnostics) ErrorText() string {
	diagnostics = diagnostics.Normalize()
	parts := make([]string, 0, 7)
	if diagnostics.ModelName != "" {
		parts = append(parts, "model="+diagnostics.ModelName)
	}
	if diagnostics.ResolvedRepository != "" {
		parts = append(parts, "repository="+diagnostics.ResolvedRepository)
	}
	if diagnostics.Revision != "" {
		parts = append(parts, "revision="+diagnostics.Revision)
	}
	if diagnostics.File != "" {
		parts = append(parts, "file="+diagnostics.File)
	}
	if diagnostics.Operation != "" {
		parts = append(parts, "operation="+diagnostics.Operation)
	}
	if diagnostics.RequestURL != "" {
		parts = append(parts, "url="+diagnostics.RequestURL)
	}
	if diagnostics.UpstreamStatusCode != 0 {
		parts = append(parts, fmt.Sprintf("status=%d", diagnostics.UpstreamStatusCode))
	}
	if len(parts) == 0 {
		return "managed runtime pull diagnostic unavailable"
	}
	return "managed runtime pull diagnostics: " + strings.Join(parts, " ")
}

// PullDiagnosticsError carries safe facts while retaining the typed cause for
// errors.Is/errors.As. It intentionally does not implement Unwrap: generic
// debug renderers must not walk into arbitrary transport or filesystem text.
type PullDiagnosticsError struct {
	Diagnostics PullDiagnostics
	Cause       error
}

func (failure *PullDiagnosticsError) Error() string {
	if failure == nil {
		return ""
	}
	return failure.Diagnostics.ErrorText()
}

func (failure *PullDiagnosticsError) Is(target error) bool {
	if failure == nil || failure.Cause == nil {
		return false
	}
	return errors.Is(failure.Cause, target)
}

func (failure *PullDiagnosticsError) As(target any) bool {
	if failure == nil || failure.Cause == nil {
		return false
	}
	return errors.As(failure.Cause, target)
}

func normalizePullDiagnosticText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, character := range value {
		switch {
		case character == '\r' || character == '\n' || character == '\t':
			builder.WriteByte(' ')
		case character < 0x20 || character == 0x7f:
			// Drop control characters rather than allowing line injection.
		default:
			builder.WriteRune(character)
		}
	}
	return strings.TrimSpace(builder.String())
}

func normalizePullDiagnosticRepository(value string) string {
	value = normalizePullDiagnosticSafeField(value)
	value = strings.TrimPrefix(value, "upstream-repository:")
	if value == "" || strings.ContainsRune(value, '\\') || strings.ContainsRune(value, '\x00') ||
		path.IsAbs(value) || filepath.IsAbs(value) || hasWindowsDrivePrefix(value) || strings.Contains(value, "://") {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return ""
	}
	return clean
}

func normalizePullDiagnosticRevision(value string) string {
	value = normalizePullDiagnosticSafeField(value)
	if value == "" || strings.ContainsRune(value, '\\') || strings.ContainsRune(value, '\x00') ||
		path.IsAbs(value) || filepath.IsAbs(value) || hasWindowsDrivePrefix(value) {
		return ""
	}
	return value
}

func normalizePullDiagnosticFile(value string) string {
	value = normalizePullDiagnosticSafeField(value)
	if value == "" || strings.ContainsRune(value, '\\') || strings.ContainsRune(value, '\x00') || path.IsAbs(value) || filepath.IsAbs(value) || hasWindowsDrivePrefix(value) {
		return ""
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != value {
		return ""
	}
	return clean
}

func normalizePullDiagnosticURL(value string) string {
	value = normalizePullDiagnosticText(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.Fragment = ""
	if containsPullDiagnosticSensitiveText(parsed.Path) {
		return ""
	}
	allowed := url.Values{}
	query := parsed.Query()
	for _, key := range []string{"download", "revision"} {
		for _, item := range query[key] {
			item = normalizePullDiagnosticText(item)
			if item != "" && !containsPullDiagnosticSensitiveText(item) &&
				!strings.ContainsRune(item, '\\') && !strings.ContainsRune(item, '\x00') {
				allowed.Add(key, item)
			}
		}
	}
	parsed.RawQuery = allowed.Encode()
	return parsed.String()
}

func normalizePullDiagnosticSafeField(value string) string {
	value = normalizePullDiagnosticText(value)
	if containsPullDiagnosticSensitiveText(value) {
		return ""
	}
	return value
}

func containsPullDiagnosticSensitiveText(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"authorization", "bearer ", "cookie", "password=", "passwd=", "secret=",
		"token=", "api-key=", "api_key=", "apikey=", "access-token=", "refresh-token=",
		"hf_token=", "body=",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// AssetArtifactKind separates model payloads from backend runtime artifacts.
// The distinction is part of the cache identity so one kind can never satisfy
// a request for the other kind.
type AssetArtifactKind string

const (
	AssetArtifactKindModel   AssetArtifactKind = "MODEL"
	AssetArtifactKindBackend AssetArtifactKind = "BACKEND"
)

// AssetReadinessState names the detached availability state of scoped assets.
type AssetReadinessState string

const (
	AssetReadinessMissing   AssetReadinessState = "MISSING"
	AssetReadinessPreparing AssetReadinessState = "PREPARING"
	AssetReadinessAvailable AssetReadinessState = "AVAILABLE"
	AssetReadinessFailed    AssetReadinessState = "FAILED"
)

// AssetIntegrityState names the result of verifying prepared assets.
type AssetIntegrityState string

const (
	AssetIntegrityUnknown  AssetIntegrityState = "UNKNOWN"
	AssetIntegrityVerified AssetIntegrityState = "VERIFIED"
	AssetIntegrityFailed   AssetIntegrityState = "FAILED"
)

// AssetPreparationOutcome distinguishes a cache hit from newly prepared assets.
type AssetPreparationOutcome string

const (
	AssetPreparationAlreadyAvailable AssetPreparationOutcome = "ALREADY_AVAILABLE"
	AssetPreparationPrepared         AssetPreparationOutcome = "PREPARED"
)

// AssetRemovalOutcome reports whether a remove operation changed asset state.
type AssetRemovalOutcome string

const (
	AssetRemovalRemoved       AssetRemovalOutcome = "REMOVED"
	AssetRemovalAlreadyAbsent AssetRemovalOutcome = "ALREADY_ABSENT"
)

// AssetArtifact describes one prepared artifact without exposing its cache or
// filesystem location.
type AssetArtifact struct {
	Kind   AssetArtifactKind
	Name   string
	Bytes  int64
	SHA256 string
}

// AssetSnapshot contains detached readiness, integrity, source, and artifact
// facts for one scoped model.
type AssetSnapshot struct {
	ModelName        string
	Readiness        AssetReadinessState
	Integrity        AssetIntegrityState
	Source           SourceMetadata
	Revision         string
	Artifacts        []AssetArtifact
	BackendArtifacts []AssetArtifact
	TotalBytes       int64
}

// Clone returns a detached asset snapshot safe for a peer to retain.
func (snapshot AssetSnapshot) Clone() AssetSnapshot {
	snapshot.Artifacts = append([]AssetArtifact(nil), snapshot.Artifacts...)
	snapshot.BackendArtifacts = append([]AssetArtifact(nil), snapshot.BackendArtifacts...)
	return snapshot
}

// AssetRequirement describes one immutable artifact needed by a preparation
// transaction. URL and filesystem details are derived privately from the
// source reference and never appear in results.
type AssetRequirement struct {
	Name   string
	Bytes  int64
	SHA256 string
}

// Validate rejects unsafe artifact names before any cache, filesystem, or
// network effect can occur.
func (requirement AssetRequirement) Validate() error {
	name := strings.TrimSpace(requirement.Name)
	cleanName := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	if invalidAssetName(name, cleanName) {
		return fmt.Errorf("%w: asset name is invalid", ErrAssetPreparationInterrupted)
	}
	if requirement.Bytes < 0 {
		return fmt.Errorf("%w: asset %q has a negative size", ErrAssetPreparationInterrupted, name)
	}
	if err := validateAssetDigest(name, requirement.SHA256); err != nil {
		return err
	}
	return nil
}

func invalidAssetName(name, cleanName string) bool {
	return name == "" || cleanName != name || name == "." ||
		strings.ContainsRune(name, '\x00') || strings.Contains(name, "\\") ||
		path.IsAbs(filepath.ToSlash(name)) || filepath.IsAbs(filepath.FromSlash(name)) ||
		filepath.VolumeName(name) != "" || hasWindowsDrivePrefix(name)
}

func hasWindowsDrivePrefix(name string) bool {
	return len(name) >= 2 && name[1] == ':' &&
		((name[0] >= 'a' && name[0] <= 'z') || (name[0] >= 'A' && name[0] <= 'Z'))
}

func validateAssetDigest(name, rawDigest string) error {
	digest := strings.TrimSpace(rawDigest)
	if digest == "" {
		return nil
	}
	if len(digest) != 64 {
		return fmt.Errorf("%w: asset %q digest is invalid", ErrAssetIntegrityFailed, name)
	}
	for _, character := range digest {
		if !isAssetHexCharacter(character) {
			return fmt.Errorf("%w: asset %q digest is invalid", ErrAssetIntegrityFailed, name)
		}
	}
	return nil
}

func isAssetHexCharacter(character rune) bool {
	return character >= '0' && character <= '9' || character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F'
}

// AssetOfflineError lists every artifact that was unavailable when a
// preparation request was explicitly offline. Names are safe logical names;
// cache paths, URLs, and authorization details are deliberately omitted.
type AssetOfflineError struct {
	Missing []string
}

func (failure *AssetOfflineError) Error() string {
	if failure == nil {
		return ""
	}
	missing := append([]string(nil), failure.Missing...)
	return fmt.Sprintf("%v: missing artifacts: %s", ErrAssetOffline, strings.Join(missing, ", "))
}

func (failure *AssetOfflineError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return ErrAssetOffline
}

// PrepareModelAssetsRequest asks Models to make one scoped model's assets
// available from its configured source.
type PrepareModelAssetsRequest struct {
	Scope            RuntimeScopeRef
	Name             string
	Reference        ModelReference
	Offline          bool
	Artifacts        []AssetRequirement
	Backend          string
	BackendReference ModelReference
	BackendArtifacts []AssetRequirement
}

// Validate checks fields whose validity does not depend on private scope,
// source, or cache state.
func (request PrepareModelAssetsRequest) Validate() error {
	name := request.Name
	if !request.Reference.IsZero() {
		name = request.Reference.NameOrURI
	}
	if err := validateAssetModelName(name); err != nil {
		return err
	}
	if err := validateAssetRequirements(request.Artifacts); err != nil {
		return err
	}
	if err := validateAssetRequirements(request.BackendArtifacts); err != nil {
		return err
	}
	if !request.BackendReference.IsZero() && strings.TrimSpace(request.Backend) == "" {
		return fmt.Errorf("%w: backend identity is required", ErrAssetPreparationInterrupted)
	}
	return nil
}

func validateAssetRequirements(requirements []AssetRequirement) error {
	seen := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if _, exists := seen[requirement.Name]; exists {
			return fmt.Errorf("%w: asset %q is requested more than once", ErrAssetPreparationInterrupted, requirement.Name)
		}
		seen[requirement.Name] = struct{}{}
	}
	return nil
}

// PrepareModelAssetsResult reports detached asset facts and whether the
// operation reused already-available assets or prepared them.
type PrepareModelAssetsResult struct {
	Asset   AssetSnapshot
	Outcome AssetPreparationOutcome
}

// PreflightModelAssetsResult reports cache-aware download requirements without
// downloading artifact content. Metadata and zero-body reachability checks may
// be performed by the implementation; the byte totals describe only assets
// that are still required after the cache inspection.
type PreflightModelAssetsResult struct {
	ModelName               string
	BackendBytes            int64
	ModelBytes              int64
	TotalBytes              int64
	BackendDownloadRequired bool
	ModelDownloadRequired   bool
}

// InspectModelAssetsRequest asks Models to inspect one scoped model's assets.
// VerifyIntegrity requests checksum verification as part of that inspection.
type InspectModelAssetsRequest struct {
	Scope           RuntimeScopeRef
	Name            string
	VerifyIntegrity bool
}

// Validate checks fields whose validity does not depend on private scope,
// source, or cache state.
func (request InspectModelAssetsRequest) Validate() error {
	return validateAssetModelName(request.Name)
}

// InspectModelAssetsResult returns detached readiness and verification facts.
type InspectModelAssetsResult struct {
	Asset AssetSnapshot
}

// RemoveModelAssetsRequest asks Models to remove one scoped model's assets.
type RemoveModelAssetsRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// Validate checks fields whose validity does not depend on private scope or
// cache state.
func (request RemoveModelAssetsRequest) Validate() error {
	return validateAssetModelName(request.Name)
}

// RemoveModelAssetsResult reports the resulting readiness and whether assets
// were removed or were already absent.
type RemoveModelAssetsResult struct {
	ModelName    string
	Revision     string
	CachePath    string
	BytesRemoved int64
	Readiness    AssetReadinessState
	Outcome      AssetRemovalOutcome
}

func validateAssetModelName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: empty model name", ErrNotFound)
	}
	return nil
}

// PullModelRequest is the plain assets pull request. Peers identify a model by
// Name without importing models/internal/assets or nested puller/cache types.
type PullModelRequest struct {
	Scope RuntimeScopeRef
	Name  string
}

// ValidatePullModelRequest checks the plain pull-model request. Empty names
// fail closed as ErrNotFound without touching nested asset-puller packages.
func ValidatePullModelRequest(request PullModelRequest) error {
	return validateAssetModelName(request.Name)
}

// DownloadedFile describes one cached artifact materialized by a managed
// local-model asset pull. Peers consume this Models-owned vocabulary without
// nested assets puller or cache implementation types.
type DownloadedFile struct {
	Path   string
	Bytes  int64
	SHA256 string
}

// PullResult carries the Models-owned outcome of pulling one model into the
// managed local cache, including downloaded-file and pull-outcome vocabulary
// peers need. Transport packages map it to public response contracts. Asset
// operations stay on the singular root Service; peers must not import a nested
// asset-gateway interface for this slice.
type PullResult struct {
	ModelName          string
	ProviderLocality   string
	Outcome            string
	CachePath          string
	Revision           string
	DownloadedFiles    []DownloadedFile
	ManagedPullOutcome string
	ReadinessState     string
	LifecycleState     string
	SourceKind         string
	SourceID           string
	ResolverNotes      string
	FailureStage       PullStage
	PullDiagnostics    PullDiagnostics
}

// PullError preserves a classified pull result while retaining its cause.
type PullError struct {
	Result PullResult
	Cause  error
}

func (e *PullError) Error() string {
	if e == nil {
		return ""
	}
	if e.Result.ManagedPullOutcome == "" {
		return fmt.Sprintf("managed runtime pull failed for %q", e.Result.ModelName)
	}
	return fmt.Sprintf(
		"managed runtime pull for %q failed with outcome %s (readiness %s)",
		e.Result.ModelName,
		e.Result.ManagedPullOutcome,
		e.Result.ReadinessState,
	)
}

func (e *PullError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
