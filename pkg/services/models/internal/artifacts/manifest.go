// Package artifacts decodes and selects the immutable backend publication
// consumed by Models. It intentionally contains no downloader, cache, process,
// protocol client, or activation behavior.
package artifacts

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"

	backendregistry "github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"
)

const manifestSchemaVersion = 1

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	namePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	tokenPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type targetFacts struct {
	operatingSystem string
	architecture    string
	accelerators    []string
}

var supportedTargets = map[string]targetFacts{
	"darwin-arm64": {
		operatingSystem: "darwin",
		architecture:    "arm64",
		accelerators:    []string{"metal"},
	},
	"linux-amd64": {
		operatingSystem: "linux",
		architecture:    "amd64",
		accelerators:    []string{"cpu"},
	},
	"windows-amd64": {
		operatingSystem: "windows",
		architecture:    "amd64",
		accelerators:    []string{"cpu"},
	},
}

// SelectionRequest identifies one exact backend capability request.
type SelectionRequest struct {
	Backend          string
	OperatingSystem  string
	Architecture     string
	ProtocolRevision string
	Accelerator      string
}

// BackendIdentity preserves the pinned backend identity without exposing a
// LocalAI package or protocol type.
type BackendIdentity struct {
	ID                string
	SourceRepository  string
	SourceCommit      string
	SourcePinVariable string
}

// SourceIdentity preserves the immutable LocalAI source provenance.
type SourceIdentity struct {
	Repository string
	Commit     string
	Path       string
}

// ProtocolIdentity preserves the protocol path and immutable revision.
type ProtocolIdentity struct {
	Path     string
	Revision string
}

// TargetCompatibility preserves the target and declared accelerator facts.
type TargetCompatibility struct {
	ID              string
	OperatingSystem string
	Architecture    string
	Accelerators    []string
}

// ArtifactIdentity preserves the immutable archive identity and integrity
// facts needed by later download and verification work.
type ArtifactIdentity struct {
	Name      string
	Location  string
	SizeBytes int64
	SHA256    string
}

// PublicationIdentity preserves the immutable release identity that owns the
// selected archive.
type PublicationIdentity struct {
	ID                string
	ReleaseTag        string
	Repository        string
	PinFingerprint    string
	PackagingRevision int
}

// ArtifactDescriptor is one detached, provider-neutral selection result.
type ArtifactDescriptor struct {
	ID          string
	Publication PublicationIdentity
	Backend     BackendIdentity
	Source      SourceIdentity
	Protocol    ProtocolIdentity
	Target      TargetCompatibility
	Artifact    ArtifactIdentity
}

type publication struct {
	identity   PublicationIdentity
	repository string
	releaseTag string
	source     SourceIdentity
	protocol   ProtocolIdentity
}

type manifestEntry struct {
	id       string
	backend  BackendIdentity
	source   SourceIdentity
	protocol ProtocolIdentity
	target   TargetCompatibility
	artifact ArtifactIdentity
}

// Manifest is a validated, immutable snapshot of the P1 publication shape.
// Its wire representation stays private so callers cannot bypass validation.
type Manifest struct {
	publication publication
	entries     []manifestEntry
}

type wireManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Kind          string          `json:"kind"`
	Publication   wirePublication `json:"publication"`
	Artifacts     []wireArtifact  `json:"artifacts"`
}

type wirePublication struct {
	ID                string        `json:"id"`
	ReleaseTag        string        `json:"releaseTag"`
	Repository        string        `json:"repository"`
	PinFingerprint    string        `json:"pinFingerprint"`
	PackagingRevision *int          `json:"packagingRevision"`
	Source            wireSource    `json:"source"`
	Protocol          wireProtocol  `json:"protocol"`
	Toolchain         wireToolchain `json:"toolchain"`
}

type wireToolchain struct {
	GoVersion       string `json:"goVersion"`
	CMakeVersion    string `json:"cmakeVersion"`
	ProtobufVersion string `json:"protobufVersion"`
	GRPCVersion     string `json:"grpcVersion"`
	GRPCCommit      string `json:"grpcCommit"`
	VCPKGCommit     string `json:"vcpkgCommit"`
}

type wireArtifact struct {
	ID       string       `json:"id"`
	Backend  wireBackend  `json:"backend"`
	Source   wireSource   `json:"source"`
	Protocol wireProtocol `json:"protocol"`
	Target   wireTarget   `json:"target"`
	Artifact wireArchive  `json:"artifact"`
}

type wireBackend struct {
	ID     string            `json:"id"`
	Source wireBackendSource `json:"source"`
}

type wireBackendSource struct {
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	PinVariable string `json:"pinVariable"`
}

type wireSource struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
	Path       string `json:"path,omitempty"`
}

type wireProtocol struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}

type wireTarget struct {
	ID              string   `json:"id"`
	OperatingSystem string   `json:"operatingSystem"`
	Architecture    string   `json:"architecture"`
	Accelerators    []string `json:"accelerators"`
}

type wireArchive struct {
	Name      string `json:"name"`
	Location  string `json:"location"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

// Decode validates and snapshots one P1-shaped manifest without performing IO.
func Decode(data []byte) (Manifest, error) {
	var raw wireManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return Manifest{}, failuref(FailureMalformedManifest, "document", "", "decode JSON: %v", err)
	}
	if err := rejectTrailingJSON(decoder); err != nil {
		return Manifest{}, err
	}
	return validateManifest(raw)
}

// DecodeManifest is the descriptive alias for Decode.
func DecodeManifest(data []byte) (Manifest, error) {
	return Decode(data)
}

func rejectTrailingJSON(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return failuref(FailureMalformedManifest, "document", "", "decode trailing JSON: %v", err)
	}
	return failure(FailureMalformedManifest, "document", "", "contains more than one JSON value")
}

func validateManifest(raw wireManifest) (Manifest, error) {
	if raw.SchemaVersion != manifestSchemaVersion {
		return Manifest{}, failuref(FailureMalformedManifest, "schemaVersion", "", "must be %d", manifestSchemaVersion)
	}
	if raw.Kind != "localai-backend-artifacts" {
		return Manifest{}, failuref(FailureMalformedManifest, "kind", raw.Kind, "must be localai-backend-artifacts")
	}
	publication, err := validatePublication(raw.Publication)
	if err != nil {
		return Manifest{}, err
	}
	if len(raw.Artifacts) == 0 {
		return Manifest{}, failure(FailureMalformedManifest, "artifacts", "", "must contain at least one entry")
	}

	entries := make([]manifestEntry, 0, len(raw.Artifacts))
	seenIDs := make(map[string]struct{}, len(raw.Artifacts))
	for index, rawEntry := range raw.Artifacts {
		entry, entryErr := validateEntry(rawEntry, publication, index)
		if entryErr != nil {
			return Manifest{}, entryErr
		}
		if _, duplicate := seenIDs[entry.id]; duplicate {
			return Manifest{}, failuref(FailureDuplicateArtifact, artifactField(index, "id"), entry.id, "identity must be unique")
		}
		seenIDs[entry.id] = struct{}{}
		entries = append(entries, entry)
	}
	return Manifest{publication: publication, entries: entries}, nil
}

func validatePublication(raw wirePublication) (publication, error) {
	if !validToken(raw.ID) || raw.ID != raw.ReleaseTag {
		return publication{}, failure(FailureMalformedManifest, "publication.releaseTag", raw.ReleaseTag, "id and releaseTag must be the same safe value")
	}
	if !validRepositoryName(raw.Repository) {
		return publication{}, failure(FailureMalformedManifest, "publication.repository", raw.Repository, "must be an owner/name identifier")
	}
	if !digestPattern.MatchString(raw.PinFingerprint) {
		return publication{}, failure(FailureMalformedManifest, "publication.pinFingerprint", raw.PinFingerprint, "must be a lowercase 64-character SHA-256")
	}
	packagingRevision := 0
	if raw.PackagingRevision != nil {
		if *raw.PackagingRevision <= 0 {
			return publication{}, failure(FailureMalformedManifest, "publication.packagingRevision", strconv.Itoa(*raw.PackagingRevision), "must be a positive integer")
		}
		packagingRevision = *raw.PackagingRevision
	}
	source, err := validateSource(raw.Source, "publication.source", false)
	if err != nil {
		return publication{}, err
	}
	protocol, err := validateProtocol(raw.Protocol, "publication.protocol")
	if err != nil {
		return publication{}, err
	}
	if err := validateToolchain(raw.Toolchain); err != nil {
		return publication{}, err
	}
	return publication{
		identity: PublicationIdentity{
			ID: raw.ID, ReleaseTag: raw.ReleaseTag, Repository: raw.Repository,
			PinFingerprint: raw.PinFingerprint, PackagingRevision: packagingRevision,
		},
		repository: raw.Repository, releaseTag: raw.ReleaseTag, source: source, protocol: protocol,
	}, nil
}

func validateToolchain(toolchain wireToolchain) error {
	values := map[string]string{
		"goVersion": toolchain.GoVersion, "cmakeVersion": toolchain.CMakeVersion,
		"protobufVersion": toolchain.ProtobufVersion, "grpcVersion": toolchain.GRPCVersion,
	}
	for field, value := range values {
		if !validToken(value) {
			return failure(FailureMalformedManifest, "publication.toolchain."+field, value, "must be non-empty")
		}
	}
	for field, value := range map[string]string{"grpcCommit": toolchain.GRPCCommit, "vcpkgCommit": toolchain.VCPKGCommit} {
		if !commitPattern.MatchString(value) {
			return failure(FailureMalformedManifest, "publication.toolchain."+field, value, "must be a lowercase 40-character commit SHA")
		}
	}
	return nil
}

func validateEntry(raw wireArtifact, publication publication, index int) (manifestEntry, error) {
	prefix := fmtArtifactIndex(index)
	if !validIdentity(raw.ID) {
		return manifestEntry{}, failure(FailureMalformedManifest, prefix+".id", raw.ID, "must be a non-empty safe identity")
	}
	facts, known := backendregistry.LookupArtifact(raw.Backend.ID)
	if !known {
		return manifestEntry{}, failure(FailureUnknownBackend, prefix+".backend.id", raw.Backend.ID, "backend is outside the supported LocalAI set")
	}
	if raw.Backend.Source.Repository != facts.Artifact.SourceRepository {
		return manifestEntry{}, failure(FailureMalformedManifest, prefix+".backend.source.repository", raw.Backend.Source.Repository, "does not match the pinned backend identity")
	}
	if !commitPattern.MatchString(raw.Backend.Source.Commit) || !validToken(raw.Backend.Source.PinVariable) {
		return manifestEntry{}, failure(FailureMalformedManifest, prefix+".backend.source", "", "requires a lowercase source commit SHA and pin variable")
	}
	source, err := validateSource(raw.Source, prefix+".source", true)
	if err != nil {
		return manifestEntry{}, err
	}
	if source.Repository != publication.source.Repository || source.Commit != publication.source.Commit || source.Path != facts.Artifact.SourcePath {
		return manifestEntry{}, failure(FailureMalformedManifest, prefix+".source", "", "does not match publication or backend provenance")
	}
	protocol, err := validateProtocol(raw.Protocol, prefix+".protocol")
	if err != nil {
		return manifestEntry{}, err
	}
	if protocol != publication.protocol {
		return manifestEntry{}, failure(FailureMalformedManifest, prefix+".protocol", "", "does not match publication protocol provenance")
	}
	target, err := validateTarget(raw.Target, prefix+".target")
	if err != nil {
		return manifestEntry{}, err
	}
	archive, err := validateArchive(raw.Artifact, publication, prefix+".artifact")
	if err != nil {
		return manifestEntry{}, err
	}
	return manifestEntry{
		id: raw.ID,
		backend: BackendIdentity{
			ID: raw.Backend.ID, SourceRepository: raw.Backend.Source.Repository,
			SourceCommit: raw.Backend.Source.Commit, SourcePinVariable: raw.Backend.Source.PinVariable,
		},
		source: source, protocol: protocol, target: target, artifact: archive,
	}, nil
}

func validateSource(raw wireSource, field string, requirePath bool) (SourceIdentity, error) {
	if !isGitHubRepository(raw.Repository) || !commitPattern.MatchString(raw.Commit) {
		return SourceIdentity{}, failure(FailureMalformedManifest, field, "", "requires a GitHub repository URL and lowercase commit SHA")
	}
	if requirePath && !validRelativePath(raw.Path) {
		return SourceIdentity{}, failure(FailureMalformedManifest, field+".path", raw.Path, "must be a safe POSIX relative path")
	}
	return SourceIdentity{Repository: raw.Repository, Commit: raw.Commit, Path: raw.Path}, nil
}

func validateProtocol(raw wireProtocol, field string) (ProtocolIdentity, error) {
	if !validRelativePath(raw.Path) || !commitPattern.MatchString(raw.Revision) {
		return ProtocolIdentity{}, failure(FailureMalformedManifest, field, "", "requires a safe protocol path and lowercase revision SHA")
	}
	return ProtocolIdentity{Path: raw.Path, Revision: raw.Revision}, nil
}

func validateTarget(raw wireTarget, field string) (TargetCompatibility, error) {
	facts, ok := supportedTargets[raw.ID]
	if !ok || raw.OperatingSystem != facts.operatingSystem || raw.Architecture != facts.architecture {
		return TargetCompatibility{}, failure(FailureUnsupportedPlatform, field, raw.ID, "target is outside the supported darwin-arm64, linux-amd64, windows-amd64 set")
	}
	if !sameStrings(raw.Accelerators, facts.accelerators) {
		return TargetCompatibility{}, failure(FailureUnsupportedPlatform, field+".accelerators", strings.Join(raw.Accelerators, ","), "accelerators do not match the pinned target")
	}
	return TargetCompatibility{
		ID: raw.ID, OperatingSystem: raw.OperatingSystem, Architecture: raw.Architecture,
		Accelerators: append([]string(nil), raw.Accelerators...),
	}, nil
}

func validateArchive(raw wireArchive, publication publication, field string) (ArtifactIdentity, error) {
	if !namePattern.MatchString(raw.Name) {
		return ArtifactIdentity{}, failure(FailureMalformedManifest, field+".name", raw.Name, "must be a safe archive file name")
	}
	if raw.SizeBytes <= 0 {
		return ArtifactIdentity{}, failure(FailureInvalidSize, field+".sizeBytes", "", "must be a positive integer")
	}
	if !digestPattern.MatchString(raw.SHA256) {
		return ArtifactIdentity{}, failure(FailureInvalidDigest, field+".sha256", raw.SHA256, "must be a lowercase 64-character SHA-256")
	}
	if err := validateLocation(raw.Location, publication, raw.Name, field+".location"); err != nil {
		return ArtifactIdentity{}, err
	}
	return ArtifactIdentity{Name: raw.Name, Location: raw.Location, SizeBytes: raw.SizeBytes, SHA256: raw.SHA256}, nil
}

func validateLocation(location string, publication publication, name, field string) error {
	parsed, err := url.Parse(location)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return failure(FailureUnsafeLocation, field, location, "must be an HTTPS GitHub release URL")
	}
	expectedPath := "/" + publication.repository + "/releases/download/" + publication.releaseTag + "/" + name
	if parsed.Path != expectedPath || path.Clean(parsed.Path) != parsed.Path {
		return failure(FailureUnsafeLocation, field, location, "must reference the approved immutable publication archive")
	}
	return nil
}

func isGitHubRepository(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return false
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if strings.HasSuffix(name, ".git") {
		name = strings.TrimSuffix(name, ".git")
	}
	parts := strings.Split(name, "/")
	return len(parts) == 2 && validToken(parts[0]) && validToken(parts[1])
}

func validRepositoryName(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && validToken(parts[0]) && validToken(parts[1])
}

func validToken(value string) bool {
	return tokenPattern.MatchString(value)
}

func validIdentity(value string) bool {
	return value != "" && !strings.Contains(value, "\\") && !strings.Contains(value, "..") && !hasControl(value)
}

func validRelativePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || hasControl(value) {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func hasControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func artifactField(index int, field string) string {
	return "artifacts[" + strconv.Itoa(index) + "]." + field
}

func fmtArtifactIndex(index int) string {
	return "artifacts[" + strconv.Itoa(index) + "]"
}
