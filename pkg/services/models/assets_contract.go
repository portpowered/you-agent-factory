package models

import (
	"errors"
	"fmt"
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
)

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
	ModelName string
	Readiness AssetReadinessState
	Outcome   AssetRemovalOutcome
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
