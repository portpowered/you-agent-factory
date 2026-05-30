package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFactoryServiceComposeCollaboratorsMatchBuildFactoryService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeMinimalFactoryJSON(t, dir)

	ctx := t.Context()
	cfg := &FactoryServiceConfig{Dir: dir}

	built, err := BuildFactoryService(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	root, err := ResolveFactoryServiceRoot(&FactoryServiceConfig{Dir: dir})
	if err != nil {
		t.Fatalf("ResolveFactoryServiceRoot: %v", err)
	}
	load, err := LoadFactoryConfigForCompose(&FactoryServiceConfig{Dir: dir}, root)
	if err != nil {
		t.Fatalf("LoadFactoryConfigForCompose: %v", err)
	}
	clock := ServiceClockForCompose(&FactoryServiceConfig{Dir: dir}, load)
	collaborators := NewFactoryServiceCollaborators(
		&FactoryServiceConfig{Dir: dir},
		clock,
		root.BaseLogger,
		NewFactorySessionsRegistry(),
	)
	composed, err := ComposeFactoryService(
		ctx,
		&FactoryServiceConfig{Dir: dir},
		root,
		collaborators,
		load,
		clock,
	)
	if err != nil {
		t.Fatalf("ComposeFactoryService: %v", err)
	}

	if built.ComposeCollaboratorSnapshot() != composed.ComposeCollaboratorSnapshot() {
		t.Fatalf("compose snapshot mismatch: built=%+v composed=%+v", built.ComposeCollaboratorSnapshot(), composed.ComposeCollaboratorSnapshot())
	}
}

func writeMinimalFactoryJSON(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{"name":"compose-equiv","workTypes":[]}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}
