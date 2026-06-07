package factorysave

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSaveUpsertNamedAndActivateForSession_PersistsChosenTargetName(t *testing.T) {
	sessionRoot := t.TempDir()
	host := &upsertNamedSaveHost{
		sessionRootDir: sessionRoot,
		serialized: factoryapi.Factory{
			Name: "imported-target",
			Id:   stringPointer("imported-target-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := New(sessionRoot, factory.RealClock{}, func() factoryconfig.WorkstationLoader { return nil }, host)

	imported := factoryapi.Factory{
		Name: "imported-target",
		Id:   stringPointer("embedded-runtime"),
		WorkTypes: &[]factoryapi.WorkType{{
			Name: "story",
			States: []factoryapi.WorkState{
				{Name: "init", Type: factoryapi.WorkStateTypeINITIAL},
				{Name: "complete", Type: factoryapi.WorkStateTypeTERMINAL},
				{Name: "failed", Type: factoryapi.WorkStateTypeFAILED},
			},
		}},
		Workers: &[]factoryapi.Worker{{
			Name: "planner",
			Type: workerTypeModel(),
			Body: stringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   workstationTypeModel(),
			Body:   stringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	saved, err := svc.Save(
		context.Background(),
		"session-alpha",
		factoryapi.FactorySaveModeUpsertNamedAndActivate,
		imported,
	)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Name != "imported-target" {
		t.Fatalf("saved factory name = %q, want imported-target", saved.Name)
	}
	if host.activatedName != "imported-target" {
		t.Fatalf("activated factory name = %q, want imported-target", host.activatedName)
	}

	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(sessionRoot, "imported-target")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDir(imported-target): %v", err)
	}
	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if !strings.Contains(string(factoryJSON), `"name": "imported-target"`) &&
		!strings.Contains(string(factoryJSON), `"name":"imported-target"`) {
		t.Fatalf("factory.json name = %s, want imported-target", factoryJSON)
	}
}

type upsertNamedSaveHost struct {
	sessionRootDir string
	serialized     factoryapi.Factory
	activatedName  string
}

func (h *upsertNamedSaveHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return &factorysessions.LiveSession{
		ID: sessionID,
		SessionState: factorysessions.SessionState{
			FactoryDir: h.sessionRootDir,
			FolderPath: h.sessionRootDir,
		},
	}, nil
}

func (h *upsertNamedSaveHost) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (h *upsertNamedSaveHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (h *upsertNamedSaveHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h *upsertNamedSaveHost) ActivateSessionEditableFactory(
	_ context.Context,
	_ *factorysessions.LiveSession,
	_ string,
	_ string,
	_ string,
	name factoryapi.FactoryName,
	runtimeName string,
) error {
	h.activatedName = string(name)
	if runtimeName != string(name) {
		return nil
	}
	return nil
}

func (h *upsertNamedSaveHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (h *upsertNamedSaveHost) CurrentFactoryDefinitionVersionAtRoot(string, factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	return factoryapi.HybridLogicalTimestamp{}, nil
}

func (h *upsertNamedSaveHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, nil
}

func (h *upsertNamedSaveHost) SerializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	_ *factoryconfig.LoadedFactoryConfig,
) (factoryapi.Factory, error) {
	serialized := h.serialized
	serialized.Name = name
	return serialized, nil
}
