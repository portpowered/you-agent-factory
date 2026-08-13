package runtimeopening

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestActivationRequestCarriesExplicitRuntimeInputs(t *testing.T) {
	t.Parallel()

	skipPermissions := true
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
			Directory:        "/factory",
			SourcePath:       "/source",
			ExecutionBaseDir: "/runtime",
		},
		FactoryRuntime: factoryruntime.RuntimeOpeningRequest{Verbose: true},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			BackendScopeID: "scope",
			Host: factorysessions.RuntimeHostRequest{
				Host: "127.0.0.1",
				Port: 8080,
			},
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID:                          "runner",
			InvocationSkipPermissionsOverride: &skipPermissions,
		},
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.RuntimeID != "runtime-1" || activation.FactorySessionID != factorysessions.DefaultSessionID {
		t.Fatalf("activation identity = %#v, want runtime-1/%q", activation, factorysessions.DefaultSessionID)
	}
	if activation.Inputs.Definition.SourcePath != "/source" || activation.Inputs.Session.BackendScopeID != "scope" {
		t.Fatalf("activation inputs lost source or session values: %#v", activation.Inputs)
	}
	if activation.Inputs.Workers.InvocationSkipPermissionsOverride == nil || !*activation.Inputs.Workers.InvocationSkipPermissionsOverride {
		t.Fatal("activation inputs lost worker permission override")
	}
}

func TestActivationRequestDetachesMockWorkerInputs(t *testing.T) {
	t.Parallel()

	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Workers: workers.RuntimeOpeningRequest{
			MockWorkers: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					RunType: workers.MockWorkerRunTypeScript,
					ScriptConfig: &workers.MockWorkerScriptConfig{
						Command: "run",
						Env:     map[string]string{"TOKEN": "one"},
					},
				}},
			},
		},
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Env["TOKEN"] = "caller-mutated"
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Args = []string{"caller-mutated"}
	got := activation.Inputs.Workers.MockWorkers.MockWorkers[0]
	if got.ScriptConfig.Env["TOKEN"] != "one" || len(got.ScriptConfig.Args) != 0 {
		t.Fatalf("activation inputs retained caller mutation: %#v", got)
	}
}

func TestActivationRequestCarriesFactorySessionCorrelation(t *testing.T) {
	t.Parallel()

	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
	}
	activation, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			FactorySessionID: "session-1",
		},
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.FactorySessionID != "session-1" || activation.Snapshot.Invocation.FactorySessionID != "session-1" {
		t.Fatalf("activation session correlation = %#v / %#v, want session-1", activation.FactorySessionID, activation.Snapshot.Invocation)
	}
}

func TestActivationRequestDerivesDirectoryForSourceOnlySnapshot(t *testing.T) {
	t.Parallel()

	sourcePath := filepath.Join(t.TempDir(), factorydefinitions.FactoryConfigFile)
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions: activationDefinitionsStub{snapshot: factorydefinitions.RuntimeSnapshot{
			EffectiveFactory:  factorydefinitions.FactoryConfig{Name: "source-only"},
			DefinitionVersion: &factorydefinitions.FactoryVersion{Logical: 1},
		}},
	}
	activation, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{SourcePath: sourcePath},
	})
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.Snapshot.FactoryDir != filepath.Dir(sourcePath) {
		t.Fatalf("snapshot FactoryDir = %q, want %q", activation.Snapshot.FactoryDir, filepath.Dir(sourcePath))
	}
}

func TestActivationRequestReturnsTypedDefinitionsFailureBeforeRuntimeActivation(t *testing.T) {
	t.Parallel()

	want := &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
			Field:   "source",
			Message: "invalid Factory source",
		},
		Cause: factorydefinitions.ErrInvalidRuntimeSnapshotDefinition,
	}
	factory := &Factory{
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{err: want},
	}
	_, err := factory.activationRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
	})
	if !errors.Is(err, factorydefinitions.ErrInvalidRuntimeSnapshotDefinition) {
		t.Fatalf("activationRequest() error = %v, want typed Definitions failure", err)
	}
}

type activationDefinitionsStub struct {
	factorydefinitions.Service
	snapshot factorydefinitions.RuntimeSnapshot
	err      error
}

func (stub activationDefinitionsStub) ResolveRuntimeSnapshot(
	context.Context,
	factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	if stub.err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, stub.err
	}
	return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: stub.snapshot}, nil
}

func activationSnapshot() factorydefinitions.RuntimeSnapshot {
	return factorydefinitions.RuntimeSnapshot{
		FactoryDir:        "/factory",
		RuntimeBaseDir:    "/runtime",
		DefinitionVersion: &factorydefinitions.FactoryVersion{Logical: 1},
		EffectiveFactory:  factorydefinitions.FactoryConfig{Name: "snapshot"},
	}
}

func TestOpenForRequestRoutesLegacyReplayThroughRuntimeRoot(t *testing.T) {
	t.Parallel()

	root := &replayRoutingRoot{}
	replayInputs := &legacyReplayInputsStub{}
	factory := &Factory{
		runtimeRoot:               root,
		replayInputs:              replayInputs,
		generateRuntimeInstanceID: func() string { return "runtime-1" },
		factoryDefinitions:        activationDefinitionsStub{snapshot: activationSnapshot()},
		decodeReplayConfig: func(*factorydefinitions.FactorySnapshot) (factorydefinitions.ReplayRuntimeConfig, error) {
			return replayRuntimeConfigStub{}, nil
		},
	}
	_, err := factory.openForRequest(context.Background(), &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Recordings:        recordings.RuntimeOpeningRequest{ReplayPath: "legacy.json"},
	})
	if err != nil {
		t.Fatalf("openForRequest(legacy replay) error = %v", err)
	}
	if root.activations != 1 {
		t.Fatalf("Runtime root activations = %d, want one", root.activations)
	}
	if replayInputs.calls != 1 {
		t.Fatalf("replay input classifications = %d, want one", replayInputs.calls)
	}
}

type replayRoutingRoot struct {
	factoryruntime.Service
	activations int
}

func (root *replayRoutingRoot) Activate(
	context.Context,
	factoryruntime.RuntimeActivationRequest,
) (factoryruntime.RuntimeActivationResult, error) {
	root.activations++
	return factoryruntime.RuntimeActivationResult{
		RuntimeID: "runtime-1",
		Runtime: factoryruntime.RuntimeActivationView{
			RuntimeID: "runtime-1",
			Service:   &activatedRuntimeService{products: runtimeProducts{}},
		},
	}, nil
}

func (root *replayRoutingRoot) Deactivate(
	context.Context,
	factoryruntime.RuntimeDeactivationRequest,
) (factoryruntime.RuntimeDeactivationResult, error) {
	return factoryruntime.RuntimeDeactivationResult{}, nil
}

type legacyReplayInputsStub struct {
	calls int
}

type replayRuntimeConfigStub struct{}

func (replayRuntimeConfigStub) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{Name: "legacy"}
}
func (replayRuntimeConfigStub) FactoryDir() string     { return "/factory" }
func (replayRuntimeConfigStub) RuntimeBaseDir() string { return "/factory" }
func (replayRuntimeConfigStub) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}
func (replayRuntimeConfigStub) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (replayRuntimeConfigStub) WorkstationByID(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}

func (stub *legacyReplayInputsStub) LoadReplayInput(
	recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	stub.calls++
	snapshot := factorydefinitions.FactorySnapshot(`{"name":"legacy"}`)
	return recordings.LoadReplayInputResult{
		Legacy: &factorydefinitions.ReplayArtifact{Factory: &snapshot},
	}, nil
}
