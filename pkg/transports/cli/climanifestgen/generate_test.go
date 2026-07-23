package climanifestgen_test

import (
	"bytes"
	"crypto/sha256"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

type testFileSystem struct{}

func (testFileSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (testFileSystem) Remove(path string) error             { return os.Remove(path) }
func (testFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (testFileSystem) WriteFile(path string, payload []byte, mode fs.FileMode) error {
	return os.WriteFile(path, payload, mode)
}
func (testFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func artifactStore(t *testing.T) generatedartifacts.LocalStore {
	t.Helper()
	return mustArtifactStore()
}

func mustArtifactStore() generatedartifacts.LocalStore {
	store, err := generatedartifacts.NewLocalStore(testFileSystem{})
	if err != nil {
		panic(err)
	}
	return store
}

func TestExtractFactoryConfigInitFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
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
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
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

func TestExtractSessionFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractSessionFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractSessionFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.SessionFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.SessionFamilyCommandIDs))
	}
	for _, id := range climanifestgen.SessionFamilyCommandIDs {
		record, ok := family.Commands[id]
		if !ok || record.ID != id {
			t.Fatalf("session command %q = %#v", id, record)
		}
	}
}

func TestExtractSessionFamilyRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{"you.session": {ID: "you.session"}},
	}
	if _, err := climanifestgen.ExtractSessionFamily(manifest); err == nil || !strings.Contains(err.Error(), "you.session.create") {
		t.Fatalf("ExtractSessionFamily() error = %v, want stable missing command ID", err)
	}
}

func TestAssertSessionFamilyCommandIDRejectsCrossFamilyID(t *testing.T) {
	if err := climanifestgen.AssertSessionFamilyCommandID("you.work.list"); err == nil {
		t.Fatal("expected cross-family command rejection")
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

func TestAssertWorkFamilyCommandIDRejectsOutOfFamily(t *testing.T) {
	if err := climanifestgen.AssertWorkFamilyCommandID("you.work.submit"); err == nil {
		t.Fatal("expected out-of-family command id rejection")
	}
	if err := climanifestgen.AssertWorkFamilyCommandID("you.work.list"); err != nil {
		t.Fatalf("AssertWorkFamilyCommandID(you.work.list) error = %v", err)
	}
}

func TestExtractWorkFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractWorkFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractWorkFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.WorkFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.WorkFamilyCommandIDs))
	}
	for _, id := range climanifestgen.WorkFamilyCommandIDs {
		record, ok := family.Commands[id]
		if !ok {
			t.Fatalf("missing work-family command %q", id)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
		if record.Path == "" {
			t.Fatalf("command %q missing path", id)
		}
	}
}

func TestExtractWorkFamilyRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{
			"you.work": {ID: "you.work", Path: "you work"},
		},
	}
	if _, err := climanifestgen.ExtractWorkFamily(manifest); err == nil {
		t.Fatal("expected missing work-family command error")
	}
}

func TestExtractWorkFamilyRejectsEmptyManifest(t *testing.T) {
	if _, err := climanifestgen.ExtractWorkFamily(climanifest.Manifest{}); err == nil {
		t.Fatal("expected empty manifest rejection")
	}
}

func TestExtractRunSubmitFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractRunSubmitFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractRunSubmitFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.RunSubmitFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.RunSubmitFamilyCommandIDs))
	}
	for _, id := range climanifestgen.RunSubmitFamilyCommandIDs {
		if record, ok := family.Commands[id]; !ok || record.ID != id {
			t.Fatalf("run/submit command %q = %#v, present = %t", id, record, ok)
		}
	}
}

func TestExtractRunSubmitFamilyRejectsMissingAndOutOfFamilyCommands(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{
			"you.run": {ID: "you.run", Path: "you run"},
		},
	}
	if _, err := climanifestgen.ExtractRunSubmitFamily(manifest); err == nil {
		t.Fatal("ExtractRunSubmitFamily() missing submit commands = nil, want error")
	}
	if err := climanifestgen.AssertRunSubmitFamilyCommandID("you.work.list"); err == nil {
		t.Fatal("AssertRunSubmitFamilyCommandID() out-of-family command = nil, want error")
	}
}

func TestProductionWorkFamilyArtifactsMatchGenerator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	drift, err := checkCLIArtifacts(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production work-family artifacts drift: %#v", drift)
	}

	payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath)))
	if err != nil {
		t.Fatalf("read typed family manifests artifact: %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("typed family manifests artifact is empty")
	}
}

func TestCheckRejectsRetiredWorkFamilyJSONArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	target := filepath.Join(root, filepath.FromSlash(climanifestgen.WorkFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected retired work-family artifact drift")
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != climanifestgen.WorkFamilyJSONPath {
		t.Fatalf("drift = %#v, want %q unexpected", drift, climanifestgen.WorkFamilyJSONPath)
	}
}

func TestCheckDetectsMissingWorkFamilyArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	typedTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	if err := os.Remove(typedTarget); err != nil {
		t.Fatalf("remove generated typed manifests: %v", err)
	}

	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() || len(drift.Missing) == 0 {
		t.Fatalf("missing drift = %#v, want missing generated artifacts", drift)
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

	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("first Generate() error = %v", err)
	}
	first := fileDigests(t, root, []string{
		climanifestgen.RuntimeFamilyManifestsPath,
		climanifestgen.RepresentativeFamilyCommandIDsPath,
		climanifestgen.FactoryConfigInitFamilyCommandIDsPath,
		climanifestgen.ModelsDocsFamilyCommandIDsPath,
		climanifestgen.RunSubmitFamilyCommandIDsPath,
		climanifestgen.MCPFamilyCommandIDsPath,
	})
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("second Generate() error = %v", err)
	}
	second := fileDigests(t, root, []string{
		climanifestgen.RuntimeFamilyManifestsPath,
		climanifestgen.RepresentativeFamilyCommandIDsPath,
		climanifestgen.FactoryConfigInitFamilyCommandIDsPath,
		climanifestgen.ModelsDocsFamilyCommandIDsPath,
		climanifestgen.RunSubmitFamilyCommandIDsPath,
		climanifestgen.MCPFamilyCommandIDsPath,
	})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated generation changed artifact digests:\nfirst=%v\nsecond=%v", first, second)
	}
}

func TestCheckRejectsRetiredRepresentativeFamilyJSONArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	target := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected retired representative-family artifact drift")
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != climanifestgen.RepresentativeFamilyJSONPath {
		t.Fatalf("drift = %#v, want %q unexpected", drift, climanifestgen.RepresentativeFamilyJSONPath)
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
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		return err
	}
	// Generator fixtures need both classification sources. Existing tests call
	// this helper with commands.json, so keep that setup complete here.
	if filepath.Base(source) == filepath.Base(climanifest.ProductionManifestPath) {
		compatibilitySource := filepath.Join(filepath.Dir(source), filepath.Base(climanifest.CompatibilityManifestPath))
		compatibilityTarget := filepath.Join(filepath.Dir(target), filepath.Base(climanifest.CompatibilityManifestPath))
		return copyFile(compatibilitySource, compatibilityTarget)
	}
	return nil
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
	drift, err := checkCLIArtifacts(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production factory/config/init artifacts drift: %#v", drift)
	}

	payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath)))
	if err != nil {
		t.Fatalf("read typed family manifests artifact: %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("typed family manifests artifact is empty")
	}
}

func TestProductionCLIArtifactsMatchGenerator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	drift, err := checkCLIArtifacts(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production CLI manifest artifacts drift: %#v", drift)
	}

	for _, path := range []string{
		climanifestgen.RuntimeFamilyManifestsPath,
		climanifestgen.RepresentativeFamilyCommandIDsPath,
		climanifestgen.ModelsDocsFamilyCommandIDsPath,
		climanifestgen.RunSubmitFamilyCommandIDsPath,
	} {
		payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(path)))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			t.Fatalf("%s is empty", path)
		}
	}
}

func TestCheckRejectsRetiredRunSubmitFamilyJSONArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	if err := copyFile(
		filepath.Join(repoRoot, climanifest.ProductionManifestPath),
		filepath.Join(root, climanifest.ProductionManifestPath),
	); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(climanifestgen.RunSubmitFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != climanifestgen.RunSubmitFamilyJSONPath {
		t.Fatalf("drift = %#v, want %q unexpected", drift, climanifestgen.RunSubmitFamilyJSONPath)
	}
}

func TestProductionRepresentativeFamilyArtifactsMatchGenerator(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	drift, err := checkCLIArtifacts(repoRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("production representative-family artifacts drift: %#v", drift)
	}

	payload, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath)))
	if err != nil {
		t.Fatalf("read typed family manifests artifact: %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("typed family manifests artifact is empty")
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
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	typedTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	payload, err := os.ReadFile(typedTarget)
	if err != nil {
		t.Fatalf("read generated json: %v", err)
	}
	if err := os.WriteFile(typedTarget, bytes.ReplaceAll(payload, []byte("\n"), []byte("\r\n")), 0o644); err != nil {
		t.Fatalf("write CRLF artifact: %v", err)
	}

	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("CRLF artifacts drift = %#v, want current", drift)
	}
}

func TestExtractModelsDocsFamilyRejectsMissingCommand(t *testing.T) {
	manifest := climanifest.Manifest{
		RootPath: "you",
		Commands: map[string]climanifest.Command{
			"you.docs": {ID: "you.docs", Path: "you docs"},
		},
	}
	if _, err := climanifestgen.ExtractModelsDocsFamily(manifest); err == nil {
		t.Fatal("expected missing models/docs-family command error")
	}
}

func TestExtractModelsDocsFamilyRejectsEmptyManifest(t *testing.T) {
	if _, err := climanifestgen.ExtractModelsDocsFamily(climanifest.Manifest{}); err == nil {
		t.Fatal("expected empty manifest rejection")
	}
}

func TestFactoryConfigInitFamilyArtifactFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	payload, err := climanifestgen.FactoryConfigInitFamilyArtifact(artifactStore(t), repoRoot)
	if err != nil {
		t.Fatalf("FactoryConfigInitFamilyArtifact() error = %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("FactoryConfigInitFamilyArtifact() returned empty payload")
	}
}

func TestExtractModelsDocsFamilyFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	manifest, err := climanifest.LoadProduction(artifactStore(t), filepath.Join(repoRoot, climanifest.ProductionManifestPath))
	if err != nil {
		t.Fatalf("LoadProduction() error = %v", err)
	}

	family, err := climanifestgen.ExtractModelsDocsFamily(manifest)
	if err != nil {
		t.Fatalf("ExtractModelsDocsFamily() error = %v", err)
	}
	if len(family.Commands) != len(climanifestgen.ModelsDocsFamilyCommandIDs) {
		t.Fatalf("command count = %d, want %d", len(family.Commands), len(climanifestgen.ModelsDocsFamilyCommandIDs))
	}
	for _, id := range climanifestgen.ModelsDocsFamilyCommandIDs {
		record, ok := family.Commands[id]
		if !ok {
			t.Fatalf("missing models/docs-family command %q", id)
		}
		if record.ID != id {
			t.Fatalf("command %q record id = %q", id, record.ID)
		}
	}
}

func TestCheckRejectsRetiredFactoryConfigInitFamilyJSONArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	target := filepath.Join(root, filepath.FromSlash(climanifestgen.FactoryConfigInitFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected retired factory/config/init-family artifact drift")
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != climanifestgen.FactoryConfigInitFamilyJSONPath {
		t.Fatalf("drift = %#v, want %q unexpected", drift, climanifestgen.FactoryConfigInitFamilyJSONPath)
	}
}

func TestModelsDocsArtifactFromProductionManifest(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	payload, err := climanifestgen.ModelsDocsArtifact(artifactStore(t), repoRoot)
	if err != nil {
		t.Fatalf("ModelsDocsArtifact() error = %v", err)
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		t.Fatal("ModelsDocsArtifact() returned empty payload")
	}
}

func TestCheckRejectsRetiredModelsDocsFamilyJSONArtifact(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	target := filepath.Join(root, filepath.FromSlash(climanifestgen.ModelsDocsFamilyJSONPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}
	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() {
		t.Fatal("expected retired models/docs-family artifact drift")
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != climanifestgen.ModelsDocsFamilyJSONPath {
		t.Fatalf("drift = %#v, want %q unexpected", drift, climanifestgen.ModelsDocsFamilyJSONPath)
	}
}

func TestIsFactoryConfigInitFamilyCommandID(t *testing.T) {
	if !climanifestgen.IsFactoryConfigInitFamilyCommandID("you.factory.query") {
		t.Fatal("expected you.factory.query in factory/config/init family")
	}
	if climanifestgen.IsFactoryConfigInitFamilyCommandID("you.docs") {
		t.Fatal("expected you.docs outside factory/config/init family")
	}
}

func TestAssertModelsDocsFamilyCommandIDRejectsOutOfFamily(t *testing.T) {
	if err := climanifestgen.AssertModelsDocsFamilyCommandID("you.run"); err == nil {
		t.Fatal("expected out-of-family command id rejection")
	}
	if err := climanifestgen.AssertModelsDocsFamilyCommandID("you.docs"); err != nil {
		t.Fatalf("AssertModelsDocsFamilyCommandID(you.docs) error = %v", err)
	}
}

func TestCheckDetectsMissingFactoryConfigInitFamilyArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	typedTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	if err := os.Remove(typedTarget); err != nil {
		t.Fatalf("remove generated typed manifests: %v", err)
	}

	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() || len(drift.Missing) == 0 {
		t.Fatalf("missing drift = %#v, want missing factory/config/init artifacts", drift)
	}
}

func TestIsModelsDocsFamilyCommandID(t *testing.T) {
	if !climanifestgen.IsModelsDocsFamilyCommandID("you.docs") {
		t.Fatal("expected you.docs in models/docs family")
	}
	if climanifestgen.IsModelsDocsFamilyCommandID("you.run") {
		t.Fatal("expected you.run outside models/docs family")
	}
}

func TestCheckDetectsMissingModelsDocsFamilyArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	root := t.TempDir()
	manifestSource := filepath.Join(repoRoot, climanifest.ProductionManifestPath)
	manifestTarget := filepath.Join(root, climanifest.ProductionManifestPath)
	if err := copyFile(manifestSource, manifestTarget); err != nil {
		t.Fatalf("copy production manifest: %v", err)
	}
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	typedTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	if err := os.Remove(typedTarget); err != nil {
		t.Fatalf("remove generated typed manifests: %v", err)
	}

	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() || len(drift.Missing) == 0 {
		t.Fatalf("missing drift = %#v, want missing models/docs artifacts", drift)
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
	if err := writeCLIArtifacts(root); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	typedTarget := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	if err := os.Remove(typedTarget); err != nil {
		t.Fatalf("remove generated typed manifests: %v", err)
	}

	drift, err := checkCLIArtifacts(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if drift.Empty() || len(drift.Missing) == 0 {
		t.Fatalf("missing drift = %#v, want missing generated artifacts", drift)
	}
}

func writeCLIArtifacts(root string) error {
	store := mustArtifactStore()
	artifacts, err := climanifestgen.Artifacts(store, root)
	if err != nil {
		return err
	}
	return store.Write(root, artifacts)
}

func checkCLIArtifacts(root string) (climanifestgen.Drift, error) {
	store := mustArtifactStore()
	artifacts, err := climanifestgen.Artifacts(store, root)
	if err != nil {
		return climanifestgen.Drift{}, err
	}
	base, err := store.Check(root, artifacts)
	if err != nil {
		return climanifestgen.Drift{}, err
	}
	return climanifestgen.AnnotateDrift(base), nil
}
