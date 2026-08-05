package factorydefinitions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
)

const ReplayV1SourceFormat = "agent-factory.replay.v1"

const (
	snapshotIdentityMetadataKey  = "snapshot_identity"
	snapshotArtifactsMetadataKey = "snapshot_artifacts_sha256"
)

// FactorySnapshotSource is the narrow effective-definition projection required
// by snapshot capture.
type FactorySnapshotSource = contracts.FactorySnapshotSource

// LoadedFactorySource is the complete effective definition view retained by a
// live Factory Session.
type LoadedFactorySource = contracts.LoadedFactorySource

// Snapshots is the focused Factory Definitions capability for capture,
// portable import preparation, and atomic snapshot materialization. Its
// methods expose only Factory Definitions values and typed input failures.
type Snapshots interface {
	CaptureFactorySnapshot(context.Context, CaptureFactorySnapshotRequest) (CaptureFactorySnapshotResult, error)
	CaptureLoadedFactorySnapshot(context.Context, CaptureLoadedFactorySnapshotRequest) (CaptureFactorySnapshotResult, error)
	PrepareFactorySnapshotImport(context.Context, PrepareFactorySnapshotImportRequest) (PrepareFactorySnapshotImportResult, error)
	MaterializeFactorySnapshot(context.Context, MaterializeFactorySnapshotRequest) (MaterializeFactorySnapshotResult, error)
}

// SnapshotIdentity is the stable, content-addressed identity of a detached
// Factory snapshot. It is derived from canonical snapshot content rather than
// object allocation or map iteration order.
type SnapshotIdentity string

// SnapshotErrorClassification identifies safe, inspectable snapshot input
// failures. It deliberately omits host paths and serialized Factory content.
type SnapshotErrorClassification string

const (
	SnapshotErrorMissing   SnapshotErrorClassification = "missing"
	SnapshotErrorMalformed SnapshotErrorClassification = "malformed"
	SnapshotErrorIntegrity SnapshotErrorClassification = "integrity"
	SnapshotErrorUnsafe    SnapshotErrorClassification = "unsafe"
)

var (
	ErrFactorySnapshotMissing   = errors.New("factory snapshot is missing")
	ErrMalformedFactorySnapshot = ErrInvalidFactorySnapshotPayload
	ErrFactorySnapshotIntegrity = errors.New("factory snapshot integrity check failed")
)

// SnapshotInputError carries safe snapshot failure facts. Identity is a
// content digest and Artifact is a Factory-relative target path, so Error
// never includes serialized input or a host filesystem location.
type SnapshotInputError struct {
	Classification SnapshotErrorClassification
	Identity       SnapshotIdentity
	Artifact       string
	Cause          error
}

func (e *SnapshotInputError) Error() string {
	if e == nil {
		return "invalid factory snapshot input"
	}
	message := snapshotErrorSentinel(e.Classification).Error()
	if e.Identity != "" {
		message = fmt.Sprintf("%s %q", message, e.Identity)
	}
	if e.Artifact != "" {
		message = fmt.Sprintf("%s artifact %q", message, e.Artifact)
	}
	return message
}

func (e *SnapshotInputError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *SnapshotInputError) Is(target error) bool {
	if e == nil {
		return false
	}
	// Legacy root materialize callers classify a missing snapshot as an unsafe
	// materialization request. Preserve that compatibility match while callers
	// of Snapshots can inspect the more specific Missing classification.
	if e.Classification == SnapshotErrorMissing && target == ErrUnsafeFactorySnapshotMaterialize {
		return true
	}
	return target == snapshotErrorSentinel(e.Classification)
}

// NewSnapshotInputError constructs one stable typed snapshot failure.
func NewSnapshotInputError(
	classification SnapshotErrorClassification,
	identity SnapshotIdentity,
	artifact string,
	cause error,
) error {
	return &SnapshotInputError{
		Classification: classification,
		Identity:       identity,
		Artifact:       artifact,
		Cause:          cause,
	}
}

func snapshotErrorSentinel(classification SnapshotErrorClassification) error {
	switch classification {
	case SnapshotErrorMissing:
		return ErrFactorySnapshotMissing
	case SnapshotErrorIntegrity:
		return ErrFactorySnapshotIntegrity
	case SnapshotErrorUnsafe:
		return ErrUnsafeFactorySnapshotMaterialize
	case SnapshotErrorMalformed:
		fallthrough
	default:
		return ErrMalformedFactorySnapshot
	}
}

// SnapshotMaterializationFileSystem is the exact host filesystem effect used
// to stage and publish an already-validated snapshot directory. It is injected
// during composition and is never exposed by Snapshots operations.
type SnapshotMaterializationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	Lstat(string) (fs.FileInfo, error)
	Readlink(string) (string, error)
	EvalSymlinks(string) (string, error)
	WalkDir(string, fs.WalkDirFunc) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
	Chmod(string, fs.FileMode) error
	MkdirAll(string, fs.FileMode) error
	MkdirTemp(string, string) (string, error)
	Remove(string) error
	RemoveAll(string) error
	Rename(string, string) error
}

// FactorySnapshotConfigDecoder maps one detached snapshot into the canonical
// Factory configuration used for portable artifact validation and staging.
// It is an internal composition port, never an argument to Snapshots methods.
type FactorySnapshotConfigDecoder func(*FactorySnapshot) (*FactoryConfig, error)

// SnapshotIdentityOf returns the deterministic content identity of a
// detached snapshot. Existing declaration fields are excluded so a snapshot
// can safely carry its own identity and artifact-integrity facts.
func SnapshotIdentityOf(snapshot *FactorySnapshot) (SnapshotIdentity, error) {
	object, err := snapshotObject(snapshot)
	if err != nil {
		return "", err
	}
	removeSnapshotIntegrityDeclarations(object)
	canonical, err := json.Marshal(object)
	if err != nil {
		return "", fmt.Errorf("canonicalize factory snapshot: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return SnapshotIdentity("sha256:" + hex.EncodeToString(sum[:])), nil
}

func snapshotObject(snapshot *FactorySnapshot) (map[string]any, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("snapshot is required")
	}
	var object map[string]any
	if err := snapshot.Decode(&object); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if object == nil {
		return nil, fmt.Errorf("snapshot object is required")
	}
	return object, nil
}

func removeSnapshotIntegrityDeclarations(object map[string]any) {
	metadata, ok := object["metadata"].(map[string]any)
	if !ok {
		return
	}
	delete(metadata, snapshotIdentityMetadataKey)
	delete(metadata, snapshotArtifactsMetadataKey)
	if len(metadata) == 0 {
		delete(object, "metadata")
	}
}

// SealFactorySnapshot returns a detached snapshot with declared identity and
// bundled-artifact integrity facts derived from its complete portable content.
func SealFactorySnapshot(
	snapshot *FactorySnapshot,
	factoryConfig *FactoryConfig,
) (*FactorySnapshot, SnapshotIdentity, error) {
	object, err := snapshotObject(snapshot)
	if err != nil {
		return nil, "", err
	}
	removeSnapshotIntegrityDeclarations(object)
	artifacts, err := snapshotArtifactsIntegrity(factoryConfig)
	if err != nil {
		classification, artifact := snapshotArtifactFailure(err)
		return nil, "", NewSnapshotInputError(classification, "", artifact, err)
	}
	metadata, _ := object["metadata"].(map[string]any)
	if metadata == nil {
		metadata = make(map[string]any)
		object["metadata"] = metadata
	}
	metadata[snapshotArtifactsMetadataKey] = artifacts
	unsealed, err := NewFactorySnapshot(object)
	if err != nil {
		return nil, "", err
	}
	identity, err := SnapshotIdentityOf(unsealed)
	if err != nil {
		return nil, "", err
	}
	object["metadata"].(map[string]any)[snapshotIdentityMetadataKey] = string(identity)
	sealed, err := NewFactorySnapshot(object)
	if err != nil {
		return nil, "", err
	}
	return sealed, identity, nil
}

// VerifyFactorySnapshot validates any declared or expected identity and the
// complete bundled-artifact content before a snapshot can be materialized.
func VerifyFactorySnapshot(
	snapshot *FactorySnapshot,
	factoryConfig *FactoryConfig,
	expected SnapshotIdentity,
) (SnapshotIdentity, error) {
	identity, err := SnapshotIdentityOf(snapshot)
	if err != nil {
		return "", NewSnapshotInputError(SnapshotErrorMalformed, "", "", err)
	}
	object, err := snapshotObject(snapshot)
	if err != nil {
		return "", NewSnapshotInputError(SnapshotErrorMalformed, "", "", err)
	}
	metadata, _ := object["metadata"].(map[string]any)
	if declared, _ := metadata[snapshotIdentityMetadataKey].(string); declared != "" && SnapshotIdentity(declared) != identity {
		return "", NewSnapshotInputError(SnapshotErrorIntegrity, identity, "", fmt.Errorf("declared snapshot identity differs"))
	}
	if expected != "" && expected != identity {
		return "", NewSnapshotInputError(SnapshotErrorIntegrity, identity, "", fmt.Errorf("expected snapshot identity differs"))
	}
	artifacts, err := snapshotArtifactsIntegrity(factoryConfig)
	if err != nil {
		classification, artifact := snapshotArtifactFailure(err)
		return "", NewSnapshotInputError(classification, identity, artifact, err)
	}
	if declared, _ := metadata[snapshotArtifactsMetadataKey].(string); declared != "" && declared != artifacts {
		return "", NewSnapshotInputError(SnapshotErrorIntegrity, identity, "", fmt.Errorf("declared snapshot artifact integrity differs"))
	}
	return identity, nil
}

func snapshotArtifactsIntegrity(factoryConfig *FactoryConfig) (string, error) {
	if factoryConfig == nil || factoryConfig.ResourceManifest == nil || len(factoryConfig.ResourceManifest.BundledFiles) == 0 {
		return "sha256:" + hex.EncodeToString(sha256.New().Sum(nil)), nil
	}
	files := append([]BundledFileConfig(nil), factoryConfig.ResourceManifest.BundledFiles...)
	sort.Slice(files, func(left, right int) bool { return files[left].TargetPath < files[right].TargetPath })
	hash := sha256.New()
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if err := ValidatePortableBundledFileType(file); err != nil {
			return "", newSnapshotArtifactError(SnapshotErrorMalformed, file.TargetPath, err)
		}
		if err := ValidatePortableBundledFileTarget(file); err != nil {
			return "", newSnapshotArtifactError(SnapshotErrorMalformed, file.TargetPath, err)
		}
		if strings.TrimSpace(file.Content.Inline) == "" {
			return "", newSnapshotArtifactError(
				SnapshotErrorMissing,
				file.TargetPath,
				fmt.Errorf("inline content is required"),
			)
		}
		if _, exists := seen[file.TargetPath]; exists {
			return "", newSnapshotArtifactError(
				SnapshotErrorMalformed,
				file.TargetPath,
				fmt.Errorf("duplicate bundled artifact"),
			)
		}
		seen[file.TargetPath] = struct{}{}
		_, _ = hash.Write([]byte(file.TargetPath))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.Content.Inline))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type snapshotArtifactError struct {
	classification SnapshotErrorClassification
	artifact       string
	cause          error
}

func (e *snapshotArtifactError) Error() string {
	if e == nil || e.cause == nil {
		return "invalid snapshot artifact"
	}
	return e.cause.Error()
}

func (e *snapshotArtifactError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newSnapshotArtifactError(
	classification SnapshotErrorClassification,
	artifact string,
	cause error,
) error {
	return &snapshotArtifactError{
		classification: classification,
		artifact:       artifact,
		cause:          cause,
	}
}

func snapshotArtifactFailure(err error) (SnapshotErrorClassification, string) {
	var artifactErr *snapshotArtifactError
	if errors.As(err, &artifactErr) {
		return artifactErr.classification, artifactErr.artifact
	}
	return SnapshotErrorMalformed, ""
}
