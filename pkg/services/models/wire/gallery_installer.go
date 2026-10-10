package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/gallery"
)

// NewLocalAIGalleryInstaller constructs an inert Models-owned installer.
func NewLocalAIGalleryInstaller(runner platformprocess.CommandRunner, client AssetHTTPDoer,
	environment AssetResolveEnvironment, cache func() (string, error), locator platformprocess.ExecutableLocator,
	mkdir AssetMakeDirectories, inspect, inspectLink AssetInspectPath, open AssetOpenFile,
	create AssetCreateTempFile, rename AssetRenamePath, remove AssetRemovePath, operatingSystem string) (GalleryBackendInstaller, error) {
	installer := &gallery.Installer{OperatingSystem: operatingSystem, Runner: runner, Client: client, Environment: environment,
		CacheDirectory: cache, ExecutableLocator: locator, MakeDirectories: mkdir, InspectPath: inspect,
		InspectLink: inspectLink, OpenFile: open, CreateTempFile: create, RenamePath: rename, RemovePath: remove}
	return installer.Install, nil
}
