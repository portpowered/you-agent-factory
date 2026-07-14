package climanifestgen_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestExtractFactoryConfigInitFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractFactoryConfigInitFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractFactoryConfigInitFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.FactoryConfigInitFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.FactoryConfigInitFamilyCommandIDs))
	}
	for _, id := range climanifestgen.FactoryConfigInitFamilyCommandIDs {
		record, ok := family.Commands[id]
		if !ok {
			t.Fatalf("missing factory/config/init command %q", id)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
		if record.Path == "" {
			t.Fatalf("command %q missing path", id)
		}
	}
}

func TestExtractFactoryConfigInitFamilyRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{
			"you.factory": {ID: "you.factory", Path: "you factory"},
		},
	}
	if _, err := climanifestgen.ExtractFactoryConfigInitFamily(manifest); err == nil {
		t.Fatal("expected missing factory/config/init command error")
	}
}

func TestAssertFactoryConfigInitFamilyCommandIDRejectsOutOfFamily(t *testing.T) {
	if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID("you.session.show"); err == nil {
		t.Fatal("expected out-of-family command id rejection")
	}
	if err := climanifestgen.AssertFactoryConfigInitFamilyCommandID("you.factory.query"); err != nil {
		t.Fatalf("AssertFactoryConfigInitFamilyCommandID(you.factory.query) error = %v", err)
	}
}

func TestExtractRepresentativeFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractRepresentativeFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractRepresentativeFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.RepresentativeFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.RepresentativeFamilyCommandIDs))
	}
	for _, id := range climanifestgen.RepresentativeFamilyCommandIDs {
		record, ok := family.Commands[id]
		if !ok {
			t.Fatalf("missing representative-family command %q", id)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
		if record.Path == "" {
			t.Fatalf("command %q missing path", id)
		}
	}
}

func TestExtractRepresentativeFamilyRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{
			"you": {ID: "you", Path: "you"},
		},
	}
	if _, err := climanifestgen.ExtractRepresentativeFamily(manifest); err == nil {
		t.Fatal("expected missing representative-family command error")
	}
}

func TestAssertRepresentativeFamilyCommandIDRejectsOutOfFamily(t *testing.T) {
	if err := climanifestgen.AssertRepresentativeFamilyCommandID("you.session.list"); err == nil {
		t.Fatal("expected out-of-family command id rejection")
	}
	if err := climanifestgen.AssertRepresentativeFamilyCommandID("you.session.show"); err != nil {
		t.Fatalf("AssertRepresentativeFamilyCommandID(you.session.show) error = %v", err)
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}

	if err := climanifestgen.Generate(root); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	first := fileDigests(t, root, []string{
		climanifestgen.RepresentativeFamilyJSONPath,
		climanifestgen.RepresentativeFamilyCommandIDsPath,
		climanifestgen.FactoryConfigInitFamilyJSONPath,
		climanifestgen.FactoryConfigInitFamilyCommandIDsPath,
	})
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	second := fileDigests(t, root, []string{
		climanifestgen.RepresentativeFamilyJSONPath,
		climanifestgen.RepresentativeFamilyCommandIDsPath,
		climanifestgen.FactoryConfigInitFamilyJSONPath,
		climanifestgen.FactoryConfigInitFamilyCommandIDsPath,
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated generation changed artifact digests:\nfirst=%v\nsecond=%v", first, second)
	}
}

func TestCheckDetectsStaleRepresentativeFamilyArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	target := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected stale representative-family artifact drift")
	}
	if len(drift.Stale) != 1 || drift.Stale[0] != climanifestgen.RepresentativeFamilyJSONPath {
		t.Fatalf("stale drift = %#v, want %q stale", drift, climanifestgen.RepresentativeFamilyJSONPath)
	}
}

func copyFile(source, target string) error {
	payload, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, payload, 0o644)
}

func fileDigests(t *testing.T, root string, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	digests := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		digests[path] = sha256.Sum256(content)
	}
	return digests
}

func TestProductionFactoryConfigInitFamilyArtifactsMatchGenerator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	drift, err := climanifestgen.Check(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production factory/config/init artifacts drift: %#v", drift)
	}

	payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(climanifestgen.FactoryConfigInitFamilyJSONPath)))
	if err != nil {
		t.Fatalf("read factory/config/init family artifact: %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("factory/config/init family artifact is empty")
	}
}

func TestProductionRepresentativeFamilyArtifactsMatchGenerator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	drift, err := climanifestgen.Check(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production representative-family artifacts drift: %#v", drift)
	}

	payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath)))
	if err != nil {
		t.Fatalf("read representative family artifact: %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("representative family artifact is empty")
	}
}

func TestExtractRepresentativeFamilyRejectsEmptyManifest(t *testing.T) {
	if _, err := climanifestgen.ExtractRepresentativeFamily(climanifest.Manifest{}); err == nil {
		t.Fatal("expected empty manifest rejection")
	}
}

func TestCheckTreatsCRLFArtifactsAsCurrent(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	jsonTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	payload, err := os.ReadFile(jsonTarget)
	if err != nil {
		t.Fatalf("read generated json: %v", err)
	}
	if err := os.WriteFile(jsonTarget, bytes.ReplaceAll(payload, []byte("\n"), []byte("\r\n")), 0o644); err != nil {
		t.Fatalf("write CRLF artifact: %v", err)
	}

	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("CRLF artifacts drift = %#v, want current", drift)
	}
}

func TestCheckDetectsMissingRepresentativeFamilyArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := climanifestgen.Generate(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	jsonTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	if err := os.Remove(jsonTarget); err != nil {
		t.Fatalf("remove generated json: %v", err)
	}

	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() || len(drift.Missing) == 0 {
		t.Fatalf("missing drift = %#v, want missing generated artifacts", drift)
	}
}
