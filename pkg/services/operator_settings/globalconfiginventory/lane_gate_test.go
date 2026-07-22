package globalconfiginventory_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/globalconfiginventory"
	identityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/identityinventory"
)

// productionLoaderSources records the loader implementation files this inventory
// lane must not alter. Update hashes only when intentionally changing loader behavior
// outside this lane.
var productionLoaderSources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/services/operator_settings/operator_config.go",
		sha256Hex:    "7414fa453ddedfdc9f3f4f860401da2428c8f74a3e7722d2bc9fb5b87af5dcc5",
	},
	{
		relativePath: "pkg/services/operator_settings/identity.go",
		sha256Hex:    "3096225cbc7ccfed3d71cec83e44450b6910d7b4dccf209b6d045015a978438e",
	},
	{
		relativePath: "pkg/services/operator_settings/provider_scope.go",
		sha256Hex:    "946f04376270a5a6756912ab17ab0685444170a1ca0a981efc12358f603f897e",
	},
}

var inventoryLaneRoots = []string{
	"pkg/services/operator_settings/globalconfiginventory",
	"pkg/services/operator_settings/testdata",
	"pkg/services/operator_settings/identityinventory/testdata",
}

func TestProductionLoaderSources_UnchangedForInventoryLane(t *testing.T) {
	t.Parallel()

	for _, src := range productionLoaderSources {
		src := src
		t.Run(src.relativePath, func(t *testing.T) {
			t.Parallel()

			path := testutil.MustRepoPath(t, src.relativePath)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read production loader source %s: %v", path, err)
			}
			data = globalconfiginventory.NormalizeSourceBytes(data)

			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			if got != src.sha256Hex {
				t.Fatalf(
					"production loader source drift detected for %s; update lane gate hashes only when intentionally changing loader behavior\ngot %s, want %s",
					src.relativePath,
					got,
					src.sha256Hex,
				)
			}
		})
	}
}

func TestInventoryLane_RepeatedSerializationIsByteIdenticalAcrossOwners(t *testing.T) {
	t.Parallel()

	topologyFirst, err := globalconfiginventory.MarshalCanonicalJSON(globalconfiginventory.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology first MarshalCanonicalJSON() error = %v", err)
	}
	topologySecond, err := globalconfiginventory.MarshalCanonicalJSON(globalconfiginventory.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(topologyFirst, topologySecond) {
		t.Fatalf("repeated global config topology inventory json differs")
	}

	operatorFirst, err := operator_settings.MarshalInputInventoryJSON(operator_settings.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator first MarshalInputInventoryJSON() error = %v", err)
	}
	operatorSecond, err := operator_settings.MarshalInputInventoryJSON(operator_settings.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(operatorFirst, operatorSecond) {
		t.Fatalf("repeated operator config input inventory json differs")
	}

	systemFirst, err := identityinventory.MarshalInputInventoryJSON(identityinventory.ProjectInputInventory())
	if err != nil {
		t.Fatalf("system first MarshalInputInventoryJSON() error = %v", err)
	}
	systemSecond, err := identityinventory.MarshalInputInventoryJSON(identityinventory.ProjectInputInventory())
	if err != nil {
		t.Fatalf("system second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(systemFirst, systemSecond) {
		t.Fatalf("repeated system config input inventory json differs")
	}
}

func TestInventoryLane_DoesNotAuthorDraft202012Schemas(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, root := range inventoryLaneRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()

			scanRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
			err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					return nil
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lower := strings.ToLower(string(data))
				if strings.Contains(lower, `"$schema"`) && strings.Contains(lower, "draft-2020-12") {
					t.Fatalf("draft-2020-12 schema document found in inventory lane: %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk inventory lane root %s: %v", scanRoot, err)
			}
		})
	}
}
