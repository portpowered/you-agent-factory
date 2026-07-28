package cli_test

import (
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
)

type manifestSourceStore func(string) ([]byte, error)

func (read manifestSourceStore) Read(path string) ([]byte, error) { return read(path) }

func loadCLICommandManifest(t *testing.T, relativePath string) climanifest.Manifest {
	t.Helper()

	path := testutil.MustRepoPath(t, relativePath)
	store := manifestSourceStore(os.ReadFile)
	var (
		manifest climanifest.Manifest
		err      error
	)
	if relativePath == climanifest.CompatibilityManifestPath {
		manifest, err = climanifest.LoadCompatibility(store, path)
	} else {
		manifest, err = climanifest.LoadProduction(store, path)
	}
	if err != nil {
		t.Fatalf("load manifest %q = %v", relativePath, err)
	}
	return manifest
}

func providersExecuteAdjacentCommandIDs(manifest climanifest.Manifest) []string {
	var matches []string
	for id, command := range manifest.Commands {
		if !isProvidersExecuteAdjacentCommand(id, command.Path) {
			continue
		}
		matches = append(matches, id)
	}
	return matches
}

func isProvidersExecuteAdjacentCommand(id, path string) bool {
	if strings.HasPrefix(id, "you.providers.") {
		remainder := strings.TrimPrefix(id, "you.providers.")
		switch remainder {
		case "list", "show":
			return false
		default:
			return true
		}
	}

	normalizedPath := strings.TrimSpace(path)
	if !strings.HasPrefix(normalizedPath, "you providers ") {
		return false
	}
	remainder := strings.TrimPrefix(normalizedPath, "you providers ")
	switch remainder {
	case "list", "show":
		return false
	default:
		return true
	}
}

func TestAcceptedCLIContract_HasNoPublicProvidersExecuteAdjacentSurface(t *testing.T) {
	t.Parallel()

	for _, relativePath := range []string{
		climanifest.ProductionManifestPath,
		climanifest.CompatibilityManifestPath,
	} {
		relativePath := relativePath
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			manifest := loadCLICommandManifest(t, relativePath)
			if matches := providersExecuteAdjacentCommandIDs(manifest); len(matches) > 0 {
				t.Fatalf(
					"manifest %q declares Providers execute-adjacent commands %v; this packet must not invent execute CLI UX",
					relativePath,
					matches,
				)
			}
		})
	}
}

func TestProvidersCLIAdapter_ServiceExposesNoExecutePath(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeOf((*providerscli.Service)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		if typ.Method(i).Name == "Execute" {
			t.Fatal("Providers CLI Service exposes Execute; no accepted execute-adjacent CLI surface exists to wire")
		}
	}
}

func TestProvidersCLIAdapter_ListAndShowDoNotInvokeExecute(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{
		listResult: providers.ListProvidersResult{
			Providers: []providers.Descriptor{{
				ID:           providers.IDCodex,
				DisplayName:  "Codex",
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessReady,
			}},
		},
		getResult: providers.GetProviderResult{
			Provider: providers.Descriptor{
				ID:           providers.IDCodex,
				DisplayName:  "Codex",
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessReady,
			},
		},
	}
	service := constructedProvidersCLIService(t, root)

	if err := service.List(providerscli.ListConfig{
		Context: t.Context(),
		Output:  io.Discard,
	}); err != nil {
		t.Fatalf("List() = %v", err)
	}
	if err := service.Show(providerscli.ShowConfig{
		Context:    t.Context(),
		Output:     io.Discard,
		ProviderID: string(providers.IDCodex),
	}); err != nil {
		t.Fatalf("Show() = %v", err)
	}
	if root.executeCalls != 0 {
		t.Fatalf("Execute calls = %d, want 0 for catalog-only adapter paths", root.executeCalls)
	}
}
