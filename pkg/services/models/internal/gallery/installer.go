package gallery

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// Installer owns verified LocalAI bootstrap and gallery installation.
// Successful binary selection is memoized only within this immutable effect shape.
type Installer struct {
	OperatingSystem   string
	Runner            platformprocess.CommandRunner
	Client            effects.AssetHTTPDoer
	Environment       effects.AssetResolveEnvironment
	CacheDirectory    func() (string, error)
	ExecutableLocator platformprocess.ExecutableLocator
	MakeDirectories   effects.AssetMakeDirectories
	InspectPath       effects.AssetInspectPath
	InspectLink       effects.AssetInspectPath
	OpenFile          effects.AssetOpenFile
	CreateTempFile    effects.AssetCreateTempFile
	RenamePath        effects.AssetRenamePath
	RemovePath        effects.AssetRemovePath
	mu                sync.Mutex
	command           string
}

func (installer *Installer) Install(ctx context.Context, name string, offline bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("unsafe LocalAI gallery backend name %q", name)
	}
	cache, err := installer.CacheDirectory()
	if err != nil {
		return "", fmt.Errorf("resolve LocalAI backend cache: %w", err)
	}
	root := filepath.Join(cache, "you", "localai-backends")
	installed := filepath.Join(root, name)
	if offline {
		if !installer.installedGalleryBackendReady(installed) {
			return "", fmt.Errorf("%w: LocalAI gallery backend %q is not installed", models.ErrAssetOffline, name)
		}
		return installed, nil
	}
	installer.mu.Lock()
	defer installer.mu.Unlock()
	command := installer.command
	var commandErr error
	if command == "" {
		command, commandErr = installer.resolveLocalAIBinary(ctx, installer.Client, "")
		if commandErr == nil {
			installer.command = command
		}
	}
	if commandErr != nil {
		return "", fmt.Errorf("find local-ai for gallery backend install: %w", commandErr)
	}
	if err := installer.MakeDirectories(root, 0o700); err != nil {
		return "", fmt.Errorf("create LocalAI backend cache: %w", err)
	}
	result, runErr := installer.Runner.Run(ctx, platformprocess.CommandRequest{
		Command: command,
		Args:    []string{"backends", "install", "--backends-path=" + root, name},
	})
	if runErr != nil {
		return "", fmt.Errorf("install LocalAI gallery backend %q: %w", name, runErr)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("install LocalAI gallery backend %q: exit code %d", name, result.ExitCode)
	}
	if !installer.installedGalleryBackendReady(installed) {
		return "", fmt.Errorf("LocalAI gallery backend %q has no installed directory", name)
	}
	return installed, nil
}

func (installer *Installer) installedGalleryBackendReady(directory string) bool {
	info, err := installer.InspectLink(directory)
	if err != nil || !info.IsDir() {
		return false
	}
	script, err := installer.InspectLink(filepath.Join(directory, "run.sh"))
	return err == nil && script.Mode().IsRegular() &&
		(installer.OperatingSystem == "windows" || script.Mode().Perm()&0o111 != 0)
}
