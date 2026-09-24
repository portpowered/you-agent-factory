package factorydefinition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/validationassert"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	platformsnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	factoryeditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	factorysnapshot "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestValidateEditableFactoryTopology_MatchesValidateFactoryAPIPrePersist(t *testing.T) {
	t.Parallel()

	factory, err := factoryfixtures.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}

	svc := newTestService(stubDefinitionHost{})
	saveErr := svc.ValidateEditableFactoryTopology(context.Background(), mustFactorySnapshot(factory))
	var topologyErr *factorydefinitions.ValidationTopologyError
	if !errors.As(saveErr, &topologyErr) {
		t.Fatalf("ValidateEditableFactoryTopology error = %v, want topology validation error", saveErr)
	}
	validationassert.HasDomainTargetCode(t, topologyErr.Targets, factoryvalidation.CodeDuplicateIdentifier)
}

func TestSaveReplaceCurrentForSession_RejectsStaleBaseVersion(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
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
	svc := newTestService(host, host.activationGateway())

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
			Worker: definitionStringPtr("planner"),
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
	t.Parallel()
	rootDir := t.TempDir()
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
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
	svc := newTestService(host, host.activationGateway())

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
			Worker: definitionStringPtr("planner"),
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
	if _, err := persistNamedFactoryForTest(sessionRoot, "alpha", versioned, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(sessionRoot, "alpha"); err != nil {
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
	svc := newTestService(host, host.activationGateway())
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
			Worker: definitionStringPtr("planner"),
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
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
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
	svc := newTestService(host, host.activationGateway())
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
			Worker: definitionStringPtr("planner"),
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
	if _, err := svc.PersistPayloadFromView(nil, factorydefinitions.FactoryVersion{}); err == nil {
		t.Fatal("PersistPayloadFromView: expected error for nil service")
	}
	if _, err := svc.PreparePersistedFactoryPayload("alpha", nil, factorydefinitions.FactoryVersion{}); err == nil {
		t.Fatal("PreparePersistedFactoryPayload: expected error for nil service")
	}
	if err := svc.ValidateEditableFactoryTopology(context.Background(), nil); err == nil {
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

	err := newTestService(stubDefinitionHost{}).ValidateUpsertNamedFactoryRequest(context.Background(), "bad/name", nil)
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("ValidateUpsertNamedFactoryRequest error = %v, want invalid named factory name", err)
	}
}

func validateDefinitionSnapshotForTest(
	ctx context.Context,
	snapshot *factorydefinitions.FactorySnapshot,
	loader factorydefinitions.WorkstationLoader,
) error {
	return factoryeditable.ValidateSnapshot(
		ctx,
		snapshot,
		loader,
		func(snapshot *factorydefinitions.FactorySnapshot, loader factorydefinitions.WorkstationLoader) (factorydefinitions.DefinitionValidationRequest, error) {
			return validationentry.MapEditableFactorySnapshot(snapshot, loader, testCanonicalFactoryLoader)
		},
		testFactoryDefinitionValidator(),
	)
}

func (h *splitLayoutSaveHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return validateDefinitionSnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func TestService_SerializeNamedFactoryUpsertResponse_ReturnsThinBundledFiles(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, rootDir, factoryfixtures.MinimalFactoryConfig())
	runtimeCfg, err := factorydefinitioncomposition.LoadCurrent(rootDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}

	got, err := newTestService(stubDefinitionHost{workflowID: "workflow-1"}).SerializeNamedFactoryUpsertResponse("alpha", runtimeCfg)
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
	_, err := newTestService(host, host.activationGateway()).SaveReplaceCurrentSnapshotForSession(context.Background(), factorysessions.DefaultSessionID, mustEditableFactoryForTest(t, factoryapi.Factory{}))
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

func (h *splitLayoutSaveHost) activationGateway() *trackingActivationGateway {
	return &trackingActivationGateway{
		saveNow:      time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC),
		runSessionID: factorysessions.DefaultSessionID,
		activateErr:  h.activateErr,
	}
}

func (h *splitLayoutSaveHost) PersistRootDir() string { return h.sessionRootDir }
func (h *splitLayoutSaveHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}
func (h *splitLayoutSaveHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}
func (h *splitLayoutSaveHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return definitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (h *splitLayoutSaveHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return definitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}
func (h *splitLayoutSaveHost) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return prepareFactoryLayoutForDefinitionTest(
		context.Background(),
		segment,
		payload,
		factoryvalidation.New(nil),
	)
}
func (h *splitLayoutSaveHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}
func (h *splitLayoutSaveHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	return definitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}
func (h *splitLayoutSaveHost) CurrentRuntimeConfig() loadedFactorySource {
	return nil
}
func (h *splitLayoutSaveHost) WorkflowID() string { return "" }

func (h *splitLayoutSaveHost) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	return &factorydefinitions.DefinitionSession{
		ID:         sessionID,
		FactoryDir: h.sessionRootDir,
		FolderPath: h.sessionRootDir,
		IsDefault:  true,
	}, nil
}

func (h *splitLayoutSaveHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, errors.New("not implemented")
}

func (h *splitLayoutSaveHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return h.sessionRootDir
}

func (h *splitLayoutSaveHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return mustFactorySnapshot(h.current), nil
}

func (h *splitLayoutSaveHost) ReplaceFactoryLayoutAtDir(
	targetDir string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	h.replaceCalled = true
	expectedDir := h.sessionRootDir
	if h.current.Name != apisurface.DefaultCurrentFactoryName {
		namedDir, err := definitionTestNamedPaths.ResolveExistingDir(h.sessionRootDir, string(h.current.Name))
		if err != nil {
			return nil, err
		}
		expectedDir = namedDir
	}
	if targetDir != expectedDir {
		return nil, fmt.Errorf("unexpected replace target dir %q, want %q", targetDir, expectedDir)
	}
	result, err := replacePreparedFactoryLayoutForTest(targetDir, prepared)
	if err != nil {
		return nil, err
	}
	return &factorydefinitions.FactorySplitLayoutReplaceResult{
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

type resultActivationGateway struct {
	*trackingActivationGateway
}

func (g *resultActivationGateway) ActivateSessionEditableFactoryWithResult(
	ctx context.Context,
	session *factorydefinitions.DefinitionSession,
	sessionID string,
	sessionRootDir string,
	factoryDir string,
	name string,
	runtimeName string,
) (factorydefinitions.DefinitionActivationResult, error) {
	if err := g.trackingActivationGateway.ActivateSessionEditableFactory(
		ctx,
		session,
		sessionID,
		sessionRootDir,
		factoryDir,
		name,
		runtimeName,
	); err != nil {
		return factorydefinitions.DefinitionActivationResult{}, err
	}
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		return factorydefinitions.DefinitionActivationResult{}, err
	}
	return factorydefinitions.DefinitionActivationResult{
		LoadedSource: loaded,
		Available:    true,
	}, nil
}

type resultAtomicSaveHost struct {
	*splitLayoutSaveHost
	captureErr       error
	loadErr          error
	failLoadAfter    int32
	loadCalls        atomic.Int32
	runtimeReadCalls atomic.Int32
	runtimeReadErr   error
}

func (h *resultAtomicSaveHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return snapshotsportabilityprepare.PrepareConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h *resultAtomicSaveHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	if h.captureErr != nil {
		return nil, h.captureErr
	}
	return platformsnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(factoryDir, factoryConfig, runtimeConfig, sourceDirectory, metadata)
}

func (h *resultAtomicSaveHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	call := h.loadCalls.Add(1)
	if h.failLoadAfter > 0 && call > h.failLoadAfter {
		return nil, h.loadErr
	}
	return h.splitLayoutSaveHost.LoadFactory(factoryDir, loader)
}

func (h *resultAtomicSaveHost) SessionRuntimeConfig(string) (factorydefinitions.LoadedFactorySource, error) {
	h.runtimeReadCalls.Add(1)
	return nil, h.runtimeReadErr
}

func TestSaveReplaceCurrent_PreparesResponseBeforeActivationFailure(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	initial := versionedNamedFactoryPayload(t, "root", 1, versionTime)
	initialPath := filepath.Join(rootDir, factorydefinitions.FactoryConfigFile)
	if err := os.WriteFile(initialPath, initial, 0o644); err != nil {
		t.Fatalf("write initial factory: %v", err)
	}

	responseErr := errors.New("response snapshot unavailable")
	host := &resultAtomicSaveHost{
		splitLayoutSaveHost: &splitLayoutSaveHost{
			sessionRootDir: rootDir,
			current: factoryapi.Factory{
				Name: apisurface.DefaultCurrentFactoryName,
				Id:   saveStringPointer("root-runtime"),
				Version: &factoryapi.HybridLogicalTimestamp{
					Logical:  1,
					Physical: versionTime,
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	replacement := editableFactoryFromPayload(t, namedFactoryPayload(t, "replacement"))
	replacement.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	_, err := svc.SaveReplaceCurrentSnapshotForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		replacement,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("save error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.restoreCalled {
		t.Fatal("expected durable layout restore after response preparation failure")
	}
	got, readErr := os.ReadFile(initialPath)
	if readErr != nil {
		t.Fatalf("read restored factory: %v", readErr)
	}
	if string(got) != string(initial) {
		t.Fatalf("restored factory = %q, want original %q", got, initial)
	}
}

func TestSaveReplaceCurrent_UsesPreparedResponseWithoutPostSwapVersionOrRuntimeReads(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if err := os.WriteFile(
		filepath.Join(rootDir, factorydefinitions.FactoryConfigFile),
		versionedNamedFactoryPayload(t, "root", 1, versionTime),
		0o644,
	); err != nil {
		t.Fatalf("write initial factory: %v", err)
	}
	host := &resultAtomicSaveHost{
		splitLayoutSaveHost: &splitLayoutSaveHost{
			sessionRootDir: rootDir,
			current: factoryapi.Factory{
				Name: apisurface.DefaultCurrentFactoryName,
				Id:   saveStringPointer("root-runtime"),
				Version: &factoryapi.HybridLogicalTimestamp{
					Logical:  1,
					Physical: versionTime,
				},
			},
		},
		failLoadAfter:  2,
		loadErr:        errors.New("post-swap version read must not happen"),
		runtimeReadErr: errors.New("post-swap runtime config read must not happen"),
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	replacement := editableFactoryFromPayload(t, namedFactoryPayload(t, "replacement"))
	replacement.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	saved, err := svc.SaveReplaceCurrentSnapshotForSession(
		context.Background(),
		factorysessions.DefaultSessionID,
		replacement,
	)
	if err != nil {
		t.Fatalf("SaveReplaceCurrentSnapshotForSession: %v", err)
	}
	if gateway.activateCalls.Load() != 1 {
		t.Fatalf("activation calls = %d, want 1", gateway.activateCalls.Load())
	}
	if host.loadCalls.Load() != 2 {
		t.Fatalf("host LoadFactory calls = %d, want current and preflight candidate only", host.loadCalls.Load())
	}
	if host.runtimeReadCalls.Load() != 0 {
		t.Fatalf("post-swap runtime config reads = %d, want 0", host.runtimeReadCalls.Load())
	}
	if saved == nil {
		t.Fatal("saved response is missing snapshot")
	}
	response, mapErr := factorySnapshotForCompatibilityTest(saved)
	if mapErr != nil || response.Version == nil {
		t.Fatalf("map saved response = %#v, %v; want version", response, mapErr)
	}
	if got := response.Version.Logical.Int64(); got != 2 {
		t.Fatalf("saved version logical = %d, want 2", got)
	}
	if !host.discardCalled {
		t.Fatal("expected successful replacement to discard its backup")
	}
}

type resultAtomicUpsertHost struct {
	*atomicUpsertSaveHost
	captureErr error
}

func (h *resultAtomicUpsertHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return snapshotsportabilityprepare.PrepareConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h *resultAtomicUpsertHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	if h.captureErr != nil {
		return nil, h.captureErr
	}
	return platformsnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(factoryDir, factoryConfig, runtimeConfig, sourceDirectory, metadata)
}

func TestSaveUpsertNamed_PreparesResponseBeforeActivationAndDiscardsCreatedCandidate(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	if _, err := persistNamedFactoryForTest(
		rootDir,
		"alpha",
		versionedNamedFactoryPayload(t, "alpha", 1, versionTime),
		factoryvalidation.New(nil),
	); err != nil {
		t.Fatalf("persist alpha: %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("write alpha pointer: %v", err)
	}

	responseErr := errors.New("upsert response snapshot unavailable")
	host := &resultAtomicUpsertHost{
		atomicUpsertSaveHost: &atomicUpsertSaveHost{
			splitLayoutSaveHost: &splitLayoutSaveHost{
				sessionRootDir: rootDir,
				current: factoryapi.Factory{
					Name:    "alpha",
					Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	candidate := editableFactoryFromPayload(t, namedFactoryPayload(t, "beta"))

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		candidate,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("upsert error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.discardCalled {
		t.Fatal("expected created candidate discard after response preparation failure")
	}
	if got, readErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); readErr != nil || got != "alpha" {
		t.Fatalf("current pointer after failed upsert = %q, error=%v, want alpha", got, readErr)
	}
	if _, resolveErr := definitionTestNamedPaths.ResolveExistingDir(rootDir, "beta"); !errors.Is(resolveErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("failed candidate resolution error = %v, want named Factory not found", resolveErr)
	}
}

func TestSaveUpsertNamed_PreparesResponseBeforeActivationAndRestoresExistingLayout(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	versionTime := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	initial := versionedNamedFactoryPayload(t, "alpha", 1, versionTime)
	if _, err := persistNamedFactoryForTest(rootDir, "alpha", initial, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("persist alpha: %v", err)
	}
	if err := definitionTestNamedPaths.WriteCurrentPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("write alpha pointer: %v", err)
	}

	responseErr := errors.New("existing upsert response unavailable")
	host := &resultAtomicUpsertHost{
		atomicUpsertSaveHost: &atomicUpsertSaveHost{
			splitLayoutSaveHost: &splitLayoutSaveHost{
				sessionRootDir: rootDir,
				current: factoryapi.Factory{
					Name:    "alpha",
					Version: &factoryapi.HybridLogicalTimestamp{Logical: 1, Physical: versionTime},
				},
			},
		},
		captureErr: responseErr,
	}
	gateway := &resultActivationGateway{trackingActivationGateway: host.activationGateway()}
	svc := newTestService(host, gateway)
	candidate := editableFactoryFromPayload(t, namedFactoryPayload(t, "alpha"))
	candidate.Version = &factorydefinitions.FactoryVersion{Logical: 2, Physical: versionTime.Add(time.Second)}

	_, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(
		context.Background(),
		factoryapiSessionIDForTest,
		candidate,
	)
	if !errors.Is(err, responseErr) {
		t.Fatalf("upsert error = %v, want response snapshot error", err)
	}
	if gateway.activateCalls.Load() != 0 {
		t.Fatalf("activation calls = %d, want 0 before response preparation", gateway.activateCalls.Load())
	}
	if !host.restoreCalled {
		t.Fatal("expected existing layout restore after response preparation failure")
	}
	got, readErr := os.ReadFile(filepath.Join(rootDir, "alpha", factorydefinitions.FactoryConfigFile))
	if readErr != nil {
		t.Fatalf("read restored alpha: %v", readErr)
	}
	if !strings.Contains(string(got), `"logical": "1"`) && !strings.Contains(string(got), `"logical":1`) {
		t.Fatalf("restored alpha payload = %q, want version 1", got)
	}
	if pointer, pointerErr := definitionTestNamedPaths.ReadCurrentPointer(rootDir); pointerErr != nil || pointer != "alpha" {
		t.Fatalf("current pointer after failed replacement = %q, error=%v, want alpha", pointer, pointerErr)
	}
}
