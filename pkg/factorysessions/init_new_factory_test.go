package factorysessions

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestValidateInitNewFactoryNestedDir_AllowsMissingNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := ValidateInitNewFactoryNestedDir(root); err != nil {
		t.Fatalf("ValidateInitNewFactoryNestedDir(missing) = %v, want nil", err)
	}
}

func TestValidateInitNewFactoryNestedDir_AllowsEmptyNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("Mkdir(nested): %v", err)
	}
	if err := ValidateInitNewFactoryNestedDir(root); err != nil {
		t.Fatalf("ValidateInitNewFactoryNestedDir(empty nested) = %v, want nil", err)
	}
}

func TestValidateInitNewFactoryNestedDir_RejectsNestedFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.WriteFile(nested, []byte("not a directory\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(nested): %v", err)
	}
	err := ValidateInitNewFactoryNestedDir(root)
	if err == nil {
		t.Fatal("ValidateInitNewFactoryNestedDir(file) = nil, want conflict")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonConflict || field != "folderPath" {
		t.Fatalf("ValidationReasonFromError = (%q, %q, %v), want conflict on folderPath", reason, field, ok)
	}
}

func TestValidateInitNewFactoryNestedDir_RejectsPopulatedNestedDirectory(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, interfaces.FactoryDir)
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested): %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "notes.txt"), []byte("existing notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes): %v", err)
	}
	err := ValidateInitNewFactoryNestedDir(root)
	if err == nil {
		t.Fatal("ValidateInitNewFactoryNestedDir(populated) = nil, want conflict")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonConflict || field != "folderPath" {
		t.Fatalf("ValidationReasonFromError = (%q, %q, %v), want conflict on folderPath", reason, field, ok)
	}
}
