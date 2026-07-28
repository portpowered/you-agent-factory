package mockworkers_test

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
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/interface"
)

// productionLoaderSources records the loader implementation files this inventory
// lane must not alter. Update hashes only when intentionally changing loader behavior
// outside this lane.
var productionLoaderSources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/services/workers/internal/interface/mock_workers_config.go",
		sha256Hex:    "a3d23e6b3b626390b4683b5fcf989d5ced5fda31e67d0456b00b82f887043dfb",
	},
}

var inventoryLaneRoots = []string{
	"pkg/services/workers/internal/interface/testdata",
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
			data = mockworkers.NormalizeSourceBytes(data)

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

	topologyFirst, err := mockworkers.MarshalCanonicalJSON(mockworkers.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology first MarshalCanonicalJSON() error = %v", err)
	}
	topologySecond, err := mockworkers.MarshalCanonicalJSON(mockworkers.ProjectTopologyInventory())
	if err != nil {
		t.Fatalf("topology second MarshalCanonicalJSON() error = %v", err)
	}
	if !bytes.Equal(topologyFirst, topologySecond) {
		t.Fatalf("repeated mock workers topology inventory json differs")
	}

	inputFirst, err := mockworkers.MarshalInputInventoryJSON(mockworkers.ProjectInputInventory())
	if err != nil {
		t.Fatalf("input first MarshalInputInventoryJSON() error = %v", err)
	}
	inputSecond, err := mockworkers.MarshalInputInventoryJSON(mockworkers.ProjectInputInventory())
	if err != nil {
		t.Fatalf("input second MarshalInputInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(inputFirst, inputSecond) {
		t.Fatalf("repeated mock workers input inventory json differs")
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

func TestInventoryLane_DocumentsNonAcceptedCapabilitiesWithoutInventingThem(t *testing.T) {
	t.Parallel()

	inventory := mockworkers.ProjectTopologyInventory()
	categories := make([]string, 0, len(inventory.NotAcceptedCapabilities))
	for _, capability := range inventory.NotAcceptedCapabilities {
		categories = append(categories, capability.Category)
	}
	for _, want := range []string{"media", "dispatch delay", "artifact payloads", "response sequences"} {
		if !containsSubstring(categories, want) {
			t.Fatalf("missing not-accepted capability category %q in %#v", want, categories)
		}
	}
}
