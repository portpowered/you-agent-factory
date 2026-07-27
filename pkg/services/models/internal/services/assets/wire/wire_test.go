package wire

import (
	"os"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestNewServiceRequiresScopedCacheInspectionDependencies(t *testing.T) {
	t.Parallel()

	scopes := testRuntimeScopes(t)
	platform := models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}
	inspect := models.AssetInspectPath(os.Stat)
	home := models.AssetResolveHomeDirectory(os.UserHomeDir)
	readFile := models.AssetReadFile(os.ReadFile)
	readDirectory := models.AssetReadDirectory(os.ReadDir)

	tests := []struct {
		name      string
		scopes    runtimescopes.Service
		platform  models.AssetHostPlatform
		inspect   models.AssetInspectPath
		home      models.AssetResolveHomeDirectory
		readFile  models.AssetReadFile
		readDir   models.AssetReadDirectory
		wantError bool
	}{
		{name: "valid", scopes: scopes, platform: platform, inspect: inspect, home: home, readFile: readFile, readDir: readDirectory},
		{name: "scopes", platform: platform, inspect: inspect, home: home, readFile: readFile, readDir: readDirectory, wantError: true},
		{name: "platform", scopes: scopes, inspect: inspect, home: home, readFile: readFile, readDir: readDirectory, wantError: true},
		{name: "inspect", scopes: scopes, platform: platform, home: home, readFile: readFile, readDir: readDirectory, wantError: true},
		{name: "home", scopes: scopes, platform: platform, inspect: inspect, readFile: readFile, readDir: readDirectory, wantError: true},
		{name: "read file", scopes: scopes, platform: platform, inspect: inspect, home: home, readDir: readDirectory, wantError: true},
		{name: "read directory", scopes: scopes, platform: platform, inspect: inspect, home: home, readFile: readFile, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(
				test.scopes,
				test.platform,
				test.inspect,
				test.home,
				test.readFile,
				test.readDir,
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

func testRuntimeScopes(t *testing.T) runtimescopes.Service {
	t.Helper()
	scopes, err := runtimescopeswire.NewService(func() string { return "assets-wire-test" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	return scopes
}
