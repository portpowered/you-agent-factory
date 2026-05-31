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
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSaveNamedCurrentFactoryForSession_PersistsSplitLayout(t *testing.T) {
	sessionRoot := t.TempDir()
	payload := []byte(`{"name":"alpha","id":"alpha-runtime","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if _, err := configpersist.PersistNamedFactory(sessionRoot, "alpha", payload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(sessionRoot, "alpha")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDir(alpha): %v", err)
	}

	host := &splitLayoutNamedSaveHost{
		sessionRootDir: sessionRoot,
		factoryDir:     factoryDir,
		current: factoryapi.Factory{
			Name: "alpha",
			Id:   stringPointer("alpha-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := New(sessionRoot, factory.RealClock{}, func() factoryconfig.WorkstationLoader { return nil }, host)

	replacement := factoryapi.Factory{
		Name: "alpha",
		Id:   stringPointer("alpha-runtime"),
		Version: &factoryapi.HybridLogicalTimestamp{
			Logical:  2,
			Physical: time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC),
		},
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

	if _, err := svc.Save(context.Background(), "session-alpha", factoryapi.FactorySaveModeReplaceCurrent, replacement); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if host.replaceTargetDir != factoryDir {
		t.Fatalf("ReplaceFactoryLayoutAtDir target = %q, want named factory dir %q", host.replaceTargetDir, factoryDir)
	}

	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	workerAgents := filepath.Join(factoryDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	workerBody, err := os.ReadFile(workerAgents)
	if err != nil {
		t.Fatalf("ReadFile(planner AGENTS.md): %v", err)
	}
	if !strings.Contains(string(workerBody), "You are the planner.") {
		t.Fatalf("planner AGENTS.md = %q, want planner body", workerBody)
	}
}

type splitLayoutNamedSaveHost struct {
	sessionRootDir   string
	factoryDir       string
	current          factoryapi.Factory
	replaceTargetDir string
}

func (h *splitLayoutNamedSaveHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return &factorysessions.LiveSession{
		ID:         sessionID,
		FactoryDir: h.sessionRootDir,
		FolderPath: h.sessionRootDir,
	}, nil
}

func (h *splitLayoutNamedSaveHost) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return h.current, nil
}

func (h *splitLayoutNamedSaveHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (h *splitLayoutNamedSaveHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h *splitLayoutNamedSaveHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, factoryapi.FactoryName, string) error {
	return nil
}

func (h *splitLayoutNamedSaveHost) ReplaceFactoryLayoutAtDir(targetDir string, payload []byte) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	h.replaceTargetDir = targetDir
	return factoryconfig.ReplaceFactoryLayoutAtDirWithResult(
		targetDir,
		payload,
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
}

func (h *splitLayoutNamedSaveHost) CurrentFactoryDefinitionVersionAtRoot(string, factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if h.current.Version == nil {
		return factoryapi.HybridLogicalTimestamp{}, nil
	}
	return *h.current.Version, nil
}

func (h *splitLayoutNamedSaveHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, nil
}

func (h *splitLayoutNamedSaveHost) SerializeNamedFactoryUpsertResponse(factoryapi.FactoryName, *factoryconfig.LoadedFactoryConfig) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}
