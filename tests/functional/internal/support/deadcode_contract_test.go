package support

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestAcceptedCommandResults_ReturnsRequestedCompleteResponses(t *testing.T) {
	results := AcceptedCommandResults(3)

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, result := range results {
		if got := string(result.Stdout); got != "Done. COMPLETE" {
			t.Fatalf("results[%d].Stdout = %q, want %q", i, got, "Done. COMPLETE")
		}
	}
}

func TestProviderCommandRequestsForWorker_FiltersRecordedRequests(t *testing.T) {
	runner := testutil.NewProviderCommandRunner()
	requests := []workers.CommandRequest{
		{WorkerType: "planner"},
		{WorkerType: "executor"},
		{WorkerType: "planner"},
	}
	for _, request := range requests {
		if _, err := runner.Run(context.Background(), request); err != nil {
			t.Fatalf("runner.Run(%#v): %v", request, err)
		}
	}

	filtered := ProviderCommandRequestsForWorker(runner, "planner")

	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	for i, request := range filtered {
		if request.WorkerType != "planner" {
			t.Fatalf("filtered[%d].WorkerType = %q, want %q", i, request.WorkerType, "planner")
		}
	}
}

func TestCountFactoryEvents_CountsMatchingEventTypes(t *testing.T) {
	events := []factoryapi.FactoryEvent{
		{Type: factoryapi.FactoryEventTypeDispatchRequest},
		{Type: factoryapi.FactoryEventTypeDispatchResponse},
		{Type: factoryapi.FactoryEventTypeDispatchRequest},
	}

	if got := CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest); got != 2 {
		t.Fatalf("CountFactoryEvents(dispatch request) = %d, want 2", got)
	}
	if got := CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse); got != 1 {
		t.Fatalf("CountFactoryEvents(dispatch response) = %d, want 1", got)
	}
}

func TestFactoryRelationsValue_ReturnsNilAndPopulatedRelations(t *testing.T) {
	var nilRelations *[]factoryapi.Relation
	if got := FactoryRelationsValue(nilRelations); got != nil {
		t.Fatalf("FactoryRelationsValue(nil) = %#v, want nil", got)
	}

	relations := []factoryapi.Relation{{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "generated-beta",
		TargetWorkName: "generated-alpha",
	}}

	got := FactoryRelationsValue(&relations)
	if len(got) != 1 {
		t.Fatalf("FactoryRelationsValue(...) length = %d, want 1", len(got))
	}
	if got[0] != relations[0] {
		t.Fatalf("FactoryRelationsValue(...) = %#v, want %#v", got[0], relations[0])
	}
}

func TestUpdateFactoryConfig_RewritesScaffoldedFactoryConfig(t *testing.T) {
	dir := ScaffoldFactory(t, map[string]any{
		"name": "original-factory",
		"workstations": []map[string]any{{
			"name": "writer",
		}},
	})

	UpdateFactoryConfig(t, dir, func(cfg map[string]any) {
		cfg["name"] = "updated-factory"
		cfg["labels"] = map[string]any{
			"team": "runtime",
		}
	})

	data, err := os.ReadFile(filepath.Join(dir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read updated factory config: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal updated factory config: %v", err)
	}

	if got := cfg["name"]; got != "updated-factory" {
		t.Fatalf("factory name = %#v, want %q", got, "updated-factory")
	}
	labels, ok := cfg["labels"].(map[string]any)
	if !ok {
		t.Fatalf("factory labels type = %T, want map[string]any", cfg["labels"])
	}
	if got := labels["team"]; got != "runtime" {
		t.Fatalf("factory labels.team = %#v, want %q", got, "runtime")
	}
}

func TestNewStaticSuccessCommandRunner_ReturnsFixedStdoutWithoutFailureFields(t *testing.T) {
	runner := NewStaticSuccessCommandRunner("script-output-ok")

	result, err := runner.Run(context.Background(), workers.CommandRequest{Command: "script-tool"})
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	if got := string(result.Stdout); got != "script-output-ok" {
		t.Fatalf("result.Stdout = %q, want %q", got, "script-output-ok")
	}
	if got := string(result.Stderr); got != "" {
		t.Fatalf("result.Stderr = %q, want empty", got)
	}
	if result.ExitCode != 0 {
		t.Fatalf("result.ExitCode = %d, want 0", result.ExitCode)
	}
}
