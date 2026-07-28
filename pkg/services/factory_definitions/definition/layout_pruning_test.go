package factorydefinition_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPreparePersistedFactoryPayload_PrunesStaleLayout(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: factorydefinitions.SupportedFactoryLayoutSchemaVersion,
		Nodes: &[]factoryapi.FactoryLayoutNode{{
			Id:       "workstation:process",
			Position: factoryapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}, {
			Id:       "workstation:stale-node",
			Position: factoryapi.FactoryLayoutPoint{X: 30, Y: 40},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &factoryapi.FactoryLayoutViewport{Zoom: 1},
	}

	prepared, err := preparePersistedFactoryPayload("alpha", factory, factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(2),
		Physical: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("preparePersistedFactoryPayload: %v", err)
	}
	if prepared.Config == nil || prepared.Config.Layout == nil {
		t.Fatal("expected pruned layout on prepared config")
	}
	if len(prepared.Config.Layout.Nodes) != 1 || prepared.Config.Layout.Nodes[0].ID != "workstation:process" {
		t.Fatalf("pruned layout nodes = %#v", prepared.Config.Layout.Nodes)
	}

	var canonical map[string]any
	if err := json.Unmarshal(prepared.Canonical, &canonical); err != nil {
		t.Fatalf("Unmarshal canonical: %v", err)
	}
	layout, ok := canonical["layout"].(map[string]any)
	if !ok {
		t.Fatalf("canonical layout = %#v, want object", canonical["layout"])
	}
	nodes, ok := layout["nodes"].([]any)
	if !ok || len(nodes) != 1 {
		t.Fatalf("canonical layout nodes = %#v, want one node", layout["nodes"])
	}
	if _, ok := canonical["layoutOutcomes"]; ok {
		t.Fatalf("canonical payload must not persist removed layout metadata: %#v", canonical["layoutOutcomes"])
	}
}

func TestPreparePersistedFactoryPayload_PreservesUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: 99,
		Nodes: &[]factoryapi.FactoryLayoutNode{{
			Id:       "workstation:process",
			Position: factoryapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &factoryapi.FactoryLayoutViewport{Zoom: 1},
	}

	prepared, err := preparePersistedFactoryPayload("alpha", factory, factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(2),
		Physical: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("preparePersistedFactoryPayload: %v", err)
	}
	if prepared.Config == nil || prepared.Config.Layout == nil {
		t.Fatal("expected layout preserved on prepared config")
	}
	if prepared.Config.Layout.SchemaVersion != 99 {
		t.Fatalf("layout schemaVersion = %d, want 99 preserved on save", prepared.Config.Layout.SchemaVersion)
	}
}

func TestPrepareFactoryLayoutPayload_PrunesStaleLayoutOnNamedFactoryPersistPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	payload := namedFactoryPayloadWithStaleLayout(t)
	factoryDir, err := persistNamedFactoryForTest(root, "alpha", payload, factoryvalidation.New(nil))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factorydefinitioncomposition.LoadCurrent(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	cfg := loaded.FactoryConfig()
	if cfg.Layout == nil || len(cfg.Layout.Nodes) != 1 {
		t.Fatalf("loaded layout nodes = %#v, want one pruned node", cfg.Layout)
	}
	if cfg.Layout.Nodes[0].ID != "workstation:process" {
		t.Fatalf("loaded layout node id = %q, want workstation:process", cfg.Layout.Nodes[0].ID)
	}
}

func namedFactoryPayloadWithStaleLayout(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: factorydefinitions.SupportedFactoryLayoutSchemaVersion,
		Nodes: &[]factoryapi.FactoryLayoutNode{{
			Id:       "workstation:process",
			Position: factoryapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}, {
			Id:       "workstation:stale-node",
			Position: factoryapi.FactoryLayoutPoint{X: 30, Y: 40},
			Size:     &factoryapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &factoryapi.FactoryLayoutViewport{Zoom: 1},
	}
	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("Marshal(factory): %v", err)
	}
	return payload
}
