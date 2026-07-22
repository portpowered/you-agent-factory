package factorydefinition_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPreparePersistedFactoryPayload_NormalizesInlineBodiesOutOfCanonicalJSON(t *testing.T) {
	t.Parallel()
	body := "You are the planner."
	factory := factoryapi.Factory{
		Name: "alpha",
		Workers: &[]factoryapi.Worker{{
			Name: "planner",
			Type: workerTypeModel(),
			Body: &body,
		}},
	}
	version := factoryapi.HybridLogicalTimestamp{
		Logical:  3,
		Physical: time.Date(2026, 5, 31, 14, 0, 0, 0, time.UTC),
	}

	prepared, err := preparePersistedFactoryPayload("alpha", factory, version)
	if err != nil {
		t.Fatalf("preparePersistedFactoryPayload: %v", err)
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

func TestValidateEditableFactoryTopology_UsesSameNormalizationAsPersist(t *testing.T) {
	t.Parallel()
	body := "inline worker body"
	factory := factoryapi.Factory{
		Name: "alpha",
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "task",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name: "worker-a",
			Type: workerTypeModel(),
			Body: &body,
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "process",
			Worker: "worker-a",
			Type:   workstationTypeModel(),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "task", State: "complete"},
			},
		}},
	}

	view, err := prepareEditableFactoryPersistView("factory", factory)
	if err != nil {
		t.Fatalf("prepareEditableFactoryPersistView: %v", err)
	}
	if view.Config == nil || len(view.Config.Workers) == 0 || view.Config.Workers[0].Body != body {
		t.Fatalf("expanded config should retain inline body for layout, got %#v", view.Config)
	}
	if strings.Contains(string(view.Canonical), body) {
		t.Fatalf("canonical bytes should omit inlined worker body, got %s", view.Canonical)
	}
}
