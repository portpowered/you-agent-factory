package service_test

import (
	"fmt"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

type rootLister map[string][]factorydefinitions.NamedFactoryListEntry

func (l rootLister) ListNamedFactories(
	root string,
) ([]factorydefinitions.NamedFactoryListEntry, error) {
	return append([]factorydefinitions.NamedFactoryListEntry(nil), l[root]...), nil
}

type definitionFiles map[string][]byte

func (f definitionFiles) ReadFile(path string) ([]byte, error) {
	payload, found := f[path]
	if !found {
		return nil, fmt.Errorf("missing fixture")
	}
	return append([]byte(nil), payload...), nil
}

func TestSourceDiscoversDiskAndPackagedCandidatesWithoutMaterialization(t *testing.T) {
	t.Parallel()

	factoryDir := filepath.Join("/project", "alpha")
	discovery, err := factoryservice.NewEffectiveCatalogDiscovery(
		rootLister{
			"/project": {{
				Name:       "alpha",
				FactoryDir: factoryDir,
			}},
		}.ListNamedFactories,
		definitionFiles{
			filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile): definitionJSON("disk"),
		}.ReadFile,
		[]factorydefinitions.PackagedDefinition{{
			Name: "@you/review",
			JSON: definitionJSON("packaged"),
		}},
	)
	if err != nil {
		t.Fatalf("new effective catalog source: %v", err)
	}

	disk, err := discovery.ListRoot(t.Context(), "/project")
	if err != nil {
		t.Fatalf("list disk candidates: %v", err)
	}
	if len(disk) != 1 || disk[0].Name != "alpha" || disk[0].Location == nil {
		t.Fatalf("disk candidates = %#v, want alpha with location", disk)
	}
	packaged, err := discovery.ListPackaged(t.Context())
	if err != nil {
		t.Fatalf("list packaged candidates: %v", err)
	}
	if len(packaged) != 1 || packaged[0].Name != "@you/review" {
		t.Fatalf("packaged candidates = %#v, want @you/review", packaged)
	}
	if packaged[0].Location != nil {
		t.Fatalf("packaged location = %q, want nil", *packaged[0].Location)
	}
}
