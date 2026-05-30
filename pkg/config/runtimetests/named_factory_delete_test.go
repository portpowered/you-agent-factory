package runtimetests

import (
	"errors"
	. "github.com/portpowered/infinite-you/pkg/config"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestDeleteNamedFactory_RemovesNamedDirectory(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}

	if err := DeleteNamedFactory(rootDir, "alpha"); err != nil {
		t.Fatalf("DeleteNamedFactory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(rootDir, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("alpha directory still exists: %v", err)
	}
	if _, err := ResolveNamedFactoryDir(rootDir, "beta"); err != nil {
		t.Fatalf("beta should remain: %v", err)
	}
}

func TestDeleteNamedFactory_RejectsCurrentFactory(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	err := DeleteNamedFactory(rootDir, "alpha")
	if err == nil {
		t.Fatal("expected delete current factory to fail")
	}
	if !errors.Is(err, ErrNamedFactoryIsCurrent) {
		t.Fatalf("error = %v, want ErrNamedFactoryIsCurrent", err)
	}
	if !strings.Contains(err.Error(), ".current-factory") {
		t.Fatalf("error = %v, want actionable current-pointer guidance", err)
	}
	if _, statErr := os.Stat(filepath.Join(rootDir, "alpha")); statErr != nil {
		t.Fatalf("alpha should remain on disk: %v", statErr)
	}
}

func TestDeleteNamedFactory_RejectsMissingFactory(t *testing.T) {
	rootDir := t.TempDir()

	err := DeleteNamedFactory(rootDir, "missing")
	if err == nil {
		t.Fatal("expected missing factory delete to fail")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error = %v, want os.ErrNotExist", err)
	}
}

func TestDeleteNamedFactory_PreservesCurrentPointerFile(t *testing.T) {
	rootDir := t.TempDir()
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	if err := DeleteNamedFactory(rootDir, "beta"); err != nil {
		t.Fatalf("DeleteNamedFactory(beta): %v", err)
	}

	current, err := ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if current != "alpha" {
		t.Fatalf("current = %q, want alpha", current)
	}
	if _, err := os.Stat(filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile)); err != nil {
		t.Fatalf("current pointer file missing: %v", err)
	}
}
