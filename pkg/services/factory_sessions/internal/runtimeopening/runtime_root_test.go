package runtimeopening

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveRuntimeRootNormalizesSharedProcessInputs(t *testing.T) {
	dir := t.TempDir()
	root, err := ResolveRuntimeRoot(filepath.Join(dir, "."), nil, "", func() string { return "runtime-id" }, os.UserHomeDir)
	if err != nil {
		t.Fatalf("resolve runtime root: %v", err)
	}
	if root.FactoryRootDir != filepath.Clean(dir) {
		t.Fatalf("root = %q, want %q", root.FactoryRootDir, filepath.Clean(dir))
	}
	if root.BaseLogger == nil {
		t.Fatal("resolve runtime root did not normalize the base logger")
	}
	if root.RuntimeInstanceID != "runtime-id" {
		t.Fatalf("runtime instance ID = %q, want runtime-id", root.RuntimeInstanceID)
	}
}

func TestResolveRuntimeRootPreservesExplicitIdentityWithoutGenerator(t *testing.T) {
	root, err := ResolveRuntimeRoot(t.TempDir(), nil, "explicit-runtime", nil, os.UserHomeDir)
	if err != nil {
		t.Fatalf("ResolveRuntimeRoot: %v", err)
	}
	if root.RuntimeInstanceID != "explicit-runtime" {
		t.Fatalf("runtime instance ID = %q", root.RuntimeInstanceID)
	}
}

func TestResolveRuntimeRootFailsClosedWithoutRequiredIdentityGenerator(t *testing.T) {
	_, err := ResolveRuntimeRoot(t.TempDir(), nil, "", nil, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "ID generator is required") {
		t.Fatalf("error = %v, want missing ID generator failure", err)
	}
	_, err = ResolveRuntimeRoot(t.TempDir(), nil, "", func() string { return "  " }, os.UserHomeDir)
	if err == nil || !strings.Contains(err.Error(), "empty identity") {
		t.Fatalf("error = %v, want empty generated identity failure", err)
	}
}

func TestResolveDefinitionPathPreservesReplayAndExplicitSourceSelection(t *testing.T) {
	t.Parallel()

	replay := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	got, err := resolveDefinitionPath(&replay, "recording.json", nil, nil)
	if err != nil || got != "factory-root" {
		t.Fatalf("replay path = (%q, %v), want (factory-root, nil)", got, err)
	}

	sourcePath := filepath.Join(t.TempDir(), "factory.yaml")
	explicit := factorydefinitions.RuntimeOpeningRequest{
		Directory:  "factory-root",
		SourcePath: sourcePath,
	}
	got, err = resolveDefinitionPath(&explicit, "", nil, func() (string, error) {
		return t.TempDir(), nil
	})
	if err != nil || got != filepath.Clean(sourcePath) {
		t.Fatalf("explicit source path = (%q, %v), want (%q, nil)", got, err, sourcePath)
	}
	if explicit.Directory != "factory-root" {
		t.Fatalf("explicit source changed runtime root to %q", explicit.Directory)
	}
}

func TestResolveDefinitionPathResolvesCurrentFactoryAndErrors(t *testing.T) {
	t.Parallel()

	definition := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	currentDir := filepath.Join(t.TempDir(), "current")
	got, err := resolveDefinitionPath(
		&definition,
		"",
		func(root string) (string, error) {
			if root != "factory-root" {
				t.Fatalf("current root = %q, want factory-root", root)
			}
			return currentDir, nil
		},
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil || got != filepath.Clean(currentDir) || definition.Directory != got {
		t.Fatalf("current Factory path = (%q, %v, directory %q)", got, err, definition.Directory)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"},
		"",
		nil,
		nil,
	); err == nil || !strings.Contains(err.Error(), "named Factory path resolver is required") {
		t.Fatalf("missing current resolver error = %v", err)
	}

	want := errors.New("current unavailable")
	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"},
		"",
		func(string) (string, error) { return "", want },
		nil,
	); !errors.Is(err, want) {
		t.Fatalf("current resolver error = %v, want %v", err, want)
	}

	if _, err := resolveDefinitionPath(
		&factorydefinitions.RuntimeOpeningRequest{SourcePath: "~\\factory.yaml"},
		"",
		nil,
		func() (string, error) { return "", want },
	); !errors.Is(err, want) {
		t.Fatalf("source home resolver error = %v, want %v", err, want)
	}
}
