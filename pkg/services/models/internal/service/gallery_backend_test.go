package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestResolveJoinedBackendArtifactAcceptsInstalledDirectory(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration, bool) (modelseffects.BackendArtifactSelection, error) {
		return modelseffects.BackendArtifactSelection{InstalledPath: directory}, nil
	}}
	selection, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-llamacpp"}, false)
	if err != nil || selection.InstalledPath != directory {
		t.Fatalf("installed backend selection = %#v, error = %v", selection, err)
	}
}

func TestResolveJoinedBackendArtifactRejectsRelativeInstalledDirectory(t *testing.T) {
	t.Parallel()
	root := &Root{resolveBackendArtifact: func(context.Context, modelseffects.ResolvedHostConfiguration, bool) (modelseffects.BackendArtifactSelection, error) {
		return modelseffects.BackendArtifactSelection{InstalledPath: "relative/backend"}, nil
	}}
	_, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-whisper"}, false)
	if !errors.Is(err, models.ErrHostMissingAssets) {
		t.Fatalf("relative installed directory error = %v, want ErrHostMissingAssets", err)
	}
}

func TestResolveJoinedBackendArtifactPreservesOfflineFailure(t *testing.T) {
	t.Parallel()
	root := &Root{resolveBackendArtifact: func(_ context.Context, _ modelseffects.ResolvedHostConfiguration, offline bool) (modelseffects.BackendArtifactSelection, error) {
		if !offline {
			t.Fatal("backend resolver did not receive offline policy")
		}
		return modelseffects.BackendArtifactSelection{}, models.ErrAssetOffline
	}}
	_, err := root.resolveJoinedBackendArtifact(context.Background(), modelseffects.ResolvedHostConfiguration{Backend: "localai-whisper"}, true)
	if !errors.Is(err, models.ErrAssetOffline) {
		t.Fatalf("offline backend error = %v, want ErrAssetOffline", err)
	}
}
