package factorydefinition_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func workerTypeModel() *factoryapi.WorkerType {
	value := factoryapi.WorkerTypeModelWorker
	return &value
}

func workstationTypeModel() *factoryapi.WorkstationType {
	value := factoryapi.WorkstationTypeModelWorkstation
	return &value
}

func TestSaveDefaultCurrentFactoryForSession_PersistsSplitLayout(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	host := &splitLayoutDefaultSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name: apisurface.DefaultCurrentFactoryName,
			Id:   stringPointer("root-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	replacement := factoryapi.Factory{
		Name: apisurface.DefaultCurrentFactoryName,
		Id:   stringPointer("root-runtime"),
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

	if _, err := host.SaveFactoryForSession(context.Background(), factorysessions.DefaultSessionID, factoryapi.FactorySaveModeReplaceCurrent, replacement); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !host.discardCalled {
		t.Fatal("expected backup discard after successful activation")
	}

	factoryJSON, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}
	if strings.Contains(string(factoryJSON), "Plan the story.") {
		t.Fatalf("factory.json should omit inlined workstation body after split save, got %s", factoryJSON)
	}

	workerAgents := filepath.Join(rootDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	workerBody, err := os.ReadFile(workerAgents)
	if err != nil {
		t.Fatalf("ReadFile(planner AGENTS.md): %v", err)
	}
	if !strings.Contains(string(workerBody), "You are the planner.") {
		t.Fatalf("planner AGENTS.md = %q, want planner body", workerBody)
	}

	workstationAgents := filepath.Join(rootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName)
	workstationBody, err := os.ReadFile(workstationAgents)
	if err != nil {
		t.Fatalf("ReadFile(plan-task AGENTS.md): %v", err)
	}
	if !strings.Contains(string(workstationBody), "Plan the story.") {
		t.Fatalf("plan-task AGENTS.md = %q, want workstation body", workstationBody)
	}
}

func TestSaveDefaultCurrentFactoryForSession_RestoresTreeOnActivationFailure(t *testing.T) {
	t.Parallel()
	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	staleWorkerDir := filepath.Join(rootDir, interfaces.WorkersDir, "stale-worker")
	if err := os.MkdirAll(staleWorkerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(stale-worker): %v", err)
	}
	staleMarker := filepath.Join(staleWorkerDir, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(staleMarker, []byte("STALE_MARKER"), 0o644); err != nil {
		t.Fatalf("WriteFile(stale AGENTS.md): %v", err)
	}

	host := &splitLayoutDefaultSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name: apisurface.DefaultCurrentFactoryName,
			Id:   stringPointer("root-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
		activateErr: errors.New("activation failed"),
	}

	replacement := factoryapi.Factory{
		Name: apisurface.DefaultCurrentFactoryName,
		Id:   stringPointer("root-runtime"),
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
			Name: "worker-a",
			Type: workerTypeModel(),
			Body: stringPointer("replacement worker"),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "process",
			Worker: "worker-a",
			Type:   workstationTypeModel(),
			Body:   stringPointer("replacement workstation"),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	_, err := host.SaveFactoryForSession(context.Background(), factorysessions.DefaultSessionID, factoryapi.FactorySaveModeReplaceCurrent, replacement)
	if err == nil {
		t.Fatal("expected activation failure")
	}
	if !host.restoreCalled {
		t.Fatalf("expected split-layout restore after activation failure, save error: %v", err)
	}
	if host.discardCalled {
		t.Fatal("did not expect backup discard after activation failure")
	}

	got, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if string(got) != string(initial) {
		t.Fatalf("factory.json after failed save = %q, want restored monolithic payload", got)
	}
	if data, err := os.ReadFile(staleMarker); err != nil {
		t.Fatalf("ReadFile(stale AGENTS.md): %v", err)
	} else if string(data) != "STALE_MARKER" {
		t.Fatalf("stale AGENTS.md = %q, want STALE_MARKER restored with prior tree", data)
	}
}

type splitLayoutDefaultSaveHost struct {
	sessionRootDir string
	current        factoryapi.Factory
	activateErr    error
	restoreCalled  bool
	discardCalled  bool
}

func (h *splitLayoutDefaultSaveHost) RequireSession(sessionID string) (*interfaces.DefinitionSession, error) {
	return &interfaces.DefinitionSession{
		ID:         sessionID,
		FactoryDir: h.sessionRootDir,
		FolderPath: h.sessionRootDir,
		IsDefault:  true,
	}, nil
}

func (h *splitLayoutDefaultSaveHost) GetCurrentFactoryForSession(_ context.Context, _ string) (factoryapi.Factory, error) {
	return h.current, nil
}

func (h *splitLayoutDefaultSaveHost) WithActivationLock(fn func() error) error {
	return fn()
}

func (h *splitLayoutDefaultSaveHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h *splitLayoutDefaultSaveHost) ActivateSessionEditableFactory(context.Context, *interfaces.DefinitionSession, string, string, string, factoryapi.FactoryName, string) error {
	return h.activateErr
}

func (h *splitLayoutDefaultSaveHost) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *interfaces.PreparedFactoryLayoutPayload,
) (*interfaces.FactorySplitLayoutReplaceResult, error) {
	if targetDir != h.sessionRootDir {
		return nil, errors.New("unexpected replace target dir")
	}
	result, err := replacePreparedFactoryLayoutForTest(targetDir, prepared)
	if err != nil {
		return nil, err
	}
	return &interfaces.FactorySplitLayoutReplaceResult{
		Restore: func() {
			h.restoreCalled = true
			result.Restore()
		},
		DiscardBackup: func() {
			h.discardCalled = true
			result.DiscardBackup()
		},
	}, nil
}

func (h *splitLayoutDefaultSaveHost) CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if h.current.Version == nil {
		return factoryapi.HybridLogicalTimestamp{}, errors.New("missing version")
	}
	return *h.current.Version, nil
}

func (h *splitLayoutDefaultSaveHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, errors.New("not implemented")
}

func (h *splitLayoutDefaultSaveHost) SerializeNamedFactoryUpsertResponse(factoryapi.FactoryName, loadedFactorySource) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, errors.New("not implemented")
}

func (h *splitLayoutDefaultSaveHost) RequireFreshEditableFactoryVersionAtRoot(
	rootDir string,
	name factoryapi.FactoryName,
	baseVersion *factoryapi.HybridLogicalTimestamp,
) error {
	currentVersion, err := h.CurrentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	return requireFreshEditableFactoryVersion(baseVersion, currentVersion)
}

func (h *splitLayoutDefaultSaveHost) NextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	return nextEditableFactoryVersion(current, now)
}

func (h *splitLayoutDefaultSaveHost) PreparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*interfaces.PreparedFactoryLayoutPayload, error) {
	return preparePersistedFactoryPayload(segment, factory, version)
}

func (h *splitLayoutDefaultSaveHost) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return saveFactoryThroughDefinition(h.sessionRootDir, h, ctx, sessionID, mode, request)
}

func stringPointer(value string) *string {
	return &value
}
