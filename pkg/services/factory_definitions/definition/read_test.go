package factorydefinition

import (
	"context"
	"encoding/json"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

	factorysnapshotcapture "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/capture"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/prepare"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/impl"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorysnapshot"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type stubDefinitionHost struct {
	persistRootDir     string
	workstationLoader  factorydefinitions.WorkstationLoader
	currentRuntime     loadedFactorySource
	workflowID         string
	session            *factorydefinitions.DefinitionSession
	sessionRuntime     loadedFactorySource
	sessionPersistRoot string
	requireSessionErr  error
	sessionRuntimeErr  error
}

func (h stubDefinitionHost) PersistRootDir() string { return h.persistRootDir }
func (h stubDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return h.workstationLoader
}
func (h stubDefinitionHost) LoadFactory(
	factoryDir string,
	loader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	return factorydefinitioncomposition.LoadCurrent(factoryDir, loader)
}
func (h stubDefinitionHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	return definitionTestNamedPaths.ReadCurrentPointer(rootDir)
}
func (h stubDefinitionHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	return definitionTestNamedPaths.ResolveExistingDir(rootDir, name)
}
func (h stubDefinitionHost) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return prepareFactoryLayoutForDefinitionTest(
		context.Background(),
		segment,
		payload,
		factoryvalidation.New(nil, testCanonicalFactoryLoader),
	)
}
func (h stubDefinitionHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return persistPreparedNamedFactoryForTest(rootDir, name, prepared)
}
func (h stubDefinitionHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	return definitionTestNamedPaths.WriteCurrentPointer(rootDir, name)
}
func (h stubDefinitionHost) PreparePortableFactoryConfig(
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
func (h stubDefinitionHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	return factorysnapshotcapture.NewExplicit(factorysnapshotcapture.NewLoaded(factorysnapshot.ObjectFromFactoryConfig))(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}
func (h stubDefinitionHost) CurrentRuntimeConfig() loadedFactorySource {
	return h.currentRuntime
}
func (h stubDefinitionHost) WorkflowID() string { return h.workflowID }
func (h stubDefinitionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	if h.requireSessionErr != nil {
		return nil, h.requireSessionErr
	}
	return h.session, nil
}
func (h stubDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	if h.sessionRuntimeErr != nil {
		return nil, h.sessionRuntimeErr
	}
	return h.sessionRuntime, nil
}
func (h stubDefinitionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return h.sessionPersistRoot
}
func (h stubDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return validateDefinitionSnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (h stubDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (h stubDefinitionHost) ReplaceFactoryLayoutAtDir(
	string,
	*factorydefinitions.PreparedFactoryLayoutPayload,
) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func mustFactorySnapshot(factory factoryapi.Factory) *factorydefinitions.FactorySnapshot {
	snapshot, err := factorydefinitions.NewFactorySnapshot(factory)
	if err != nil {
		panic(err)
	}
	return snapshot
}

func namedFactoryPayload(t *testing.T, project string) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"name": project,
		"id":   project,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "worker-a",
			"type":          "MODEL_WORKER",
			"modelProvider": "CODEX",
			"model":         "gpt-5-codex",
			"body":          "You are worker " + project + ".",
		}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			"type":      "MODEL_WORKSTATION",
			"body":      "Do the " + project + " work.",
		}},
	})
	if err != nil {
		t.Fatalf("marshal named factory payload: %v", err)
	}
	return payload
}
