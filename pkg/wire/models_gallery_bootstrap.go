package wire

import (
	"fmt"
	"io"
	"os"

	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

func provideModelAssetHTTP(edges serviceedges.Edges) modelswire.AssetHTTPDoer {
	if selected := edges.ModelAssetHTTPClient; selected != nil {
		return modelswire.AssetHTTPDoer(selected)
	}
	return newModelAssetHTTPClient()
}

func provideModelAssetMakeDirectories(edges serviceedges.Edges) modelswire.AssetMakeDirectories {
	if selected := edges.ModelAssetMakeDirectories; selected != nil {
		return modelswire.AssetMakeDirectories(selected)
	}
	return os.MkdirAll
}

func provideModelAssetInspectPath(edges serviceedges.Edges) modelswire.AssetInspectPath {
	if selected := edges.ModelAssetInspectPath; selected != nil {
		return modelswire.AssetInspectPath(selected)
	}
	return os.Stat
}

func provideModelAssetResolveHomeDirectory(edges serviceedges.Edges) modelswire.AssetResolveHomeDirectory {
	if selected := edges.ModelAssetResolveHomeDirectory; selected != nil {
		return modelswire.AssetResolveHomeDirectory(selected)
	}
	return os.UserHomeDir
}

func provideModelAssetWriteFile(edges serviceedges.Edges) modelswire.AssetWriteFile {
	if selected := edges.ModelAssetWriteFile; selected != nil {
		return modelswire.AssetWriteFile(selected)
	}
	return os.WriteFile
}

func provideModelAssetRenamePath(edges serviceedges.Edges) modelswire.AssetRenamePath {
	if selected := edges.ModelAssetRenamePath; selected != nil {
		return modelswire.AssetRenamePath(selected)
	}
	return os.Rename
}

func provideModelAssetRemovePath(edges serviceedges.Edges) modelswire.AssetRemovePath {
	if selected := edges.ModelAssetRemovePath; selected != nil {
		return modelswire.AssetRemovePath(selected)
	}
	return os.Remove
}

func provideModelAssetReadFile(edges serviceedges.Edges) modelswire.AssetReadFile {
	if selected := edges.ModelAssetReadFile; selected != nil {
		return modelswire.AssetReadFile(selected)
	}
	return os.ReadFile
}

func provideModelAssetReadDirectory(edges serviceedges.Edges) modelswire.AssetReadDirectory {
	if selected := edges.ModelAssetReadDirectory; selected != nil {
		return modelswire.AssetReadDirectory(selected)
	}
	return os.ReadDir
}

func provideModelAssetCreateFile(edges serviceedges.Edges) modelswire.AssetCreateFile {
	if selected := edges.ModelAssetCreateFile; selected != nil {
		return modelswire.AssetCreateFile(selected)
	}
	return func(path string) (io.WriteCloser, error) { return os.Create(path) }
}

func provideModelAssetOpenFile(edges serviceedges.Edges) modelswire.AssetOpenFile {
	if selected := edges.ModelAssetOpenFile; selected != nil {
		return modelswire.AssetOpenFile(selected)
	}
	return func(path string) (io.ReadCloser, error) { return os.Open(path) }
}

func provideModelAssetResolveEnvironment(edges serviceedges.Edges) modelswire.AssetResolveEnvironment {
	if selected := edges.ModelAssetResolveEnvironment; selected != nil {
		return modelswire.AssetResolveEnvironment(selected)
	}
	return os.Getenv
}

func provideModelAssetEndpoints(edges serviceedges.Edges) models.RuntimeAssetEndpoints {
	return modelswire.NormalizeAssetEndpoints(edges.ModelAssetEndpoints)
}

func provideModelAssetCoordination(edges serviceedges.Edges) (modelswire.AssetStagingCoordination, error) {

	var coordination modelswire.AssetStagingCoordination
	var err error
	if factory := edges.ModelAssetStagingCoordinationFactory; factory != nil {
		coordination, err = factory()
	} else {
		coordination, err = platformlocking.New(platformlocking.LocalFileSystem{})
	}
	if err != nil {
		return nil, fmt.Errorf("construct Models asset staging coordination: %w", err)
	}
	return coordination, nil
}

func provideModelRuntimeTempFile(edges serviceedges.Edges) modelswire.RuntimeCreateTempFile {

	if selected := edges.ModelRuntimeCreateTempFile; selected != nil {
		return adaptModelRuntimeTempFile(selected)
	}
	return func(dir, pattern string) (modelswire.RuntimeTempFile, error) { return os.CreateTemp(dir, pattern) }
}

func provideModelGalleryCache(edges serviceedges.Edges) func() (string, error) {
	if edges.ModelAssetResolveCacheDirectory != nil {
		return edges.ModelAssetResolveCacheDirectory
	}
	return os.UserCacheDir
}
func provideModelGalleryLocator(edges serviceedges.Edges) platformprocess.ExecutableLocator {
	if edges.ModelAssetExecutableLocator != nil {
		return edges.ModelAssetExecutableLocator
	}
	return platformprocess.HostExecutableLocator{}
}
func provideModelGalleryInspectLink(edges serviceedges.Edges) modelswire.AssetInspectPath {
	if edges.ModelAssetInspectLink != nil {
		return modelswire.AssetInspectPath(edges.ModelAssetInspectLink)
	}
	return os.Lstat
}
func provideModelGalleryCreateTempFile(edges serviceedges.Edges) modelswire.AssetCreateTempFile {
	if edges.ModelAssetCreateTempFile != nil {
		return modelswire.AssetCreateTempFile(edges.ModelAssetCreateTempFile)
	}
	return func(directory, pattern string) (interface {
		io.Writer
		io.Closer
		Name() string
		Chmod(os.FileMode) error
	}, error) {
		return os.CreateTemp(directory, pattern)
	}
}
