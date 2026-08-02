package configcontractsmoke

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
)

func TestCheckPassesCleanRepository(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	diagnostics, err := Check(repositoryRoot, testGlobalParser)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Fatalf("Check() diagnostics = %#v, want none", diagnostics)
	}
}

func testGlobalParser(payload []byte) error {
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return err
	}
	if _, ok := document["unexpectedTopLevel"]; ok {
		return fmt.Errorf("unexpectedTopLevel")
	}
	if strings.Contains(string(payload), "Other_Provider") {
		return fmt.Errorf("modelProvider")
	}
	return nil
}

func TestPublishedExportDiagnosticsNameFamilyAndPath(t *testing.T) {
	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	family := Families()[0]
	manifest, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(manifestPath)))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	temporaryRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(temporaryRoot, "packages", "api", "generated"), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(temporaryRoot, filepath.FromSlash(manifestPath)), manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	diagnostics := CheckPublishedExports(temporaryRoot, []Family{family})
	if len(diagnostics) != 1 {
		t.Fatalf("CheckPublishedExports() diagnostics = %#v, want one", diagnostics)
	}
	got := diagnostics[0]
	if got.Code != "config.export.missing" || got.Family != family.ID || got.Path != family.ExportPath {
		t.Fatalf("diagnostic = %#v, want missing family %q path %q", got, family.ID, family.ExportPath)
	}
	if message := got.Error(); !strings.Contains(message, string(family.ID)) || !strings.Contains(message, family.ExportPath) {
		t.Fatalf("diagnostic %q does not name family and path", message)
	}
}
