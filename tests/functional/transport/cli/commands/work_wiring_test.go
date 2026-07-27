package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	workWiringListShowRequestID = "cli-work-wiring-list-show"
	workWiringListShowWorkName  = "list-show-task"
	workWiringListShowWorkType  = "task"

	workWiringMoveRequestID = "cli-work-wiring-move"
	workWiringMoveWorkName  = "move-recovery-task"
	workWiringMoveWorkType  = "task"
)

// TestCLIWorkListAndShowReflectSubmittedWork proves you work list and you work show
// reflect work submitted through the public CLI against a running Factory Session.
func TestCLIWorkListAndShowReflectSubmittedWork(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI work list/show wiring")
	}

	factoryDir := support.ScaffoldFactory(t, workWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Work list/show wiring"}}]}`,
		workWiringListShowRequestID,
		workWiringListShowWorkName,
		workWiringListShowWorkType,
	)
	submitOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"--json",
		"submit", "batch",
		inlineBatch,
	)
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}

	var submitted workWiringBatchSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(submitOut), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, submitOut)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing accepted work identity: %#v", submitted)
	}
	workID := submitted.Works[0].WorkID

	listOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"work", "list",
		"--name", workWiringListShowWorkName,
	)
	if err != nil {
		t.Fatalf("you work list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{
		workID,
		workWiringListShowWorkName,
		workWiringListShowWorkType,
	} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("work list output missing %q:\n%s", marker, listHuman)
		}
	}

	listed := runWorkListCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, workWiringListShowWorkName)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringListShowWorkType, "init")) &&
		!support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringListShowWorkType, "complete")) {
		t.Fatalf("work list JSON missing submitted work %q at init or complete: %#v", workID, listed.Results)
	}

	shown, err := runWorkShowCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, workID)
	if err != nil {
		t.Fatalf("you work show %s: %v", workID, err)
	}
	if shown.WorkId == nil || strings.TrimSpace(*shown.WorkId) != workID {
		t.Fatalf("work show id = %#v, want %q", shown.WorkId, workID)
	}
	if shown.Name != workWiringListShowWorkName {
		t.Fatalf("work show name = %q, want %q", shown.Name, workWiringListShowWorkName)
	}
	if shown.State == nil || strings.TrimSpace(shown.State.Name) == "" {
		t.Fatalf("work show missing customer-visible state: %#v", shown)
	}

	showOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"work", "show", workID,
	)
	if err != nil {
		t.Fatalf("you work show: %v\noutput:\n%s", err, showOut)
	}
	showHuman := string(showOut)
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Name:\t" + workWiringListShowWorkName,
		"State name:\t" + shown.State.Name,
	} {
		if !strings.Contains(showHuman, marker) {
			t.Fatalf("work show output missing %q:\n%s", marker, showHuman)
		}
	}
}

// TestCLIWorkMoveChangesState proves you work move changes work state through the
// public CLI so operators can complete a manual recovery step and observe the result.
func TestCLIWorkMoveChangesState(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI work move wiring")
	}

	factoryDir := support.ScaffoldFactory(t, workWiringMoveFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Work move wiring"}}]}`,
		workWiringMoveRequestID,
		workWiringMoveWorkName,
		workWiringMoveWorkType,
	)
	submitOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"--json",
		"submit", "batch",
		inlineBatch,
	)
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}

	var submitted workWiringBatchSubmitJSON
	if err := json.Unmarshal(bytesTrimSpace(submitOut), &submitted); err != nil {
		t.Fatalf("decode submit batch JSON: %v\noutput:\n%s", err, submitOut)
	}
	if submitted.WorkCount != 1 || len(submitted.Works) != 1 || strings.TrimSpace(submitted.Works[0].WorkID) == "" {
		t.Fatalf("submit batch response missing accepted work identity: %#v", submitted)
	}
	workID := submitted.Works[0].WorkID

	waitForWorkStateViaCLI(t, ctx, binaryPath, factoryDir, baseURL, workID, "init", 15*time.Second)

	moveOut, err := runYouCLI(ctx, binaryPath, factoryDir, baseURL,
		"work", "move", workID, "complete",
	)
	if err != nil {
		t.Fatalf("you work move: %v\noutput:\n%s", err, moveOut)
	}
	assertWorkMoveWiringHumanOutput(t, string(moveOut), workID, "init", "complete")

	shown, err := runWorkShowCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, workID)
	if err != nil {
		t.Fatalf("you work show %s after move: %v", workID, err)
	}
	if shown.State == nil || shown.State.Name != "complete" {
		t.Fatalf("work show state after move = %#v, want complete", shown.State)
	}

	listed := runWorkListCLIJSON(t, ctx, binaryPath, factoryDir, baseURL, workWiringMoveWorkName)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringMoveWorkType, "complete")) {
		t.Fatalf("work list JSON missing moved work %q at complete: %#v", workID, listed.Results)
	}
}

func assertWorkMoveWiringHumanOutput(t *testing.T, output, workID, previousState, newState string) {
	t.Helper()
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Previous state:\t" + previousState,
		"New state:\t" + newState,
		"Session ID:\t~default",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("work move output missing %q:\n%s", marker, output)
		}
	}
}

type workWiringBatchSubmitJSON struct {
	WorkCount int `json:"workCount"`
	Works     []struct {
		Name   string `json:"name"`
		WorkID string `json:"workId"`
	} `json:"works"`
}

func workWiringMoveFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-work-wiring-move",
		"workTypes": []map[string]any{
			{
				"name": workWiringMoveWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
	}
}

func workWiringFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-work-wiring",
		"workTypes": []map[string]any{
			{
				"name": workWiringListShowWorkType,
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-task",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": workWiringListShowWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": workWiringListShowWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": workWiringListShowWorkType, "state": "failed"}},
			},
		},
	}
}
