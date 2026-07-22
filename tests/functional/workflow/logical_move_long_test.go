//go:build functionallong

package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestLogicalMove_Success(t *testing.T) {
	support.SkipLongFunctional(t, "slow logical-move success sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_dir"))
	configureLogicalMoveWorkstation(t, dir, "router")
	testutil.WriteSeedFile(t, dir, "task", []byte("my-payload"))
	session := support.RunFactoryToCompletion(t, dir, testutil.NewMockProvider(), 5*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"task:done": 1, "task:init": 0})
}

func TestLogicalMove_PreservesTokenColor(t *testing.T) {
	support.SkipLongFunctional(t, "slow logical-move color sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "logical_move_pipeline_dir"))
	configureLogicalMoveWorkstation(t, dir, "router")
	testutil.WriteSeedFile(t, dir, "task", []byte("preserved-payload"))
	provider := testutil.NewMockProvider(workerexecution.InferenceResponse{Content: "done COMPLETE"})
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertWorkflowSessionPlaces(t, session, map[string]int{"task:done": 1, "task:init": 0, "task:staging": 0})
	calls := provider.Calls()
	if len(calls) != 1 || len(calls[0].Dispatch.InputTokens) == 0 ||
		string(firstInputToken(calls[0].Dispatch.InputTokens).Color.Payload) != "preserved-payload" {
		t.Fatalf("provider calls = %#v, want preserved payload after logical move", calls)
	}
}

func configureLogicalMoveWorkstation(t *testing.T, dir, workstationName string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("decode factory config: %v", err)
	}
	for _, raw := range config["workstations"].([]any) {
		workstation := raw.(map[string]any)
		if workstation["name"] == workstationName {
			workstation["type"] = "LOGICAL_MOVE"
			workstation["worker"] = ""
		}
	}
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("encode factory config: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory config: %v", err)
	}
	workstationConfigPath := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.WriteFile(workstationConfigPath, []byte("---\ntype: LOGICAL_MOVE\n---\n"), 0o644); err != nil {
		t.Fatalf("write logical workstation config: %v", err)
	}
}
