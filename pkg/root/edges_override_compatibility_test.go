package root

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestProcessCapabilityRootsReifyOpaqueValuesAtTheRootBoundary(t *testing.T) {
	t.Parallel()

	if got := RecordingsProjectionFromProcess(nil); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(nil) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(nil); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(nil) = %#v, want nil", got)
	}

	withoutCapabilities, err := initializerapplication.NewProcess(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewProcess() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(withoutCapabilities); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(without capability) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(withoutCapabilities); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(without capability) = %#v, want nil", got)
	}

	wrongType, err := initializerapplication.NewProcessWithRuntimeCostsAndExecutionAndCapabilities(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		rootRecordingsProjectionCapabilityProbe{value: struct{}{}},
		rootOperatorSettingsCapabilityProbe{value: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecutionAndCapabilities() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(wrongType); got != nil {
		t.Fatalf("RecordingsProjectionFromProcess(wrong type) = %#v, want nil", got)
	}
	if got := OperatorSettingsFromProcess(wrongType); got != nil {
		t.Fatalf("OperatorSettingsFromProcess(wrong type) = %#v, want nil", got)
	}

	typedProjection := &rootRecordingsProjectionProbe{}
	typedProcess, err := initializerapplication.NewProcessWithRuntimeCostsAndExecutionAndCapabilities(
		nil,
		nil,
		rootWorkerProcessRegistry{},
		rootWorkerProcessLifecycle{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		rootRecordingsProjectionCapabilityProbe{value: typedProjection},
		rootOperatorSettingsCapabilityProbe{value: struct{}{}},
	)
	if err != nil {
		t.Fatalf("NewProcessWithRuntimeCostsAndExecutionAndCapabilities(typed) error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(typedProcess); got != typedProjection {
		t.Fatalf("RecordingsProjectionFromProcess(typed) = %#v, want %#v", got, typedProjection)
	}

	composed, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if got := RecordingsProjectionFromProcess(composed); got == nil {
		t.Fatal("RecordingsProjectionFromProcess(composed) = nil, want composed projection")
	}
	if got := OperatorSettingsFromProcess(composed); got == nil {
		t.Fatal("OperatorSettingsFromProcess(composed) = nil, want composed service")
	}
	if got := RecordingsServiceFromProcess(composed); got == nil {
		t.Fatal("RecordingsServiceFromProcess(composed) = nil, want composed service")
	}
}

type rootRecordingsProjectionCapabilityProbe struct {
	value any
}

func (probe rootRecordingsProjectionCapabilityProbe) RecordingsProjection() any {
	return probe.value
}

type rootOperatorSettingsCapabilityProbe struct {
	value any
}

func (probe rootOperatorSettingsCapabilityProbe) OperatorSettings() any {
	return probe.value
}

type rootRecordingsProjectionProbe struct{}

func (rootRecordingsProjectionProbe) ReconstructWorldState(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
	return recordings.ReconstructWorldStateResult{}, nil
}

func (rootRecordingsProjectionProbe) QueryWorkstationRequests(recordings.WorkstationRequestsQueryRequest) (recordings.WorkstationRequestsQueryResult, error) {
	return recordings.WorkstationRequestsQueryResult{}, nil
}

func (rootRecordingsProjectionProbe) ReconstructFactoryWorldState([]recordings.FactoryEvent, int) (recordings.FactoryWorldState, error) {
	return recordings.FactoryWorldState{}, nil
}

func (rootRecordingsProjectionProbe) ProjectWorkstationRequests(recordings.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (rootRecordingsProjectionProbe) SimpleDashboardRenderData(recordings.FactoryWorldState) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (rootRecordingsProjectionProbe) ProjectActiveThrottlePauses(
	recordings.InitialStructurePayload,
	[]recordings.ActiveThrottlePause,
) []recordings.FactoryWorldThrottlePause {
	return nil
}

func (rootRecordingsProjectionProbe) ValidateReconnectReplay(
	[]recordings.FactoryEvent,
	recordings.FactoryEventReconnectCursor,
	recordings.FactoryEventReconnectScope,
) error {
	return nil
}

func TestBuildProcessStartsFactoryWithFutureDefinitionFields(t *testing.T) {
	t.Parallel()

	factoryDir := rootFactoryWithProvider(t, "codex")
	factoryPath := filepath.Join(factoryDir, "factory.json")
	raw, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode factory.json: %v", err)
	}
	document["logicalRoundTrip"] = map[string]any{
		"mode":   "v2",
		"secret": "must-not-leak",
	}
	workers, ok := document["workers"].([]any)
	if !ok || len(workers) == 0 {
		t.Fatal("factory.json has no worker to extend")
	}
	workers[0].(map[string]any)["futurePolicy"] = map[string]any{"mode": "v2"}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode future factory.json: %v", err)
	}
	if err := os.WriteFile(factoryPath, encoded, 0o600); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}

	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if err := process.Execute(Input{
		Args:             []string{"you", "run", "--dir", factoryDir, "--with-mock-workers", "--quiet", "--no-record"},
		Env:              homeEnvironment(t.TempDir()),
		Context:          context.Background(),
		WorkingDirectory: factoryDir,
	}); err != nil {
		t.Fatalf("Process.Execute(run) error = %v", err)
	}
}

func TestBuildProcessAppliesTypedEdgesExternalEffectOverride(t *testing.T) {
	t.Parallel()

	const generated = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		OperatorSettingsIDGenerator: func() string { return generated },
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	configPath := runNormalInitialization(t, process, home)
	persisted := readBackendScopeID(t, configPath)
	want := operatorsettings.LocalBackendScopePrefix + generated
	if persisted != want {
		t.Fatalf("backendScopeID = %q, want edges override %q", persisted, want)
	}
}

func TestBuildProcessEmptyEdgesSelectProductionExternalEffectDefaults(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	process, err := BuildProcess(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	configPath := runNormalInitialization(t, process, home)
	persisted := readBackendScopeID(t, configPath)
	if !operatorsettings.IsLocalBackendScopeID(persisted) {
		t.Fatalf("backendScopeID = %q, want production local-<uuid> default", persisted)
	}
}

func TestBuildProcessKeepsInitializationEdgeReplacementCompatible(t *testing.T) {
	t.Parallel()

	var initializationCalls atomic.Int32
	initializationErr := errors.New("system initialization override selected")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			initializationCalls.Add(1)
			return nil, initializationErr
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	home := t.TempDir()
	err = process.Execute(Input{
		Args:             []string{"you", "run", "--factory", filepath.Join(home, "missing.json")},
		Env:              homeEnvironment(home),
		Context:          context.Background(),
		WorkingDirectory: home,
	})
	if err == nil || !strings.Contains(err.Error(), initializationErr.Error()) {
		t.Fatalf("Process.Execute(run) error = %v, want %v", err, initializationErr)
	}
	if initializationCalls.Load() == 0 {
		t.Fatal("SystemInitializationInspectPath override was not used")
	}
}

// TestBuildProcessFactoryListSkipsInitializationEdges proves effective catalog
// discovery remains read-only even when the initializer edge would fail.
func TestBuildProcessFactoryListSkipsInitializationEdges(t *testing.T) {
	t.Parallel()

	var initializationCalls atomic.Int32
	apiStarts := 0
	initializationErr := errors.New("system initialization override selected")
	process, err := BuildProcess(context.Background(), serviceedges.Edges{
		SystemInitializationInspectPath: func(string) (fs.FileInfo, error) {
			initializationCalls.Add(1)
			return nil, initializationErr
		},
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			apiStarts++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if apiStarts != 0 || initializationCalls.Load() != 0 {
		t.Fatalf(
			"construction side effects = api:%d initialization:%d, want zero",
			apiStarts,
			initializationCalls.Load(),
		)
	}

	home := t.TempDir()
	var stderr bytes.Buffer
	err = process.Execute(Input{
		Args: []string{
			"you", "--json", "factory", "list", "--dir",
			filepath.Join(home, ".you-agent-factory", "factories"),
		},
		Env:              homeEnvironment(home),
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: home,
	})
	if err != nil {
		t.Fatalf("Process.Execute(factory list) error = %v stderr=%q", err, stderr.String())
	}
	if initializationCalls.Load() != 0 {
		t.Fatalf("SystemInitializationInspectPath calls = %d, want 0", initializationCalls.Load())
	}
	if apiStarts != 0 {
		t.Fatalf("APIServerStarter calls = %d during factory list, want 0", apiStarts)
	}
}

func runNormalInitialization(t *testing.T, process *initializerapplication.Process, home string) string {
	t.Helper()

	missingFactory := filepath.Join(home, "missing.json")
	err := process.Execute(Input{
		Args:             []string{"you", "run", "--factory", missingFactory},
		Env:              homeEnvironment(home),
		Context:          context.Background(),
		WorkingDirectory: home,
	})
	if err == nil || !strings.Contains(err.Error(), "missing.json") {
		t.Fatalf("Process.Execute(run missing Factory) error = %v", err)
	}
	return filepath.Join(home, ".you-agent-factory", "config.json")
}

func readBackendScopeID(t *testing.T, configPath string) string {
	t.Helper()

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	var document struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("decode operator config: %v\ncontent:\n%s", err, raw)
	}
	return document.BackendScopeID
}
