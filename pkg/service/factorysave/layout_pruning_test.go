package factorysave

import (
	"encoding/json"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil/validationassert"
)

func TestPreparePersistedFactoryPayload_PrunesStaleLayoutAndRecordsOutcomes(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
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
	validationassert.HasDomainTargetCode(t, prepared.LayoutOutcomes, factoryvalidation.CodeLayoutUnknownNodeReference)
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
		t.Fatalf("canonical payload must not persist layoutOutcomes: %#v", canonical["layoutOutcomes"])
	}
}

func TestPreparePersistedFactoryPayload_IncludesUnsupportedSchemaVersionInLayoutOutcomes(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
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
	validationassert.HasDomainTargetCode(t, prepared.LayoutOutcomes, factoryvalidation.CodeLayoutUnsupportedSchemaVersion)
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
	factoryDir, err := configpersist.PersistNamedFactory(root, "alpha", payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(factoryDir, nil)
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

func TestStripEphemeralFactoryResponseFields_RemovesLayoutOutcomesFromSaveRequests(t *testing.T) {
	t.Parallel()

	targets := factoryvalidation.ToValidationTargets([]factoryvalidation.Target{{
		Code:     factoryvalidation.CodeLayoutUnknownNodeReference,
		Severity: factoryvalidation.SeverityWarning,
		Message:  "stale",
		Subject: factoryvalidation.Subject{
			Type:     factoryvalidation.SubjectTypeFactory,
			ID:       "layout",
			Location: factoryvalidation.SubjectLocationReference,
		},
	}})
	factory := factoryapi.Factory{
		Name: "alpha",
		LayoutOutcomes: &targets,
	}
	view, err := prepareEditableFactoryPersistView("alpha", factory)
	if err != nil {
		t.Fatalf("prepareEditableFactoryPersistView: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(view.Canonical, &decoded); err != nil {
		t.Fatalf("Unmarshal canonical: %v", err)
	}
	if _, ok := decoded["layoutOutcomes"]; ok {
		t.Fatalf("save request canonical must not include layoutOutcomes: %#v", decoded["layoutOutcomes"])
	}
}

func namedFactoryPayloadWithStaleLayout(t *testing.T) []byte {
	t.Helper()

	factory, err := factoryvalidation.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &factoryapi.FactoryLayout{
		SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
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

func TestWithLayoutOutcomes_AttachesTargetsToSaveResponse(t *testing.T) {
	t.Parallel()

	factory := factoryapi.Factory{Name: "alpha"}
	updated := withLayoutOutcomes(factory, []factoryvalidation.Target{{
		Code:     factoryvalidation.CodeLayoutUnknownEdgeReference,
		Severity: factoryvalidation.SeverityWarning,
		Message:  "pruned",
		Subject: factoryvalidation.Subject{
			Type:     factoryvalidation.SubjectTypeFactory,
			ID:       "edge-1",
			Location: factoryvalidation.SubjectLocationReference,
		},
	}})
	if updated.LayoutOutcomes == nil || len(*updated.LayoutOutcomes) != 1 {
		t.Fatalf("layoutOutcomes = %#v, want one target", updated.LayoutOutcomes)
	}
}
