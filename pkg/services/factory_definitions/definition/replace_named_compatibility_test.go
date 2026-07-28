package factorydefinition_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSaveNamedCurrentFactoryForSession_PersistsSplitLayout(t *testing.T) {
	t.Parallel()
	sessionRoot := t.TempDir()
	payload := []byte(`{"name":"alpha","id":"alpha-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`)
	if _, err := persistNamedFactoryForTest(sessionRoot, "alpha", payload, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	factoryDir, err := externalDefinitionTestNamedPaths.ResolveExistingDir(sessionRoot, "alpha")
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

	if _, err := host.SaveFactoryForSession(context.Background(), "session-alpha", factoryapi.FactorySaveModeReplaceCurrent, replacement); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if host.replaceTargetDir != factoryDir {
		t.Fatalf("ReplaceFactoryLayoutAtDir target = %q, want named factory dir %q", host.replaceTargetDir, factoryDir)
	}

	factoryJSONPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	workerAgents := filepath.Join(factoryDir, factorydefinitions.WorkersDir, "planner", factorydefinitions.FactoryAgentsFileName)
	workerBody, err := os.ReadFile(workerAgents)
	if err != nil {
		t.Fatalf("ReadFile(planner AGENTS.md): %v", err)
	}
	if !strings.Contains(string(workerBody), "You are the planner.") {
		t.Fatalf("planner AGENTS.md = %q, want planner body", workerBody)
	}
}

func TestSaveNamedCurrentFactoryForSession_CoercesDriftedPayloadName(t *testing.T) {
	t.Parallel()
	sessionRoot := t.TempDir()
	payload := []byte(`{"name":"alpha","id":"alpha-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`)
	if _, err := persistNamedFactoryForTest(sessionRoot, "alpha", payload, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	factoryDir, err := externalDefinitionTestNamedPaths.ResolveExistingDir(sessionRoot, "alpha")
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

	replacement := factoryapi.Factory{
		Name: "imported-factory",
		Id:   stringPointer("imported-runtime"),
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

	saved, err := host.SaveFactoryForSession(context.Background(), "session-alpha", factoryapi.FactorySaveModeReplaceCurrent, replacement)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Name != "alpha" {
		t.Fatalf("saved factory name = %q, want alpha", saved.Name)
	}

	factoryJSONPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if !strings.Contains(string(factoryJSON), `"name": "alpha"`) &&
		!strings.Contains(string(factoryJSON), `"name":"alpha"`) {
		t.Fatalf("factory.json name = %s, want alpha", factoryJSON)
	}
	if strings.Contains(string(factoryJSON), "imported-factory") {
		t.Fatalf("factory.json should not contain drifted imported name, got %s", factoryJSON)
	}
}

type splitLayoutNamedSaveHost struct {
	sessionRootDir   string
	factoryDir       string
	current          factoryapi.Factory
	replaceTargetDir string
}

func (h *splitLayoutNamedSaveHost) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{
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

func (h *splitLayoutNamedSaveHost) ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, factoryapi.FactoryName, string) error {
	return nil
}

func (h *splitLayoutNamedSaveHost) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	h.replaceTargetDir = targetDir
	return replacePreparedFactoryLayoutForTest(targetDir, prepared)
}

func (h *splitLayoutNamedSaveHost) CurrentFactoryDefinitionVersionAtRoot(string, factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if h.current.Version == nil {
		return factoryapi.HybridLogicalTimestamp{}, nil
	}
	return *h.current.Version, nil
}

func (h *splitLayoutNamedSaveHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, nil
}

func (h *splitLayoutNamedSaveHost) SerializeNamedFactoryUpsertResponse(factoryapi.FactoryName, loadedFactorySource) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (h *splitLayoutNamedSaveHost) RequireFreshEditableFactoryVersionAtRoot(
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

func (h *splitLayoutNamedSaveHost) NextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	return nextEditableFactoryVersion(current, now)
}

func (h *splitLayoutNamedSaveHost) PreparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return preparePersistedFactoryPayload(segment, factory, version)
}

func (h *splitLayoutNamedSaveHost) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return saveFactoryThroughDefinition(h.sessionRootDir, h, ctx, sessionID, mode, request)
}

func TestSaveUpsertNamedAndActivateForSession_PersistsChosenTargetName(t *testing.T) {
	t.Parallel()
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

	saved, err := host.SaveFactoryForSession(
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

	factoryDir, err := externalDefinitionTestNamedPaths.ResolveExistingDir(sessionRoot, "imported-target")
	if err != nil {
		t.Fatalf("ResolveNamedFactoryDir(imported-target): %v", err)
	}
	factoryJSONPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
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

func (h *upsertNamedSaveHost) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{
		ID:         sessionID,
		FactoryDir: h.sessionRootDir,
		FolderPath: h.sessionRootDir,
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
	_ *factorydefinitions.DefinitionSession,
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

func (h *upsertNamedSaveHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (h *upsertNamedSaveHost) CurrentFactoryDefinitionVersionAtRoot(string, factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	return factoryapi.HybridLogicalTimestamp{}, nil
}

func (h *upsertNamedSaveHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	factoryDir, err := externalDefinitionTestNamedPaths.ResolveExistingDir(h.sessionRootDir, "imported-target")
	if err != nil {
		return nil, err
	}
	return factorydefinitioncomposition.LoadCurrent(factoryDir, nil)
}

func (h *upsertNamedSaveHost) SerializeNamedFactoryUpsertResponse(
	name factoryapi.FactoryName,
	_ loadedFactorySource,
) (factoryapi.Factory, error) {
	serialized := h.serialized
	serialized.Name = name
	return serialized, nil
}

func (h *upsertNamedSaveHost) RequireFreshEditableFactoryVersionAtRoot(
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

func (h *upsertNamedSaveHost) NextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	return nextEditableFactoryVersion(current, now)
}

func (h *upsertNamedSaveHost) PreparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return preparePersistedFactoryPayload(segment, factory, version)
}

func (h *upsertNamedSaveHost) SaveFactoryForSession(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	return saveFactoryThroughDefinition(h.sessionRootDir, h, ctx, sessionID, mode, request)
}
