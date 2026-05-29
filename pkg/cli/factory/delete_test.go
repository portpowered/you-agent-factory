package factory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

func TestDelete_WritesHumanReadableConfirmation(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "beta", saveTestNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}

	var out strings.Builder
	if err := Delete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: &out,
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := out.String(); got != "Deleted factory alpha\n" {
		t.Fatalf("output = %q, want deleted confirmation", got)
	}
}

func TestDelete_RejectsCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	err := Delete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected delete current factory to fail")
	}
	if !strings.Contains(err.Error(), "cannot delete current factory") {
		t.Fatalf("error = %v, want current-factory refusal", err)
	}
}

func TestDelete_RejectsMissingFactory(t *testing.T) {
	rootDir := t.TempDir()

	err := Delete(DeleteConfig{
		Name:   "missing",
		Dir:    rootDir,
		Output: ioDiscard(t),
	})
	if err == nil {
		t.Fatal("expected missing factory delete to fail")
	}
	if !strings.Contains(err.Error(), "factory not found") {
		t.Fatalf("error = %v, want not-found message", err)
	}
}

func TestDelete_ListNoLongerIncludesDeletedName(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "beta", saveTestNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}

	if err := Delete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var out strings.Builder
	if err := List(ListConfig{Dir: rootDir, Output: &out}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if strings.Contains(out.String(), "alpha\t") {
		t.Fatalf("list output still includes alpha: %q", out.String())
	}
	betaDir := filepath.Join(rootDir, "beta")
	wantRow := "beta\t" + betaDir + "\t\n"
	if !strings.Contains(out.String(), wantRow) {
		t.Fatalf("list output = %q, want beta row %q", out.String(), wantRow)
	}
}

func TestDelete_RemovesDirectoryFromDisk(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := factoryconfig.PersistNamedFactory(rootDir, "alpha", saveTestNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	if err := Delete(DeleteConfig{
		Name:   "alpha",
		Dir:    rootDir,
		Output: ioDiscard(t),
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("alpha directory still exists: %v", err)
	}
}
