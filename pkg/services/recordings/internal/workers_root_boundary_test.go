package internal

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// TestNewReplayExecutionConstructsWorkersRootPorts proves Recordings replay
// execution construction returns Workers root Provider and CommandRunner ports
// without nested Workers package imports in this boundary test.
func TestNewReplayExecutionConstructsWorkersRootPorts(t *testing.T) {
	t.Parallel()

	fixturePath := filepath.Join("replay", "testdata", "inference-events.replay.json")
	artifact, err := replayimpl.Load(
		platformreplay.NewLocal(runtime.GOOS),
		filepath.FromSlash(fixturePath),
		workersRootBoundaryFactorySnapshotDecoder,
	)
	if err != nil {
		t.Fatalf("Load inference fixture: %v", err)
	}

	provider, runner, hooks, planner, err := NewReplayExecution(
		artifact,
		workersRootBoundaryFactorySnapshotDecoder,
		workersRootBoundaryRuntimeConfigDecoder,
	)
	if err != nil {
		t.Fatalf("NewReplayExecution: %v", err)
	}
	if provider == nil || runner == nil {
		t.Fatalf("ports = (%v,%v), want non-nil provider and runner", provider, runner)
	}
	if hooks == nil || planner == nil {
		t.Fatalf("hooks/planner = (%v,%v), want non-nil replay helpers", hooks, planner)
	}

	var rootProvider workers.Provider = provider
	var rootRunner workers.CommandRunner = runner
	if rootProvider == nil || rootRunner == nil {
		t.Fatal("NewReplayExecution ports must satisfy workers root contracts")
	}
}

func workersRootBoundaryFactorySnapshotDecoder(data []byte) (*factorydefinitions.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(generated)
}

func workersRootBoundaryRuntimeConfigDecoder(
	snapshot *factorydefinitions.FactorySnapshot,
) (factorydefinitions.ReplayRuntimeConfig, error) {
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(generated)
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		return nil, err
	}
	factoryDir := ""
	if generated.FactoryDirectory != nil {
		factoryDir = *generated.FactoryDirectory
	}
	return runtimefixtures.ReplayRuntimeConfigValue(config, factoryDir), nil
}
