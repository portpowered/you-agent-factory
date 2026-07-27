package contractstaging_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testpath"
)

const manifestTarget = "packages/api/generated/manifest.json"

func TestGeneratedManifestRecordsStableDevelopmentMetadata(t *testing.T) {
	t.Parallel()

	manifest := decodeRepositoryManifest(t)
	required := []string{
		"formatVersion",
		"packageId",
		"packageVersion",
		"familyFormatVersions",
		"exports",
	}
	for _, key := range required {
		if _, ok := manifest[key]; !ok {
			t.Fatalf("manifest missing %q", key)
		}
	}
	if manifest["formatVersion"] != "1.0.0" {
		t.Fatalf("formatVersion = %#v, want 1.0.0", manifest["formatVersion"])
	}
	if manifest["packageId"] != "you-agent-factory.api" {
		t.Fatalf("packageId = %#v, want you-agent-factory.api", manifest["packageId"])
	}
	if _, ok := manifest["sourceCommit"]; ok {
		t.Fatalf("development manifest contains commit-derived provenance: %#v", manifest["sourceCommit"])
	}
	families, ok := manifest["familyFormatVersions"].(map[string]any)
	if !ok || len(families) == 0 {
		t.Fatalf("familyFormatVersions = %#v", manifest["familyFormatVersions"])
	}
}

func TestGeneratedManifestHashesEveryStagedArtifact(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts := testArtifactsForRepository(t, repositoryRoot)
	manifest := decodeManifestPayload(t, artifacts[manifestTarget])
	exports := manifestExports(t, manifest)

	for repositoryPath, payload := range artifacts {
		if repositoryPath == manifestTarget {
			continue
		}
		if !strings.HasPrefix(repositoryPath, "packages/api/") {
			continue
		}
		packagePath := strings.TrimPrefix(repositoryPath, "packages/api/")
		export, ok := exportForPackagePath(exports, packagePath)
		if !ok {
			t.Fatalf("manifest missing export for staged artifact %s", repositoryPath)
		}
		wantDigest := fmt.Sprintf("%x", sha256.Sum256(payload))
		if got := export["artifactHash"]; got != wantDigest {
			t.Errorf("export %q artifactHash = %#v, want %q for %s", export["path"], got, wantDigest, repositoryPath)
		}
		documentation, ok := export["documentation"].(map[string]any)
		if !ok || documentation["sourceHash"] != wantDigest {
			t.Errorf("export %q documentation.sourceHash = %#v, want %q", export["path"], documentation["sourceHash"], wantDigest)
		}
	}
	if len(exports) != packageArtifactCount(artifacts) {
		t.Fatalf("export count = %d, want %d staged artifacts excluding manifest", len(exports), packageArtifactCount(artifacts))
	}
}

func TestManifestDigestsStableAcrossRepeatedArtifactsCalls(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	first := testArtifactsForRepository(t, repositoryRoot)
	second := testArtifactsForRepository(t, repositoryRoot)
	if !reflect.DeepEqual(first[manifestTarget], second[manifestTarget]) {
		t.Fatal("repeated Artifacts() changed manifest bytes")
	}
}

func TestManifestDigestChangesWhenStagedArtifactSourceChanges(t *testing.T) {
	root := checkFixture(t)
	before := testArtifactsForRepository(t, root)
	beforeDigest := exportDigestForPath(t, decodeManifestPayload(t, before[manifestTarget]), "generated/cli/commands.json")

	writeCheckFixture(t, root, "contracts/cli/commands.json", `{"changed":true}`)
	if err := contractstaging.Generate(root); err != nil {
		t.Fatalf("Generate() after source change: %v", err)
	}
	afterPayload, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(manifestTarget)))
	if err != nil {
		t.Fatalf("read regenerated manifest: %v", err)
	}
	afterDigest := exportDigestForPath(t, decodeManifestPayload(t, afterPayload), "generated/cli/commands.json")
	if beforeDigest == afterDigest {
		t.Fatalf("CLI artifact change did not change manifest digest: %q", beforeDigest)
	}
}

func TestMalformedManifestArtifactHashFailsJoinedManifestContract(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts := testArtifactsForRepository(t, repositoryRoot)
	manifestSchema := compileArtifactSchema(t, artifacts["packages/api/generated/joined/contracts/manifest.schema.json"])
	assertArtifactValid(t, manifestSchema, artifacts[manifestTarget], true)

	var manifest map[string]any
	if err := json.Unmarshal(artifacts[manifestTarget], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	exports := manifestExports(t, manifest)
	for id, exportValue := range exports {
		export, ok := exportValue.(map[string]any)
		if !ok {
			t.Fatalf("export %q = %#v", id, exportValue)
		}
		export["artifactHash"] = "SHA256:ABC123"
		exports[id] = export
		break
	}
	manifest["exports"] = exports
	tampered, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode malformed manifest: %v", err)
	}
	assertArtifactValid(t, manifestSchema, tampered, false)
}

func decodeRepositoryManifest(t *testing.T) map[string]any {
	t.Helper()
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	artifacts := testArtifactsForRepository(t, repositoryRoot)
	return decodeManifestPayload(t, artifacts[manifestTarget])
}

func decodeManifestPayload(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var manifest map[string]any
	if err := json.Unmarshal(payload, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func manifestExports(t *testing.T, manifest map[string]any) map[string]any {
	t.Helper()
	exports, ok := manifest["exports"].(map[string]any)
	if !ok {
		t.Fatalf("exports = %#v", manifest["exports"])
	}
	return exports
}

func packageArtifactCount(artifacts map[string][]byte) int {
	count := 0
	for path := range artifacts {
		if path == manifestTarget {
			continue
		}
		if strings.HasPrefix(path, "packages/api/") {
			count++
		}
	}
	return count
}

func exportForPackagePath(exports map[string]any, packagePath string) (map[string]any, bool) {
	for _, exportValue := range exports {
		export, ok := exportValue.(map[string]any)
		if !ok {
			continue
		}
		if export["path"] == packagePath {
			return export, true
		}
	}
	return nil, false
}

func exportDigestForPath(t *testing.T, manifest map[string]any, packagePath string) string {
	t.Helper()
	export, ok := exportForPackagePath(manifestExports(t, manifest), packagePath)
	if !ok {
		t.Fatalf("manifest missing export for %s", packagePath)
	}
	digest, ok := export["artifactHash"].(string)
	if !ok || digest == "" {
		t.Fatalf("export %s artifactHash = %#v", packagePath, export["artifactHash"])
	}
	return digest
}
