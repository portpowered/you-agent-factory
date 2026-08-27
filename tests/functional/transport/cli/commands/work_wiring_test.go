package commands_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	workWiringListShowRequestID = "cli-work-wiring-list-show"
	workWiringListShowWorkName  = "list-show-task"
	workWiringListShowWorkType  = "task"

	workWiringMoveRequestID = "cli-work-wiring-move"
	workWiringMoveWorkName  = "move-recovery-task"
	workWiringMoveWorkType  = "task"

	workWiringMissingWorkID = "work-missing-999"

	workWiringVisualizeRequestID = "cli-work-wiring-visualize"
)

// TestCLIWorkListAndShowReflectSubmittedWork proves you work list and you work show
// reflect work submitted through the public CLI against a running Factory Session.
func testCLIWorkListAndShowReflectSubmittedWork(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := support.ScaffoldFactory(t, workWiringFactoryConfig())
	sessionID := remote.openSession(t, factoryDir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Work list/show wiring"}}]}`,
		workWiringListShowRequestID,
		workWiringListShowWorkName,
		workWiringListShowWorkType,
	)
	submitOut, err := remote.run(ctx, factoryDir, sessionID,
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

	listOut, err := remote.run(ctx, factoryDir, sessionID,
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

	listed := runWorkListCLIJSON(t, ctx, remote.process, factoryDir, remote.baseURL, sessionID, workWiringListShowWorkName)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringListShowWorkType, "init")) &&
		!support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringListShowWorkType, "complete")) {
		t.Fatalf("work list JSON missing submitted work %q at init or complete: %#v", workID, listed.Results)
	}

	shown, err := runWorkShowCLIJSON(t, ctx, remote.process, factoryDir, remote.baseURL, sessionID, workID)
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

	showOut, err := remote.run(ctx, factoryDir, sessionID,
		"work", "show", workID,
	)
	if err != nil {
		t.Fatalf("you work show: %v\noutput:\n%s", err, showOut)
	}
	showHuman := string(showOut)
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Name:\t" + workWiringListShowWorkName,
	} {
		if !strings.Contains(showHuman, marker) {
			t.Fatalf("work show output missing %q:\n%s", marker, showHuman)
		}
	}
	stateMarkerFound := false
	for _, state := range []string{"init", "complete", "failed"} {
		if strings.Contains(showHuman, "State name:\t"+state) {
			stateMarkerFound = true
			break
		}
	}
	if !stateMarkerFound {
		t.Fatalf("work show output missing a known customer-visible state:\n%s", showHuman)
	}
}

// TestCLIWorkMoveChangesState proves you work move changes work state through the
// public CLI so operators can complete a manual recovery step and observe the result.
func testCLIWorkMoveChangesState(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := support.ScaffoldFactory(t, workWiringMoveFactoryConfig())
	sessionID := remote.openSession(t, factoryDir)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Work move wiring"}}]}`,
		workWiringMoveRequestID,
		workWiringMoveWorkName,
		workWiringMoveWorkType,
	)
	submitOut, err := remote.run(ctx, factoryDir, sessionID,
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

	waitForWorkStateViaCLI(t, ctx, remote.process, factoryDir, remote.baseURL, sessionID, workID, "init", 15*time.Second)

	moveOut, err := remote.run(ctx, factoryDir, sessionID,
		"work", "move", workID, "complete",
	)
	if err != nil {
		t.Fatalf("you work move: %v\noutput:\n%s", err, moveOut)
	}
	assertWorkMoveWiringHumanOutput(t, string(moveOut), workID, "init", "complete", sessionID)

	shown, err := runWorkShowCLIJSON(t, ctx, remote.process, factoryDir, remote.baseURL, sessionID, workID)
	if err != nil {
		t.Fatalf("you work show %s after move: %v", workID, err)
	}
	if shown.State == nil || shown.State.Name != "complete" {
		t.Fatalf("work show state after move = %#v, want complete", shown.State)
	}

	listed := runWorkListCLIJSON(t, ctx, remote.process, factoryDir, remote.baseURL, sessionID, workWiringMoveWorkName)
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation(workWiringMoveWorkType, "complete")) {
		t.Fatalf("work list JSON missing moved work %q at complete: %#v", workID, listed.Results)
	}

}

// TestCLIWorkShowMissingReturnsNotFound proves you work show for a missing work id
// exits non-success with actionable not-found diagnostics and no false success payload.
func testCLIWorkShowMissingReturnsNotFound(t *testing.T, remote *sharedRemoteCLI) {
	factoryDir := support.ScaffoldFactory(t, workWiringFactoryConfig())
	sessionID := remote.openSession(t, factoryDir)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	showOut, err := remote.run(ctx, factoryDir, sessionID,
		"work", "show", workWiringMissingWorkID,
	)
	assertCLIWorkShowNotFoundFailure(t, showOut, err, workWiringMissingWorkID)

	showJSONOut, err := remote.run(ctx, factoryDir, sessionID,
		"--json",
		"work", "show", workWiringMissingWorkID,
	)
	assertCLIWorkShowNotFoundFailure(t, showJSONOut, err, workWiringMissingWorkID)
}

// TestCLIWorkRenderProducesDeterministicGraph proves you work render emits
// the same dependency graph for a fixed batch input across repeated CLI invocations.
func TestCLIWorkRenderProducesDeterministicGraph(t *testing.T) {
	workingDir := t.TempDir()
	processHarness := newLocalReusableProcessHarness(t)
	batchPath := writeWorkWiringVisualizeBatchFile(t, workingDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	firstOut, err := runYouCLI(ctx, processHarness, workingDir, "",
		"work", "render", batchPath,
	)
	if err != nil {
		t.Fatalf("you work render (first): %v\noutput:\n%s", err, firstOut)
	}

	secondOut, err := runYouCLI(ctx, processHarness, workingDir, "",
		"work", "render", batchPath,
	)
	if err != nil {
		t.Fatalf("you work render (second): %v\noutput:\n%s", err, secondOut)
	}

	first := string(firstOut)
	second := string(secondOut)
	if first != second {
		t.Fatalf("visualize output not deterministic:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	assertWorkWiringVisualizeGraphOutput(t, first)
}

func assertCLIWorkShowNotFoundFailure(
	t *testing.T,
	output []byte,
	err error,
	workID string,
) {
	t.Helper()

	if err == nil {
		t.Fatalf("you work show unexpectedly succeeded:\n%s", output)
	}

	text := string(output)
	support.RequireNotFoundCLIDiagnostic(t, text)
	if strings.Contains(text, workID) {
		t.Fatalf("work show leaked work id %q in safe diagnostic:\n%s", workID, text)
	}

	var shown factoryapi.Work
	if json.Unmarshal(bytesTrimSpace(output), &shown) == nil &&
		(shown.WorkId != nil && strings.TrimSpace(*shown.WorkId) != "" || strings.TrimSpace(shown.Name) != "") {
		t.Fatalf("work show must not emit a success work payload:\n%s", text)
	}
	if strings.Contains(text, "Work ID:\t") && strings.Contains(text, "State name:\t") {
		t.Fatalf("work show must not emit human success work payload:\n%s", text)
	}
}

func writeWorkWiringVisualizeBatchFile(t *testing.T, dir string) string {
	t.Helper()

	batchPath := filepath.Join(dir, "work-wiring-visualize-batch.json")
	content := fmt.Sprintf(`{
  "requestId": %q,
  "type": "FACTORY_REQUEST_BATCH",
  "works": [
    {"name": "alpha", "workTypeName": "task"},
    {"name": "beta", "workTypeName": "task"},
    {"name": "gamma", "workTypeName": "task"}
  ],
  "relations": [
    {"type": "DEPENDS_ON", "sourceWorkName": "beta", "targetWorkName": "alpha"},
    {"type": "DEPENDS_ON", "sourceWorkName": "gamma", "targetWorkName": "beta"}
  ]
}`, workWiringVisualizeRequestID)
	if err := os.WriteFile(batchPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write visualize batch file: %v", err)
	}
	return batchPath
}

func assertWorkWiringVisualizeGraphOutput(t *testing.T, output string) {
	t.Helper()

	if !strings.HasPrefix(output, "flowchart TD\n") {
		t.Fatalf("visualize output missing flowchart header:\n%s", output)
	}
	for _, want := range []string{
		`alpha["alpha"]`,
		`beta["beta"]`,
		`gamma["gamma"]`,
		"beta --> alpha",
		"gamma --> beta",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("visualize output missing %q:\n%s", want, output)
		}
	}
}

func assertWorkMoveWiringHumanOutput(t *testing.T, output, workID, previousState, newState, sessionID string) {
	t.Helper()
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Previous state:\t" + previousState,
		"New state:\t" + newState,
		"Session ID:\t" + sessionID,
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
