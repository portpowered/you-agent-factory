package wire

import (
	"io"
	"net/http"
	"os"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestNewServiceRequiresScopedCacheInspectionDependencies(t *testing.T) {
	t.Parallel()

	scopes := testRuntimeScopes(t)
	platform := models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	inspect := modelseffects.AssetInspectPath(os.Stat)
	home := modelseffects.AssetResolveHomeDirectory(os.UserHomeDir)
	readFile := modelseffects.AssetReadFile(os.ReadFile)
	readDirectory := modelseffects.AssetReadDirectory(os.ReadDir)
	endpoints := models.RuntimeAssetEndpoints{
		BaseURL: "https://assets.example.test", APIBaseURL: "https://api.example.test",
	}

	tests := []struct {
		name      string
		scopes    runtimescopes.Service
		platform  models.AssetHostPlatform
		client    modelseffects.AssetHTTPDoer
		endpoints models.RuntimeAssetEndpoints
		mkdir     modelseffects.AssetMakeDirectories
		inspect   modelseffects.AssetInspectPath
		home      modelseffects.AssetResolveHomeDirectory
		write     modelseffects.AssetWriteFile
		rename    modelseffects.AssetRenamePath
		remove    modelseffects.AssetRemovePath
		readFile  modelseffects.AssetReadFile
		readDir   modelseffects.AssetReadDirectory
		create    modelseffects.AssetCreateFile
		open      modelseffects.AssetOpenFile
		wantError bool
	}{
		validAssetDependencies("valid", scopes, platform, endpoints),
		{
			name: "platform", scopes: scopes, client: http.DefaultClient, endpoints: endpoints,
			mkdir: os.MkdirAll, inspect: inspect, home: home, write: os.WriteFile, rename: os.Rename,
			remove: os.Remove, readFile: readFile, readDir: readDirectory,
			create: func(path string) (io.WriteCloser, error) { return os.Create(path) },
			open:   func(path string) (io.ReadCloser, error) { return os.Open(path) }, wantError: true,
		},
		{
			name: "scopes", platform: platform, client: http.DefaultClient, endpoints: endpoints,
			mkdir: os.MkdirAll, inspect: inspect, home: home, write: os.WriteFile, rename: os.Rename,
			remove: os.Remove, readFile: readFile, readDir: readDirectory,
			create: func(path string) (io.WriteCloser, error) { return os.Create(path) },
			open:   func(path string) (io.ReadCloser, error) { return os.Open(path) }, wantError: true,
		},
		{
			name: "source", scopes: scopes, platform: platform, mkdir: os.MkdirAll,
			inspect: inspect, home: home, write: os.WriteFile, rename: os.Rename,
			remove: os.Remove, readFile: readFile, readDir: readDirectory,
			create: func(path string) (io.WriteCloser, error) { return os.Create(path) },
			open:   func(path string) (io.ReadCloser, error) { return os.Open(path) }, wantError: true,
		},
		{
			name: "cache", scopes: scopes, platform: platform, client: http.DefaultClient,
			endpoints: endpoints, wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(
				test.scopes,
				test.platform,
				test.client,
				test.endpoints,
				test.mkdir,
				test.inspect,
				test.home,
				test.write,
				test.rename,
				test.remove,
				test.readFile,
				test.readDir,
				test.create,
				test.open,
			)
			if test.wantError {
				if service != nil || err == nil {
					t.Fatalf("NewService = (%#v, %v), want dependency error", service, err)
				}
				return
			}
			if service == nil || err != nil {
				t.Fatalf("NewService = (%#v, %v), want service", service, err)
			}
		})
	}
}

func validAssetDependencies(
	name string,
	scopes runtimescopes.Service,
	platform models.AssetHostPlatform,
	endpoints models.RuntimeAssetEndpoints,
) struct {
	name      string
	scopes    runtimescopes.Service
	platform  models.AssetHostPlatform
	client    modelseffects.AssetHTTPDoer
	endpoints models.RuntimeAssetEndpoints
	mkdir     modelseffects.AssetMakeDirectories
	inspect   modelseffects.AssetInspectPath
	home      modelseffects.AssetResolveHomeDirectory
	write     modelseffects.AssetWriteFile
	rename    modelseffects.AssetRenamePath
	remove    modelseffects.AssetRemovePath
	readFile  modelseffects.AssetReadFile
	readDir   modelseffects.AssetReadDirectory
	create    modelseffects.AssetCreateFile
	open      modelseffects.AssetOpenFile
	wantError bool
} {
	return struct {
		name      string
		scopes    runtimescopes.Service
		platform  models.AssetHostPlatform
		client    modelseffects.AssetHTTPDoer
		endpoints models.RuntimeAssetEndpoints
		mkdir     modelseffects.AssetMakeDirectories
		inspect   modelseffects.AssetInspectPath
		home      modelseffects.AssetResolveHomeDirectory
		write     modelseffects.AssetWriteFile
		rename    modelseffects.AssetRenamePath
		remove    modelseffects.AssetRemovePath
		readFile  modelseffects.AssetReadFile
		readDir   modelseffects.AssetReadDirectory
		create    modelseffects.AssetCreateFile
		open      modelseffects.AssetOpenFile
		wantError bool
	}{
		name: name, scopes: scopes, platform: platform, client: http.DefaultClient, endpoints: endpoints,
		mkdir: os.MkdirAll, inspect: os.Stat, home: os.UserHomeDir, write: os.WriteFile,
		rename: os.Rename, remove: os.Remove, readFile: os.ReadFile, readDir: os.ReadDir,
		create: func(path string) (io.WriteCloser, error) { return os.Create(path) },
		open:   func(path string) (io.ReadCloser, error) { return os.Open(path) },
	}
}

func testRuntimeScopes(t *testing.T) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "assets-wire-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	return scopes
}
