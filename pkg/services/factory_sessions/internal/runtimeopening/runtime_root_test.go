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

func TestNormalizeDefinitionSourcePathPreservesReplayAndExplicitSourceSelection(t *testing.T) {
	t.Parallel()

	replay := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	if err := normalizeDefinitionSourcePath(&replay, "recording.json", nil); err != nil {
		t.Fatalf("replay path normalization: %v", err)
	}
	if replay.Directory != "factory-root" || replay.SourcePath != "" {
		t.Fatalf("replay selection = %#v, want unchanged selection", replay)
	}

	sourcePath := filepath.Join(t.TempDir(), "factory.yaml")
	explicit := factorydefinitions.RuntimeOpeningRequest{
		Directory:  "factory-root",
		SourcePath: sourcePath,
	}
	err := normalizeDefinitionSourcePath(&explicit, "", func() (string, error) {
		return t.TempDir(), nil
	})
	if err != nil || explicit.SourcePath != filepath.Clean(sourcePath) {
		t.Fatalf("explicit source path = (%q, %v), want (%q, nil)", explicit.SourcePath, err, sourcePath)
	}
	if explicit.Directory != "factory-root" {
		t.Fatalf("explicit source changed runtime root to %q", explicit.Directory)
	}
}

func TestNormalizeDefinitionSourcePathLeavesCurrentSelectionToFactoryDefinitions(t *testing.T) {
	t.Parallel()

	definition := factorydefinitions.RuntimeOpeningRequest{Directory: "factory-root"}
	err := normalizeDefinitionSourcePath(
		&definition,
		"",
		func() (string, error) { return t.TempDir(), nil },
	)
	if err != nil || definition.Directory != "factory-root" || definition.SourcePath != "" {
		t.Fatalf("current Factory selection = %#v, %v; want unchanged request", definition, err)
	}

	want := os.ErrNotExist
	if err := normalizeDefinitionSourcePath(
		&factorydefinitions.RuntimeOpeningRequest{SourcePath: "~\\factory.yaml"},
		"",
		func() (string, error) { return "", want },
	); !errors.Is(err, want) {
		t.Fatalf("source home resolver error = %v, want %v", err, want)
	}
}
