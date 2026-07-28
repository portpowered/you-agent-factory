package factorydefinition

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	generatedapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func mustEditableFactorySnapshot(t testing.TB, factory generatedapi.Factory) *interfaces.FactorySnapshot {
	t.Helper()
	snapshot, err := interfaces.NewFactorySnapshot(factory)
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	return snapshot
}

func workerTypeModel() *generatedapi.WorkerType {
	value := generatedapi.WorkerTypeModelWorker
	return &value
}

func workstationTypeModel() *generatedapi.WorkstationType {
	value := generatedapi.WorkstationTypeModelWorkstation
	return &value
}

func TestService_PreparePersistedFactoryPayload_NormalizesInlineBodiesOutOfCanonicalJSON(t *testing.T) {
	t.Parallel()

	body := "You are the planner."
	factory := generatedapi.Factory{
		Name: "alpha",
		Workers: &[]generatedapi.Worker{{
			Name: "planner",
			Type: workerTypeModel(),
			Body: &body,
		}},
	}
	version := interfaces.FactoryVersion{
		Logical:  3,
		Physical: time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC),
	}

	prepared, err := newTestService(stubDefinitionHost{}).PreparePersistedFactoryPayload("alpha", mustEditableFactorySnapshot(t, factory), version)
	if err != nil {
		t.Fatalf("PreparePersistedFactoryPayload: %v", err)
	}
	if strings.Contains(string(prepared.Canonical), body) {
		t.Fatalf("persist payload should omit inlined worker body, got %s", prepared.Canonical)
	}
	if prepared.Config == nil || len(prepared.Config.Workers) == 0 || prepared.Config.Workers[0].Body != body {
		t.Fatalf("expanded config should retain inline body for split layout, got %#v", prepared.Config)
	}

	var decoded map[string]any
	if err := json.Unmarshal(prepared.Canonical, &decoded); err != nil {
		t.Fatalf("Unmarshal(payload): %v", err)
	}
	versionObj, ok := decoded["version"].(map[string]any)
	if !ok {
		t.Fatalf("version = %#v, want object", decoded["version"])
	}
	if versionObj["logical"] != "3" {
		t.Fatalf("version.logical = %#v, want 3", versionObj["logical"])
	}
}

func TestService_PreparePersistedFactoryPayload_RejectsMissingSnapshot(t *testing.T) {
	t.Parallel()

	_, err := newTestService(stubDefinitionHost{}).PreparePersistedFactoryPayload("alpha", nil, interfaces.FactoryVersion{})
	if err == nil || !strings.Contains(err.Error(), "editable factory snapshot is required") {
		t.Fatalf("PreparePersistedFactoryPayload() error = %v, want missing snapshot guidance", err)
	}
}

func TestService_PrepareEditableFactoryPersistView_UsesSameNormalizationAsPersist(t *testing.T) {
	t.Parallel()

	body := "inline worker body"
	factory := generatedapi.Factory{
		Name: "alpha",
		WorkTypes: &[]generatedapi.WorkType{{
			Name: "task",
			States: []generatedapi.WorkState{
				{Name: "init", Type: generatedapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: generatedapi.WorkStateTypeTERMINAL},
			},
		}},
		Workers: &[]generatedapi.Worker{{
			Name: "worker-a",
			Type: workerTypeModel(),
			Body: &body,
		}},
		Workstations: &[]generatedapi.Workstation{{
			Name:   "process",
			Worker: "worker-a",
			Type:   workstationTypeModel(),
			Inputs: []generatedapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs: &[]generatedapi.WorkstationIO{
				{WorkType: "task", State: "complete"},
			},
		}},
	}

	view, err := newTestService(stubDefinitionHost{}).PrepareEditableFactoryPersistView("factory", mustEditableFactorySnapshot(t, factory))
	if err != nil {
		t.Fatalf("PrepareEditableFactoryPersistView: %v", err)
	}
	if view.Config == nil || len(view.Config.Workers) == 0 || view.Config.Workers[0].Body != body {
		t.Fatalf("expanded config should retain inline body for layout, got %#v", view.Config)
	}
	if strings.Contains(string(view.Canonical), body) {
		t.Fatalf("canonical bytes should omit inlined worker body, got %s", view.Canonical)
	}
}

func TestService_PreparePersistedFactoryPayload_PrunesStaleLayout(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &generatedapi.FactoryLayout{
		SchemaVersion: interfaces.SupportedFactoryLayoutSchemaVersion,
		Nodes: &[]generatedapi.FactoryLayoutNode{{
			Id:       "workstation:process",
			Position: generatedapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &generatedapi.FactoryLayoutSize{Width: 100, Height: 80},
		}, {
			Id:       "workstation:stale-node",
			Position: generatedapi.FactoryLayoutPoint{X: 30, Y: 40},
			Size:     &generatedapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &generatedapi.FactoryLayoutViewport{Zoom: 1},
	}

	prepared, err := newTestService(stubDefinitionHost{}).PreparePersistedFactoryPayload("alpha", mustEditableFactorySnapshot(t, factory), interfaces.FactoryVersion{
		Logical:  2,
		Physical: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PreparePersistedFactoryPayload: %v", err)
	}
	if prepared.Config == nil || prepared.Config.Layout == nil {
		t.Fatal("expected pruned layout on prepared config")
	}
	if len(prepared.Config.Layout.Nodes) != 1 || prepared.Config.Layout.Nodes[0].ID != "workstation:process" {
		t.Fatalf("pruned layout nodes = %#v", prepared.Config.Layout.Nodes)
	}
}

func TestService_PreparePersistedFactoryPayload_PreservesUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathValidAlphaFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathValidAlphaFactory: %v", err)
	}
	factory.Layout = &generatedapi.FactoryLayout{
		SchemaVersion: 99,
		Nodes: &[]generatedapi.FactoryLayoutNode{{
			Id:       "workstation:process",
			Position: generatedapi.FactoryLayoutPoint{X: 10, Y: 20},
			Size:     &generatedapi.FactoryLayoutSize{Width: 100, Height: 80},
		}},
		Viewport: &generatedapi.FactoryLayoutViewport{Zoom: 1},
	}

	prepared, err := newTestService(stubDefinitionHost{}).PreparePersistedFactoryPayload("alpha", mustEditableFactorySnapshot(t, factory), interfaces.FactoryVersion{
		Logical:  2,
		Physical: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PreparePersistedFactoryPayload: %v", err)
	}
	if prepared.Config == nil || prepared.Config.Layout == nil {
		t.Fatal("expected layout preserved on prepared config")
	}
	if prepared.Config.Layout.SchemaVersion != 99 {
		t.Fatalf("layout schemaVersion = %d, want 99 preserved on save", prepared.Config.Layout.SchemaVersion)
	}
}

func TestService_PersistPayloadFromView_StampsVersionMetadata(t *testing.T) {
	t.Parallel()

	body := "inline worker body"
	factory := generatedapi.Factory{
		Name: "alpha",
		Workers: &[]generatedapi.Worker{{
			Name: "worker-a",
			Type: workerTypeModel(),
			Body: &body,
		}},
	}
	view, err := newTestService(stubDefinitionHost{}).PrepareEditableFactoryPersistView("alpha", mustEditableFactorySnapshot(t, factory))
	if err != nil {
		t.Fatalf("PrepareEditableFactoryPersistView: %v", err)
	}
	version := interfaces.FactoryVersion{
		Logical:  9,
		Physical: time.Date(2026, 6, 8, 8, 0, 0, 0, time.UTC),
	}

	prepared, err := newTestService(stubDefinitionHost{}).PersistPayloadFromView(view, version)
	if err != nil {
		t.Fatalf("PersistPayloadFromView: %v", err)
	}
	if !strings.Contains(string(prepared.Canonical), `"logical":"9"`) {
		t.Fatalf("canonical payload = %s, want stamped version logical=9", prepared.Canonical)
	}
}

func TestService_SerializeNamedFactory_ReturnsLoadedRuntime(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	runtimeCfg, err := factorydefinitioncomposition.LoadCurrent(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	got, err := newTestService(stubDefinitionHost{}).SerializeNamedFactory("alpha", runtimeCfg, true)
	if err != nil {
		t.Fatalf("SerializeNamedFactory: %v", err)
	}
	mapped, err := factorySnapshotForCompatibilityTest(got)
	if err != nil {
		t.Fatalf("map SerializeNamedFactory result: %v", err)
	}
	if mapped.Name != "alpha" {
		t.Fatalf("factory name = %q, want alpha", mapped.Name)
	}
}

func TestService_PersistPayloadFromView_RejectsNilView(t *testing.T) {
	t.Parallel()

	_, err := newTestService(stubDefinitionHost{}).PersistPayloadFromView(nil, interfaces.FactoryVersion{
		Logical:  1,
		Physical: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil view")
	}
}
