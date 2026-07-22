package contractstaging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestShouldRunArtifactCheck(t *testing.T) {
	if shouldRunArtifactCheck(nil, map[string]stagedFile{}) {
		t.Fatal("shouldRunArtifactCheck should be false for empty inputs")
	}
	if !shouldRunArtifactCheck(map[string][]byte{"a": []byte("1")}, nil) {
		t.Fatal("shouldRunArtifactCheck should be true when expected artifacts exist")
	}
	if !shouldRunArtifactCheck(map[string][]byte{}, map[string]stagedFile{"x": {regular: true}}) {
		t.Fatal("shouldRunArtifactCheck should be true when staged artifacts exist")
	}
}

func TestFilterArtifactsByStagingDirectory(t *testing.T) {
	actual := map[string]stagedFile{
		"packages/api/generated/keep.json": {regular: true},
		"packages/readme.md":               {regular: true},
		"other/path.txt":                   {regular: false},
	}
	got := filterArtifactsByStagingDirectory(actual)
	if _, ok := got["packages/api/generated/keep.json"]; !ok {
		t.Fatal("expected staged package artifact to be kept")
	}
	if _, ok := got["packages/readme.md"]; ok {
		t.Fatal("expected non-generated package artifact to be filtered")
	}
	if _, ok := got["other/path.txt"]; ok {
		t.Fatal("expected unrelated path to be filtered")
	}
}

func TestCompareArtifactsOrdersMissingAndUnexpectedPaths(t *testing.T) {
	repo := t.TempDir()
	homPath := filepath.Join(repo, filepath.FromSlash(FactorySchemaAuthoredPath))
	if err := os.MkdirAll(filepath.Dir(homPath), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(homPath, []byte("schema"), 0o644); err != nil {
		t.Fatalf("write authored schema fixture: %v", err)
	}
	expected := map[string][]byte{
		FactorySchemaAuthoredPath:         []byte("schema"),
		"packages/api/generated/zzz.json": []byte("A"),
		"packages/api/generated/aaa.json": []byte("B"),
	}
	actual := map[string]stagedFile{
		"packages/api/generated/zzz.json":     {regular: true, payload: []byte("A")},
		"packages/api/generated/aaa.json":     {regular: false, payload: []byte("wrong")},
		"packages/api/generated/outlier.json": {regular: true, payload: []byte("extra")},
	}
	drift := compareArtifacts(repo, expected, actual)
	if len(drift.Missing) != 0 {
		t.Fatalf("drift.Missing = %#v", drift.Missing)
	}
	if !reflect.DeepEqual(drift.Stale, []string{"packages/api/generated/aaa.json"}) {
		t.Fatalf("drift.Stale = %#v", drift.Stale)
	}
	if !reflect.DeepEqual(drift.Unexpected, []string{"packages/api/generated/outlier.json"}) {
		t.Fatalf("drift.Unexpected = %#v", drift.Unexpected)
	}
}

func TestVerifyManifestMetadataRejectsMissingSourceCommit(t *testing.T) {
	manifest := map[string]any{
		"formatVersion":        "1.0.0",
		"packageId":            "you-agent-factory.api",
		"packageVersion":       "0.0.0",
		"familyFormatVersions": map[string]any{"cli": "1.0.0"},
		"exports":              map[string]any{},
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	err = verifyManifestMetadata(map[string][]byte{manifestTarget: payload})
	if err == nil {
		t.Fatal("expected missing sourceCommit to fail")
	}
	if !strings.Contains(err.Error(), "sourceCommit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManifestMetadataAcceptsCanonicalFields(t *testing.T) {
	manifest := map[string]any{
		"formatVersion":        "1.0.0",
		"packageId":            "you-agent-factory.api",
		"packageVersion":       "0.0.0",
		"sourceCommit":         filepath.Base(t.TempDir()),
		"familyFormatVersions": map[string]any{"cli": "1.0.0"},
		"exports":              map[string]any{},
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	if err := verifyManifestMetadata(map[string][]byte{manifestTarget: payload}); err != nil {
		t.Fatalf("verifyManifestMetadata() = %v", err)
	}
}
