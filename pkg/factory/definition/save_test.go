package factorydefinition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIPrePersist(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	svc := New(stubDefinitionHost{})
	saveErr := svc.ValidateEditableFactoryTopology(mustFactorySnapshot(factory))
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("ValidateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}
	validationassert.HasTargetCode(t, topologyErr.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestSaveReplaceCurrentForSession_RejectsStaleBaseVersion(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	currentVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  5,
		Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
	}
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"5","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	host := &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name:    apisurface.DefaultCurrentFactoryName,
			Id:      saveStringPointer("root-runtime"),
			Version: &currentVersion,
		},
	}
	svc := New(host)

	staleVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  4,
		Physical: currentVersion.Physical.Add(time.Second),
	}
	replacement := factoryapi.Factory{
		Name:    apisurface.DefaultCurrentFactoryName,
		Id:      saveStringPointer("root-runtime"),
		Version: &staleVersion,
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
			Type: saveWorkerTypeModel(),
			Body: saveStringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   saveWorkstationTypeModel(),
			Body:   saveStringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	_, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, replacement))
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveReplaceCurrentForSession error = %v, want %v", err, apisurface.ErrFactoryVersionStale)
	}
	if host.replaceCalled {
		t.Fatal("expected stale save to skip split-layout replacement")
	}

	factoryJSON, err := os.ReadFile(initialPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should remain unchanged after stale save, got %s", factoryJSON)
	}
}

func TestSaveReplaceCurrentForSession_PersistsSplitLayout(t *testing.T) {
	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	host := &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		current: factoryapi.Factory{
			Name: apisurface.DefaultCurrentFactoryName,
			Id:   saveStringPointer("root-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := New(host)

	replacement := factoryapi.Factory{
		Name: apisurface.DefaultCurrentFactoryName,
		Id:   saveStringPointer("root-runtime"),
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
			Type: saveWorkerTypeModel(),
			Body: saveStringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   saveWorkstationTypeModel(),
			Body:   saveStringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	if _, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, replacement)); err != nil {
		t.Fatalf("SaveReplaceCurrentForSession: %v", err)
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
}

func TestSaveReplaceCurrentForSession_ReplacesNamedCurrentFactoryLayout(t *testing.T) {
	t.Parallel()

	sessionRoot := t.TempDir()
	versionTime := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	payload := namedFactoryPayload(t, "alpha")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(1),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := config.PersistNamedFactory(sessionRoot, "alpha", versioned); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(sessionRoot, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	host := &splitLayoutSaveHost{
		sessionRootDir: sessionRoot,
		current: factoryapi.Factory{
			Name: "alpha",
			Id:   saveStringPointer("alpha"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: versionTime,
			},
		},
	}
	svc := New(host)
	replacement := factoryapi.Factory{
		Name: "alpha",
		Id:   saveStringPointer("alpha"),
		Version: &factoryapi.HybridLogicalTimestamp{
			Logical:  2,
			Physical: versionTime.Add(time.Second),
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
			Type: saveWorkerTypeModel(),
			Body: saveStringPointer("named replacement planner"),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   saveWorkstationTypeModel(),
			Body:   saveStringPointer("Plan the named story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	if _, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, replacement)); err != nil {
		t.Fatalf("SaveReplaceCurrentForSession: %v", err)
	}
	if !host.replaceCalled {
		t.Fatal("expected split-layout replacement for named current factory")
	}
}

func TestSaveReplaceCurrentForSession_RestoresLayoutWhenActivationFails(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	initial := []byte(`{"name":"root","id":"root-runtime","version":{"logical":"1","physical":"2026-05-31T12:00:00Z"},"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],"workers":[{"name":"worker-a","type":"MODEL_WORKER","body":"initial worker"}],"workstations":[{"name":"process","worker":"worker-a","type":"MODEL_WORKSTATION","body":"initial workstation","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}]}]}`)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	host := &splitLayoutSaveHost{
		sessionRootDir: rootDir,
		activateErr:    errors.New("activation failed"),
		current: factoryapi.Factory{
			Name: apisurface.DefaultCurrentFactoryName,
			Id:   saveStringPointer("root-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := New(host)
	replacement := factoryapi.Factory{
		Name: apisurface.DefaultCurrentFactoryName,
		Id:   saveStringPointer("root-runtime"),
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
			Type: saveWorkerTypeModel(),
			Body: saveStringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   saveWorkstationTypeModel(),
			Body:   saveStringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	_, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, replacement))
	if err == nil || err.Error() != "activation failed" {
		t.Fatalf("SaveReplaceCurrentForSession error = %v, want activation failed", err)
	}
	if !host.restoreCalled {
		t.Fatal("expected layout restore after activation failure")
	}
}

func TestService_NilReceiverReturnsRequiredErrors(t *testing.T) {
	t.Parallel()

	var svc *Service
	if _, err := svc.GetCurrentNamedFactory(context.Background()); err == nil {
		t.Fatal("GetCurrentNamedFactory: expected error for nil service")
	}
	if _, err := svc.GetCurrentFactoryForSession(context.Background(), "session"); err == nil {
		t.Fatal("GetCurrentFactoryForSession: expected error for nil service")
	}
	if _, err := svc.CurrentFactoryDefinitionVersionAtRoot(t.TempDir(), "alpha"); err == nil {
		t.Fatal("CurrentFactoryDefinitionVersionAtRoot: expected error for nil service")
	}
	if _, err := svc.SerializeNamedFactory("alpha", nil, true); err == nil {
		t.Fatal("SerializeNamedFactory: expected error for nil service")
	}
	if _, err := svc.PrepareEditableFactoryPersistView("alpha", nil); err == nil {
		t.Fatal("PrepareEditableFactoryPersistView: expected error for nil service")
	}
	if _, err := svc.PersistPayloadFromView(nil, interfaces.FactoryVersion{}); err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil service")
	}
	if _, err := svc.PreparePersistedFactoryPayload("alpha", nil, interfaces.FactoryVersion{}); err == nil {
		t.Fatal("PreparePersistedFactoryPayload: expected error for nil service")
	}
	if err := svc.ValidateEditableFactoryTopology(nil); err == nil {
		t.Fatal("ValidateEditableFactoryTopology: expected error for nil service")
	}
	if _, err := svc.SaveReplaceCurrentSnapshotForSession(context.Background(), "session", EditableFactory{}); err == nil {
		t.Fatal("SaveReplaceCurrentForSession: expected error for nil service")
	}
	if _, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(context.Background(), "session", EditableFactory{}); err == nil {
		t.Fatal("SaveUpsertNamedAndActivateForSession: expected error for nil service")
	}
	if err := svc.ActivateNamedFactory(context.Background(), "alpha"); err == nil {
		t.Fatal("ActivateNamedFactory: expected error for nil service")
	}
}

func TestValidateUpsertNamedFactoryRequest_RejectsInvalidFactoryName(t *testing.T) {
	t.Parallel()

	err := New(stubDefinitionHost{}).ValidateUpsertNamedFactoryRequest("bad/name", nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("ValidateUpsertNamedFactoryRequest error = %v, want invalid named factory name", err)
	}
}

func validateDefinitionSnapshotForTest(snapshot *interfaces.FactorySnapshot, loader factoryconfig.WorkstationLoader) error {
	return validationentry.ValidateEditableFactorySnapshot(snapshot, loader)
}

func (h *splitLayoutSaveHost) ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	return validateDefinitionSnapshotForTest(snapshot, h.WorkstationLoader())
}

func TestService_SerializeNamedFactoryUpsertResponse_ReturnsThinBundledFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	runtimeCfg, err := config.LoadRuntimeConfig(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	got, err := New(stubDefinitionHost{workflowID: "workflow-1"}).SerializeNamedFactoryUpsertResponse("alpha", runtimeCfg)
	if err != nil {
		t.Fatalf("SerializeNamedFactoryUpsertResponse: %v", err)
	}
	var decoded struct {
		Name string `json:"name"`
	}
	if err := got.Decode(&decoded); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.Name != "alpha" {
		t.Fatalf("factory name = %q, want alpha", decoded.Name)
	}
}

func TestSaveReplaceCurrentForSession_RejectsInvalidWritableCurrentName(t *testing.T) {
	t.Parallel()

	host := &splitLayoutSaveHost{
		sessionRootDir: t.TempDir(),
		current: factoryapi.Factory{
			Name: "bad/name",
		},
	}
	_, err := New(host).SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, factoryapi.Factory{}))
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("SaveReplaceCurrentForSession error = %v, want invalid named factory name", err)
	}
}

type splitLayoutSaveHost struct {
	sessionRootDir string
	current        factoryapi.Factory
	activateErr    error
	replaceCalled  bool
	restoreCalled  bool
	discardCalled  bool
}

func (h *splitLayoutSaveHost) PersistRootDir() string { return h.sessionRootDir }
func (h *splitLayoutSaveHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	return nil
}
func (h *splitLayoutSaveHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return nil
}
func (h *splitLayoutSaveHost) WorkflowID() string { return "" }

func (h *splitLayoutSaveHost) RequireSession(sessionID string) (*factorysessions.LiveSession, error) {
	return &factorysessions.LiveSession{
		ID: sessionID,
		SessionState: factorysessions.SessionState{
			FactoryDir: h.sessionRootDir,
			FolderPath: h.sessionRootDir,
		},
		IsDefault: true,
	}, nil
}

func (h *splitLayoutSaveHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, errors.New("not implemented")
}

func (h *splitLayoutSaveHost) SessionFactoryPersistRoot(*factorysessions.LiveSession) string {
	return h.sessionRootDir
}

func (h *splitLayoutSaveHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*interfaces.FactorySnapshot, error) {
	return mustFactorySnapshot(h.current), nil
}

func (h *splitLayoutSaveHost) WithActivationLock(fn func() error) error { return fn() }

func (h *splitLayoutSaveHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h *splitLayoutSaveHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, string, string) error {
	return h.activateErr
}

func (h *splitLayoutSaveHost) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	h.replaceCalled = true
	expectedDir := h.sessionRootDir
	if h.current.Name != apisurface.DefaultCurrentFactoryName {
		namedDir, err := factoryconfig.ResolveNamedFactoryDir(h.sessionRootDir, string(h.current.Name))
		if err != nil {
			return nil, err
		}
		expectedDir = namedDir
	}
	if targetDir != expectedDir {
		return nil, fmt.Errorf("unexpected replace target dir %q, want %q", targetDir, expectedDir)
	}
	result, err := factoryconfig.ReplaceFactoryLayoutAtDirWithPreparedWithResult(
		targetDir,
		prepared,
		factoryconfig.DefaultFactoryLayoutReplaceOptions(targetDir),
	)
	if err != nil {
		return nil, err
	}
	return &factoryconfig.FactorySplitLayoutReplaceResult{
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

func (h *splitLayoutSaveHost) SaveNow() time.Time {
	return time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC)
}

func (h *splitLayoutSaveHost) RunSessionID() string { return factorysessions.DefaultSessionID }

func (h *splitLayoutSaveHost) SessionForActivation(string) *factorysessions.LiveSession {
	return nil
}

func (h *splitLayoutSaveHost) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return "", ""
}

func (h *splitLayoutSaveHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorysessions.LiveSession) error {
	return nil
}

func (h *splitLayoutSaveHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorysessions.LiveSession, string, string, string, string) error {
	return nil
}

func saveWorkerTypeModel() *factoryapi.WorkerType {
	value := factoryapi.WorkerTypeModelWorker
	return &value
}

func saveWorkstationTypeModel() *factoryapi.WorkstationType {
	value := factoryapi.WorkstationTypeModelWorkstation
	return &value
}

func saveStringPointer(value string) *string {
	return &value
}
