package models_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pinnedTTSFileIdentity struct {
	Configured  bool   `json:"configured"`
	RegularFile bool   `json:"regularFile"`
	Path        string `json:"path,omitempty"`
	Commit      string `json:"commit,omitempty"`
	Tree        string `json:"tree,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

func readPinnedTTSFileIdentity(path string) (pinnedTTSFileIdentity, bool) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return pinnedTTSFileIdentity{Configured: true}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return pinnedTTSFileIdentity{Configured: true, RegularFile: true}, false
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return pinnedTTSFileIdentity{Configured: true, RegularFile: true, Bytes: info.Size()}, false
	}
	return pinnedTTSFileIdentity{
		Configured:  true,
		RegularFile: true,
		Bytes:       info.Size(),
		SHA256:      hex.EncodeToString(hasher.Sum(nil)),
	}, true
}

func pinnedTTSCacheDirectoriesEmpty(modelCache, hfCache string) bool {
	for _, root := range []string{modelCache, hfCache} {
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 0 {
			return false
		}
	}
	return true
}

type pinnedTreeSnapshot struct {
	Entries map[string]string
}

func inspectPinnedTree(t testing.TB, root string) pinnedTreeSnapshot {
	t.Helper()
	snapshot := pinnedTreeSnapshot{Entries: make(map[string]string)}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return fs.SkipDir
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			snapshot.Entries[relative] = "dir"
			return nil
		}
		identity, ok := readPinnedTTSFileIdentity(path)
		if !ok {
			return fmt.Errorf("read cache snapshot entry %s", relative)
		}
		snapshot.Entries[relative] = fmt.Sprintf("file:%d:%s", identity.Bytes, identity.SHA256)
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect isolated cache %q: %v", root, err)
	}
	return snapshot
}

func (snapshot pinnedTreeSnapshot) Equal(other pinnedTreeSnapshot) bool {
	if len(snapshot.Entries) != len(other.Entries) {
		return false
	}
	for path, identity := range snapshot.Entries {
		if other.Entries[path] != identity {
			return false
		}
	}
	return true
}

func (snapshot pinnedTreeSnapshot) HasPartial() bool {
	for path := range snapshot.Entries {
		lower := strings.ToLower(filepath.ToSlash(path))
		if strings.HasSuffix(lower, ".partial") || strings.Contains(lower, "/.partial/") {
			return true
		}
	}
	return false
}

func pinnedTTSBlockedProxy(t testing.TB) (string, error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", err
	}
	return "http://" + address, nil
}

func mergePinnedTTSCleanup(first, second pinnedTTSCleanup) pinnedTTSCleanup {
	return pinnedTTSCleanup{
		Checked:                first.Checked && second.Checked,
		ProcessTreeAttached:    first.ProcessTreeAttached || second.ProcessTreeAttached,
		ProcessTreeClosed:      first.ProcessTreeClosed && second.ProcessTreeClosed,
		OwnedProcessRemaining:  first.OwnedProcessRemaining || second.OwnedProcessRemaining,
		OwnedListenerRemaining: first.OwnedListenerRemaining || second.OwnedListenerRemaining,
		Observation:            "all three supervised command trees exited; cache and output cleanup are checked after the exact-byte chain",
	}
}

type pinnedTTSModelIdentity struct {
	Name          string `json:"name"`
	Source        string `json:"source"`
	Revision      string `json:"revision"`
	Verified      bool   `json:"verified"`
	ArtifactCount int    `json:"artifactCount,omitempty"`
	ArtifactBytes int64  `json:"artifactBytes,omitempty"`
}

type pinnedTTSBackendIdentity struct {
	ID               string `json:"id"`
	SourceRepository string `json:"sourceRepository,omitempty"`
	BackendCommit    string `json:"backendCommit,omitempty"`
	VibeVoiceCommit  string `json:"vibeVoiceCommit"`
	LocalAICommit    string `json:"localAICommit"`
	ProtocolRevision string `json:"protocolRevision"`
	Archive          string `json:"archive"`
	ExpectedBytes    int64  `json:"expectedBytes"`
	ExpectedSHA256   string `json:"expectedSha256"`
	ObservedBytes    int64  `json:"observedBytes,omitempty"`
	ObservedSHA256   string `json:"observedSha256,omitempty"`
	Verified         bool   `json:"verified"`
}

type pinnedTTSManagedMetadata struct {
	ModelName string                  `json:"modelName"`
	Revision  string                  `json:"revision"`
	Files     []pinnedTTSMetadataFile `json:"files"`
}

type pinnedTTSMetadataFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func inspectPinnedTTSModelCache(cacheRoot string) (pinnedTTSModelIdentity, bool) {
	identity := pinnedTTSModelIdentity{
		Name: pinnedTTSModelName, Source: pinnedTTSModelSource, Revision: pinnedTTSModelRevision,
	}
	body, err := os.ReadFile(filepath.Join(cacheRoot, pinnedTTSManagedModelDir, pinnedTTSManagedMetadataName))
	if err != nil {
		return identity, false
	}
	var metadata pinnedTTSManagedMetadata
	if json.Unmarshal(body, &metadata) != nil ||
		!strings.EqualFold(metadata.ModelName, pinnedTTSManagedModelDir) ||
		metadata.Revision != pinnedTTSModelRevision || len(metadata.Files) == 0 {
		return identity, false
	}
	modelRoot := filepath.Join(cacheRoot, pinnedTTSManagedModelDir)
	revisionRoot, ok := pinnedTTSResolvedChild(modelRoot, metadata.Revision)
	if !ok {
		return identity, false
	}
	var totalBytes int64
	for _, file := range metadata.Files {
		artifactPath, pathOK := pinnedTTSResolvedChild(revisionRoot, file.Path)
		if !pathOK || file.Bytes <= 0 || !pinnedTTSHexDigest(file.SHA256) {
			return identity, false
		}
		observed, fileOK := readPinnedTTSFileIdentity(artifactPath)
		if !fileOK || observed.Bytes != file.Bytes || !strings.EqualFold(observed.SHA256, file.SHA256) {
			return identity, false
		}
		totalBytes += observed.Bytes
	}
	identity.Verified = true
	identity.ArtifactCount = len(metadata.Files)
	identity.ArtifactBytes = totalBytes
	return identity, true
}

func inspectPinnedExactModelCache(
	cacheRoot, modelDirectory, revision string, expected []pinnedTTSAssetRequirement,
) (pinnedTTSModelIdentity, bool) {
	identity := pinnedTTSModelIdentity{
		Name: strings.ToLower(modelDirectory), Revision: revision,
	}
	if strings.EqualFold(modelDirectory, pinnedTTSManagedModelDir) {
		identity.Source = pinnedTTSModelSource
	} else if strings.EqualFold(modelDirectory, pinnedASRManagedModelDir) {
		identity.Source = pinnedASRModelSource
	}
	body, err := os.ReadFile(filepath.Join(cacheRoot, modelDirectory, pinnedTTSManagedMetadataName))
	if err != nil {
		return identity, false
	}
	var metadata pinnedTTSManagedMetadata
	if json.Unmarshal(body, &metadata) != nil ||
		!strings.EqualFold(metadata.ModelName, modelDirectory) ||
		metadata.Revision != revision || len(metadata.Files) != len(expected) {
		return identity, false
	}
	modelRoot := filepath.Join(cacheRoot, modelDirectory)
	revisionRoot, ok := pinnedTTSResolvedChild(modelRoot, metadata.Revision)
	if !ok {
		return identity, false
	}
	want := make(map[string]pinnedTTSAssetRequirement, len(expected))
	for _, artifact := range expected {
		want[filepath.ToSlash(artifact.Name)] = artifact
	}
	var totalBytes int64
	for _, file := range metadata.Files {
		name := filepath.ToSlash(strings.TrimSpace(file.Path))
		artifact, wanted := want[name]
		if !wanted || file.Bytes != artifact.Bytes || !strings.EqualFold(file.SHA256, artifact.SHA256) {
			return identity, false
		}
		artifactPath, pathOK := pinnedTTSResolvedChild(revisionRoot, file.Path)
		if !pathOK {
			return identity, false
		}
		observed, fileOK := readPinnedTTSFileIdentity(artifactPath)
		if !fileOK || observed.Bytes != artifact.Bytes || !strings.EqualFold(observed.SHA256, artifact.SHA256) {
			return identity, false
		}
		totalBytes += observed.Bytes
	}
	identity.Verified = true
	identity.ArtifactCount = len(metadata.Files)
	identity.ArtifactBytes = totalBytes
	return identity, true
}

func pinnedTTSResolvedChild(root, relative string) (string, bool) {
	relative = strings.TrimSpace(relative)
	if relative == "" || filepath.IsAbs(relative) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	path := filepath.Join(root, clean)
	pathResolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	relativeResolved, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil || relativeResolved == ".." || strings.HasPrefix(relativeResolved, ".."+string(filepath.Separator)) {
		return "", false
	}
	return path, true
}

func TestPinnedTTSModelCacheRejectsForgedMetadata(t *testing.T) {
	t.Parallel()

	cacheRoot := t.TempDir()
	revisionRoot := filepath.Join(cacheRoot, pinnedTTSManagedModelDir, pinnedTTSModelRevision)
	if err := os.MkdirAll(revisionRoot, 0o755); err != nil {
		t.Fatalf("create model cache fixture: %v", err)
	}
	body := []byte("actual pinned model bytes")
	artifactPath := filepath.Join(revisionRoot, "weights.bin")
	if err := os.WriteFile(artifactPath, body, 0o644); err != nil {
		t.Fatalf("write model cache artifact: %v", err)
	}
	actualDigest := sha256.Sum256(body)
	metadata := pinnedTTSManagedMetadata{
		ModelName: pinnedTTSModelName,
		Revision:  pinnedTTSModelRevision,
		Files: []pinnedTTSMetadataFile{{
			Path: "weights.bin", Bytes: int64(len(body)),
			SHA256: hex.EncodeToString(actualDigest[:]),
		}},
	}
	metadataBody, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal forged model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, pinnedTTSManagedModelDir, pinnedTTSManagedMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write model metadata: %v", err)
	}
	if _, ok := inspectPinnedTTSModelCache(cacheRoot); !ok {
		t.Fatal("inspectPinnedTTSModelCache() rejected metadata matching the actual revision artifact")
	}

	metadata.Files[0].Bytes++
	metadataBody, err = json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal forged model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheRoot, pinnedTTSManagedModelDir, pinnedTTSManagedMetadataName), metadataBody, 0o644); err != nil {
		t.Fatalf("write forged model metadata: %v", err)
	}
	if _, ok := inspectPinnedTTSModelCache(cacheRoot); ok {
		t.Fatal("inspectPinnedTTSModelCache() accepted metadata whose declared size differs from the actual artifact")
	}
}

type pinnedTTSAssetMetadata struct {
	Kind      string                      `json:"kind"`
	Artifacts []pinnedTTSAssetRequirement `json:"artifacts"`
}

type pinnedTTSAssetRequirement struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func inspectPinnedTTSBackendCache(cacheRoot string) (pinnedTTSBackendIdentity, bool) {
	return inspectPinnedBackendCache(cacheRoot, pinnedTTSBackendArchive, pinnedTTSBackendBytes, pinnedTTSBackendSHA256)
}

func inspectPinnedBackendCache(
	cacheRoot, archive string, expectedBytes int64, expectedSHA256 string,
) (pinnedTTSBackendIdentity, bool) {
	identity := pinnedTTSBackendIdentity{Archive: archive, ExpectedBytes: expectedBytes, ExpectedSHA256: expectedSHA256}
	switch {
	case archive == pinnedTTSBackendArchive:
		identity.ID = pinnedTTSBackendID
		identity.SourceRepository = "https://github.com/mudler/vibevoice.cpp"
		identity.BackendCommit = pinnedTTSVibeVoiceCommit
		identity.VibeVoiceCommit = pinnedTTSVibeVoiceCommit
	case archive == pinnedASRBackendArchive:
		identity.ID = pinnedASRBackendID
		identity.SourceRepository = "https://github.com/ggml-org/whisper.cpp"
		identity.BackendCommit = pinnedASRWhisperCommit
	}
	identity.LocalAICommit = pinnedTTSLocalAICommit
	identity.ProtocolRevision = pinnedTTSProtocolRevision
	backendRoot := filepath.Join(cacheRoot, "backend-artifacts", ".you-content-addressed", "backend")
	var verified bool
	err := filepath.WalkDir(backendRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != pinnedTTSMetadataName {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var metadata pinnedTTSAssetMetadata
		if json.Unmarshal(body, &metadata) != nil || metadata.Kind != "backend" {
			return nil
		}
		for _, artifact := range metadata.Artifacts {
			if filepath.ToSlash(artifact.Name) != archive ||
				artifact.Bytes != expectedBytes ||
				!strings.EqualFold(artifact.SHA256, expectedSHA256) {
				continue
			}
			artifactPath := filepath.Join(filepath.Dir(path), filepath.FromSlash(artifact.Name))
			fileIdentity, ok := readPinnedTTSFileIdentity(artifactPath)
			if !ok || fileIdentity.Bytes != expectedBytes ||
				!strings.EqualFold(fileIdentity.SHA256, expectedSHA256) {
				continue
			}
			identity.ObservedBytes = fileIdentity.Bytes
			identity.ObservedSHA256 = fileIdentity.SHA256
			identity.Verified = true
			verified = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil && err != fs.SkipAll {
		return identity, false
	}
	return identity, verified
}

func pinnedTTSHexDigest(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
