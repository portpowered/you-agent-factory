package wire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

const localAIBinaryEnvironment = "LOCALAI_BINARY"

func newLinuxBackendArtifactResolver(
	installer modelswire.GalleryBackendInstaller,
	client modelswire.AssetHTTPDoer,
) (modelswire.BackendArtifactResolver, error) {
	gallery, err := modelswire.NewGalleryBackendArtifactResolver(installer)
	if err != nil {
		return nil, err
	}
	published, err := modelswire.NewPublishedBackendArtifactResolver(client)
	if err != nil {
		return nil, err
	}
	return preferPublishedLinuxCUDA(published, gallery), nil
}

func preferPublishedLinuxCUDA(published, gallery modelswire.BackendArtifactResolver) modelswire.BackendArtifactResolver {
	return func(ctx context.Context, request modelswire.ResolvedHostConfiguration, offline bool) (modelswire.BackendArtifactSelection, error) {
		platform := request.Platform
		if !offline && (platform.Accelerator == "cuda" || (platform.Accelerator == "" && platform.CUDAAvailable)) {
			cudaRequest := request
			cudaRequest.Platform.Accelerator = "cuda"
			selection, err := published(ctx, cudaRequest, false)
			if err == nil && selection.Accelerator == "cuda" && selection.Name != "" {
				return selection, nil
			}
			if ctx.Err() != nil {
				return modelswire.BackendArtifactSelection{}, ctx.Err()
			}
		}
		return gallery(ctx, request, offline)
	}
}

// newLocalAIGalleryInstaller delegates OCI image resolution, integrity, and
// extraction to LocalAI's own backend gallery command. The returned directory
// is consumed by the managed backend launcher without copying a mutable image
// into the archive cache.
func newLocalAIGalleryInstaller(runner platformprocess.CommandRunner, client localAIHTTPDoer) (modelswire.GalleryBackendInstaller, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve LocalAI backend cache: %w", err)
	}
	root := filepath.Join(cache, "you", "localai-backends")
	resolvedCommand := ""
	return localAIGalleryInstallerAt(runner, root, func(ctx context.Context) (string, error) {
		if resolvedCommand != "" {
			return resolvedCommand, nil
		}
		command, err := resolveLocalAIBinary(ctx, client, "")
		if err == nil {
			resolvedCommand = command
		}
		return command, err
	}), nil
}

func localAIGalleryInstallerAt(
	runner platformprocess.CommandRunner,
	root string,
	resolveCommand func(context.Context) (string, error),
) modelswire.GalleryBackendInstaller {
	var mu sync.Mutex
	return func(ctx context.Context, name string, offline bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
			return "", fmt.Errorf("unsafe LocalAI gallery backend name %q", name)
		}
		installed := filepath.Join(root, name)
		if offline {
			if !installedGalleryBackendReady(installed) {
				return "", fmt.Errorf("%w: LocalAI gallery backend %q is not installed", models.ErrAssetOffline, name)
			}
			return installed, nil
		}
		mu.Lock()
		defer mu.Unlock()
		command, commandErr := resolveCommand(ctx)
		if commandErr != nil {
			return "", fmt.Errorf("find local-ai for gallery backend install: %w", commandErr)
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return "", fmt.Errorf("create LocalAI backend cache: %w", err)
		}
		result, runErr := runner.Run(ctx, platformprocess.CommandRequest{
			Command: command,
			Args:    []string{"backends", "install", "--backends-path=" + root, name},
		})
		if runErr != nil {
			return "", fmt.Errorf("install LocalAI gallery backend %q: %w", name, runErr)
		}
		if result.ExitCode != 0 {
			return "", fmt.Errorf("install LocalAI gallery backend %q: exit code %d", name, result.ExitCode)
		}
		if !installedGalleryBackendReady(installed) {
			return "", fmt.Errorf("LocalAI gallery backend %q has no installed directory", name)
		}
		return installed, nil
	}
}

func installedGalleryBackendReady(directory string) bool {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() {
		return false
	}
	script, err := os.Lstat(filepath.Join(directory, "run.sh"))
	return err == nil && script.Mode().IsRegular() &&
		(runtime.GOOS == "windows" || script.Mode().Perm()&0o111 != 0)
}
