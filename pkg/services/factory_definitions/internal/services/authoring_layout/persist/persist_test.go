package persist

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Persistence owns these file effects. Faults are injected at its exact ports;
// no parser, validator, transport, or application graph participates.
func TestNamedFactoryFailuresPreserveTargetAndDiscardStaging(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		replace bool
		fault   string
	}{
		{name: "create staging", fault: "staging"},
		{name: "create write", fault: "write"},
		{name: "create validation", fault: "validation"},
		{name: "create commit", fault: "commit"},
		{name: "replace staging", replace: true, fault: "staging"},
		{name: "replace write", replace: true, fault: "write"},
		{name: "replace validation", replace: true, fault: "validation"},
		{name: "replace commit", replace: true, fault: "commit"},
		{name: "canceled create", fault: "canceled"},
		{name: "canceled replace", replace: true, fault: "canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runPersistenceFailure(t, test.replace, test.fault)
		})
	}
}

func runPersistenceFailure(t *testing.T, replace bool, fault string) {
	t.Helper()
	rootDir := t.TempDir()
	target := filepath.Join(rootDir, "alpha")
	if replace {
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "factory.json"), []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	failure := errors.New("controlled " + fault + " failure")
	ctx := context.Background()
	if fault == "canceled" {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		ctx, failure = canceled, context.Canceled
	}
	filesystem := &failingPersistenceFilesystem{fault: fault, failure: failure}
	writes := 0
	write := func(dir string, _ *factorydefinitions.PreparedFactoryLayoutPayload, _ string) error {
		writes++
		if fault == "write" {
			return failure
		}
		return os.WriteFile(filepath.Join(dir, "factory.json"), []byte("candidate"), 0o600)
	}
	validate := func(string) error {
		if fault == "validation" {
			return failure
		}
		return nil
	}
	result, err := NamedFactory(ctx, rootDir, "alpha", &factorydefinitions.PreparedFactoryLayoutPayload{}, replace, write, validate, filesystem, func(string) error { return nil }, failedDirectoryCommit{failure: failure})
	if result != "" || !errors.Is(err, failure) {
		t.Fatalf("NamedFactory = %q, %v, want empty path and %v", result, err, failure)
	}
	if fault == "write" || fault == "validation" {
		if !errors.Is(err, factorydefinitions.ErrInvalidNamedFactory) {
			t.Fatalf("error = %v, want invalid named Factory classification", err)
		}
	}
	if fault == "canceled" && (writes != 0 || filesystem.stagingAttempts != 0) {
		t.Fatal("canceled persistence performed staging or write effects")
	}
	assertPreservedPersistenceTarget(t, rootDir, target, replace)
}

func assertPreservedPersistenceTarget(t *testing.T, rootDir, target string, existed bool) {
	t.Helper()
	if existed {
		data, err := os.ReadFile(filepath.Join(target, "factory.json"))
		if err != nil || string(data) != "original" {
			t.Fatalf("previous Factory = %q, %v, want original", data, err)
		}
	} else if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed create left target: %v", err)
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "alpha" {
			t.Fatalf("failed persistence left staging artifact %q", entry.Name())
		}
	}
}

type failingPersistenceFilesystem struct {
	platformfilesystem.Local
	fault           string
	failure         error
	stagingAttempts int
}

func (f *failingPersistenceFilesystem) MkdirTemp(dir, pattern string) (string, error) {
	f.stagingAttempts++
	if f.fault == "staging" {
		return "", f.failure
	}
	return f.Local.MkdirTemp(dir, pattern)
}

func (f *failingPersistenceFilesystem) Rename(from, to string) error {
	if f.fault == "commit" {
		return f.failure
	}
	return f.Local.Rename(from, to)
}

type failedDirectoryCommit struct{ failure error }

func (f failedDirectoryCommit) Commit(string, string, string) (string, error) {
	return "", f.failure
}

func (failedDirectoryCommit) Restore(string, string) {}
