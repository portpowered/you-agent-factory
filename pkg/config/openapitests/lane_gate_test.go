package openapitests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// productionBoundarySources records Factory schema, OpenAPI fragment, mapping,
// and generated-client files this inventory lane must not alter. Update hashes
// only when intentionally changing boundary behavior outside this lane.
var productionBoundarySources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/config/openapi_factory.go",
		sha256Hex:    "fd2750ac9c16b7f818162c2af6b05cbcf8da2847deb824748dba00feb2419aba",
	},
	{
		relativePath: "pkg/config/factory_config_mapping.go",
		sha256Hex:    "c629d624b6da772c02b96d394ea77f206495b8e28d90f6ce42f3f79e68b9200c",
	},
	{
		relativePath: "pkg/config/factory_config_mapping_internal.go",
		sha256Hex:    "71aac265e4ddb0997735e1691ec407227584d7e81cbd2423da4fab45ca6508b0",
	},
	{
		relativePath: "pkg/interfaces/factory_config.go",
		sha256Hex:    "3287ccc0708b7e92fb69772da05c86a9c87d4c27301cb318a6c1f85acca2cef9",
	},
	{
		relativePath: "api/components/schemas/data-models/Factory.yaml",
		sha256Hex:    "af7cbb1c6d33c3bd2f8af6d99105683ffa072038aa3c7e5671d7aa3e8a2e31b3",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkerType.yaml",
		sha256Hex:    "9d50ae595e3274ca543265b7765a1135c3ae6f6be918a72875b2f5ab7dadb477",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkstationType.yaml",
		sha256Hex:    "415584f86f33599b6d1676f97f3d442ae91cd8010f48c5496f079da9954b14ea",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryGuard.yaml",
		sha256Hex:    "db2adae1dee03fa54073ad6bdb9958707cd1885130389f52dedf7c7f905e590b",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryLayout.yaml",
		sha256Hex:    "f5b867440c883b88838197fc42d67f3efaf17ba76ac36eb802c38a428377dc73",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryOrchestrator.yaml",
		sha256Hex:    "46717f730e2b864a1c1979bc41ec2720b14cd7fe6cd73ad8025a18c64c8e39a6",
	},
	{
		relativePath: "api/components/schemas/data-models/Resource.yaml",
		sha256Hex:    "56099e658940395035851cfcce7424782ef9e42193b8eb608b93b45d5866ab28",
	},
	{
		relativePath: "pkg/api/generated/server.gen.go",
		sha256Hex:    "5633f2946af845d4da706190305b79a3214de3aa1645c510369de0f5a8c51136",
	},
	{
		relativePath: "pkg/generatedclient/client.gen.go",
		sha256Hex:    "2ad86aba1ef76f69b1bfdeee0d02cfc4398ebab95b556dfc01cec778eb9f8233",
	},
}

var parityInventoryLaneRoots = []string{
	"pkg/config/openapitests/testdata",
}

func TestProductionBoundarySources_UnchangedForParityLane(t *testing.T) {
	t.Parallel()

	for _, src := range productionBoundarySources {
		src := src
		t.Run(src.relativePath, func(t *testing.T) {
			t.Parallel()

			path := testutil.MustRepoPath(t, src.relativePath)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read production boundary source %s: %v", path, err)
			}
			data = globalconfiginventory.NormalizeSourceBytes(data)

			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			if got != src.sha256Hex {
				t.Fatalf(
					"production boundary source drift detected for %s; update lane gate hashes only when intentionally changing schema, mapping, or generated clients\ngot %s, want %s",
					src.relativePath,
					got,
					src.sha256Hex,
				)
			}
		})
	}
}

func TestFactoryOpenAPIParityLane_DoesNotAuthorDraft202012Schemas(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, root := range parityInventoryLaneRoots {
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
					t.Fatalf("draft-2020-12 schema document found in factory openapi parity lane: %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk factory openapi parity lane root %s: %v", scanRoot, err)
			}
		})
	}
}

func TestFactoryOpenAPIParityLane_DoesNotIndexMockWorkerOrJSCallSurfaces(t *testing.T) {
	t.Parallel()

	scope := strings.ToLower(ProjectParityInventory().Scope)
	for _, forbidden := range []string{
		"mock-worker inventory",
		"mock worker inventory",
		"js-call inventory",
		"js call inventory",
		"javascript call inventory",
	} {
		if strings.Contains(scope, forbidden) {
			t.Fatalf("parity inventory scope must not claim %q inventory: %q", forbidden, scope)
		}
	}

	for _, parityCase := range ProjectParityInventory().Cases {
		lowerID := strings.ToLower(parityCase.ID)
		lowerCategory := strings.ToLower(parityCase.Category)
		for _, forbidden := range []string{"mock-worker", "mockworker", "js-call", "jscall"} {
			if strings.Contains(lowerID, forbidden) || strings.Contains(lowerCategory, forbidden) {
				t.Fatalf("parity case %q must not index %q surfaces", parityCase.ID, forbidden)
			}
		}
	}

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, root := range parityInventoryLaneRoots {
		scanRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
		err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			lowerName := strings.ToLower(entry.Name())
			for _, forbidden := range []string{
				"mock-worker-inventory",
				"mockworker-inventory",
				"js-call-inventory",
				"jscall-inventory",
			} {
				if strings.Contains(lowerName, forbidden) {
					t.Fatalf("factory openapi parity lane must not start %q inventory artifacts: %s", forbidden, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk factory openapi parity lane root %s: %v", scanRoot, err)
		}
	}
}

func TestFactoryOpenAPIParityLane_RepeatedSerializationIsByteIdentical(t *testing.T) {
	t.Parallel()

	first, err := MarshalParityInventoryJSON(ProjectParityInventory())
	if err != nil {
		t.Fatalf("first MarshalParityInventoryJSON() error = %v", err)
	}
	second, err := MarshalParityInventoryJSON(ProjectParityInventory())
	if err != nil {
		t.Fatalf("second MarshalParityInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated factory openapi parity inventory json differs")
	}
}
