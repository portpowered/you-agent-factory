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
	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
)

// productionLoaderSources records the loader implementation files this inventory
// lane must not alter. Update hashes only when intentionally changing loader behavior
// outside this lane.
var productionLoaderSources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/config/operatorconfig/operator_config.go",
		sha256Hex:    "4d1690bb889627e3af6e47ea8f51bb8ef4aecbc083261bca969fbbf561dd853a",
	},
	{
		relativePath: "pkg/config/systemconfig/system_config.go",
		sha256Hex:    "7c5a10267c3cadc1c8828ebdd6654845e268484d82a22e2a81535f3aa931467d",
	},
	{
		relativePath: "pkg/config/systemconfig/provider_scope.go",
		sha256Hex:    "1e304bd8f1a010533670b9f52ec302cde65577bb8efa0a1397a15cb254b03469",
	},
}

var inventoryLaneRoots = []string{
	"pkg/config/globalconfiginventory",
	"pkg/config/operatorconfig/testdata",
	"pkg/config/systemconfig/testdata",
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

	operatorFirst, err := operatorconfig.MarshalInputInventoryJSON(operatorconfig.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator first MarshalInputInventoryJSON() error = %v", err)
	}
	operatorSecond, err := operatorconfig.MarshalInputInventoryJSON(operatorconfig.ProjectInputInventory())
	if err != nil {
		t.Fatalf("operator second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(operatorFirst, operatorSecond) {
		t.Fatalf("repeated operator config input inventory json differs")
	}

	systemFirst, err := systemconfig.MarshalInputInventoryJSON(systemconfig.ProjectInputInventory())
	if err != nil {
		t.Fatalf("system first MarshalInputInventoryJSON() error = %v", err)
	}
	systemSecond, err := systemconfig.MarshalInputInventoryJSON(systemconfig.ProjectInputInventory())
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
