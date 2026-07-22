package factorydefinition

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/snapshotcapture"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

func TestSaveUpsertNamedAndActivateForSession_PersistsChosenTargetName(t *testing.T) {
	t.Parallel()
	sessionRoot := t.TempDir()
	host := &upsertDefinitionHost{
		sessionRootDir: sessionRoot,
		serialized: factoryapi.Factory{
			Name: "imported-target",
			Id:   upsertStringPointer("imported-target-runtime"),
			Version: &factoryapi.HybridLogicalTimestamp{
				Logical:  1,
				Physical: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	svc := New(host)

	imported := factoryapi.Factory{
		Name: "imported-target",
		Id:   upsertStringPointer("embedded-runtime"),
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
			Type: upsertWorkerTypeModel(),
			Body: upsertStringPointer("You are the planner."),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   upsertWorkstationTypeModel(),
			Body:   upsertStringPointer("Plan the story."),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	saved, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(context.Background(), "session-alpha", mustEditableFactoryForTest(t, imported))
	if err != nil {
		t.Fatalf("SaveUpsertNamedAndActivateForSession: %v", err)
	}
	if saved.Name != "imported-target" {
		t.Fatalf("saved factory name = %q, want imported-target", saved.Name)
	}
	if host.activatedName != "imported-target" {
		t.Fatalf("activated factory name = %q, want imported-target", host.activatedName)
	}

	factoryDir, err := definitionTestNamedPaths.ResolveExistingDir(sessionRoot, "imported-target")
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

func TestSaveUpsertNamedAndActivateForSession_ReplacesExistingNamedFactory(t *testing.T) {
	t.Parallel()
	sessionRoot := t.TempDir()
	versionTime := time.Date(2026, 5, 31, 11, 0, 0, 0, time.UTC)
	payload := namedFactoryPayload(t, "imported-target")
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal payload: %v", err)
	}
	decoded["version"] = map[string]any{
		"logical":  float64(2),
		"physical": versionTime.Format(time.RFC3339Nano),
	}
	versioned, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("Marshal versioned payload: %v", err)
	}
	if _, err := persistNamedFactoryForTest(sessionRoot, "imported-target", versioned, factoryvalidation.New(nil)); err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}

	host := &upsertDefinitionHost{sessionRootDir: sessionRoot}
	svc := New(host)
	replacement := factoryapi.Factory{
		Name: "imported-target",
		Id:   upsertStringPointer("embedded-runtime"),
		Version: &factoryapi.HybridLogicalTimestamp{
			Logical:  3,
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
			Type: upsertWorkerTypeModel(),
			Body: upsertStringPointer("replacement planner"),
		}},
		Workstations: &[]factoryapi.Workstation{{
			Name:   "plan-task",
			Worker: "planner",
			Type:   upsertWorkstationTypeModel(),
			Body:   upsertStringPointer("replacement workstation"),
			Inputs: []factoryapi.WorkstationIO{{WorkType: "story", State: "init"}},
			Outputs: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "complete"},
			},
			OnFailure: &[]factoryapi.WorkstationIO{
				{WorkType: "story", State: "failed"},
			},
		}},
	}

	saved, err := svc.SaveUpsertNamedSnapshotAndActivateForSession(context.Background(), "session-alpha", mustEditableFactoryForTest(t, replacement))
	if err != nil {
		t.Fatalf("SaveUpsertNamedAndActivateForSession: %v", err)
	}
	if saved.Name != "imported-target" {
		t.Fatalf("saved factory name = %q, want imported-target", saved.Name)
	}
	if host.activatedName != "imported-target" {
		t.Fatalf("activated factory name = %q, want imported-target", host.activatedName)
	}
}

type upsertDefinitionHost struct {
	sessionRootDir string
	serialized     factoryapi.Factory
	activatedName  string
}

func (h *upsertDefinitionHost) PersistRootDir() string { return h.sessionRootDir }

func (h *upsertDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return nil
}

func (h *upsertDefinitionHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}

func (h *upsertDefinitionHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return definitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (h *upsertDefinitionHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return definitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}
func (h *upsertDefinitionHost) PrepareFactoryLayoutPayload(
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
func (h *upsertDefinitionHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}
func (h *upsertDefinitionHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	return definitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}

func (h *upsertDefinitionHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	return portableconfig.Prepare(
		factoryDir,
		factoryConfig,
		includeInlineContent,
		factorydefinitions.CloneFactoryConfig,
		factorydefinitioncomposition.ApplySupportedFiles,
		factorydefinitioncomposition.ApplyStarterWork,
	)
}

func (h *upsertDefinitionHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorysnapshotcapture.NewExplicit(
		factorysnapshot.ObjectFromFactoryConfig,
	)(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}

func (h *upsertDefinitionHost) CurrentRuntimeConfig() loadedFactorySource { return nil }

func (h *upsertDefinitionHost) WorkflowID() string { return "" }

func (h *upsertDefinitionHost) RequireSession(sessionID string) (*interfaces.DefinitionSession, error) {
	return &interfaces.DefinitionSession{
		ID:         sessionID,
		FactoryDir: h.sessionRootDir,
		FolderPath: h.sessionRootDir,
	}, nil
}

func (h *upsertDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	factoryDir, err := definitionTestNamedPaths.ResolveExistingDir(h.sessionRootDir, "imported-target")
	if err != nil {
		return nil, err
	}
	return factorydefinitioncomposition.LoadCurrent(factoryDir, nil)
}

func (h *upsertDefinitionHost) SessionFactoryPersistRoot(*interfaces.DefinitionSession) string {
	return h.sessionRootDir
}

func (h *upsertDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *interfaces.FactorySnapshot) error {
	return validateDefinitionSnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (h *upsertDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*interfaces.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (h *upsertDefinitionHost) WithActivationLock(fn func() error) error { return fn() }

func (h *upsertDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (h *upsertDefinitionHost) ActivateSessionEditableFactory(
	_ context.Context,
	_ *interfaces.DefinitionSession,
	_ string,
	_ string,
	_ string,
	name string,
	runtimeName string,
) error {
	h.activatedName = name
	if runtimeName != name {
		return nil
	}
	return nil
}

func (h *upsertDefinitionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*interfaces.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (h *upsertDefinitionHost) SaveNow() time.Time {
	return time.Date(2026, 5, 31, 12, 0, 1, 0, time.UTC)
}

func (h *upsertDefinitionHost) RunSessionID() string { return "" }

func (h *upsertDefinitionHost) SessionForActivation(string) *interfaces.DefinitionSession { return nil }

func (h *upsertDefinitionHost) NamedFactoryActivationPaths(*interfaces.DefinitionSession) (string, string) {
	return "", ""
}

func (h *upsertDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *interfaces.DefinitionSession) error {
	return nil
}

func (h *upsertDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *interfaces.DefinitionSession, string, string, string, string) error {
	return nil
}

func upsertWorkerTypeModel() *factoryapi.WorkerType {
	value := factoryapi.WorkerTypeModelWorker
	return &value
}

func upsertWorkstationTypeModel() *factoryapi.WorkstationType {
	value := factoryapi.WorkstationTypeModelWorkstation
	return &value
}

func upsertStringPointer(value string) *string {
	return &value
}
