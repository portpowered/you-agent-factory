package restart_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Generations share the customer's local board and therefore run in order.
// The integration lane supplies the CLI; this test never builds it.
func TestPlainBoardGracefulRestartPreservesFailureAfterToComplete(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "I-RESTART-FAILURE", t.Name())
	evidence.StoryID = "plain-relaunch-restores-what-resume-restores-001"
	t.Cleanup(func() { evidence.publishRestartScenario(t) })
	repo, home := t.TempDir(), t.TempDir()
	config := boardPersistenceFactoryConfig()
	station := config["workstations"].([]map[string]any)[0]
	station["inputs"] = []map[string]string{{"workType": "task", "state": "init"}}
	station["outputs"] = []map[string]string{{"workType": "task", "state": "to-complete"}}
	workType := config["workTypes"].([]map[string]any)[0]
	workType["states"] = append(workType["states"].([]map[string]string), map[string]string{"name": "to-complete", "type": "PROCESSING"})
	factory := filepath.Join(repo, "factory")
	if err := os.Rename(scaffoldBoardPersistenceFactory(t, config), factory); err != nil {
		t.Fatal(err)
	}
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", boardPersistenceWorkerConfig(currentRestartWorkerExecutable(t)))
	release := filepath.Join(repo, "release")
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	first := startBoardPersistenceDaemon(t, binary, factory, home, "", release)
	evidence.trackDaemon(t, "before-failure-shutdown", first)
	batch := `{"requestId":"failed-history","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"failed-after-plan","name":"failed-after-plan","workTypeName":"task","state":"init","payload":"synthetic plan"}]}`
	submitBoardPersistenceBatchThroughCLI(t, first, binary, factory, home, batch, "failed-history", 1)
	waitForBoardStates(t, first.baseURL, map[string]string{"failed-after-plan": "to-complete"}, time.Minute)
	if _, err := runBoardPersistenceCLIWithFreshContext(t, binary, factory, home, first.baseURL, "--json", "work", "move", "failed-after-plan", "failed"); err != nil {
		t.Fatalf("move completed plan to failed: %v", err)
	}
	want := map[string]string{"failed-after-plan": "failed"}
	before := waitPlainBoardConfirmed(t, first.baseURL, want)
	if len(before.Results) != 1 || before.Results[0].State == nil || string(before.Results[0].State.Type) != "FAILED" {
		t.Fatal("source board did not expose exactly one failed/FAILED Work")
	}
	ids := map[string]struct{}{"failed-after-plan": {}}
	history := restartWorkHistoryFingerprint(t, first.baseURL, ids)
	evidence.capturePublicObservation(t, "failed-source", first.baseURL)
	shutdownPlainBoard(t, first)
	source := plainRestartSelectedRecording(t, repo)
	copyPath := filepath.Join(repo, "same-source.json")
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyPath, contents, 0600); err != nil {
		t.Fatal(err)
	}
	for _, resumePath := range []string{"", copyPath} {
		daemon := startBoardPersistenceResumeDaemon(t, binary, factory, home, resumePath, "", release)
		evidence.trackDaemon(t, "restored-failed-board", daemon)
		after := waitForBoardStates(t, daemon.baseURL, want, time.Minute)
		if len(after.Results) != 1 || boardPersistenceStringPointerValue(after.Results[0].WorkId) != "failed-after-plan" {
			t.Fatal("restore changed Work IDs or count")
		}
		assertPlainBoardPreserved(t, before, after)
		if got := restartWorkHistoryFingerprint(t, daemon.baseURL, ids); !reflect.DeepEqual(history, got) {
			t.Fatal("restore changed canonical Work admission/state history")
		}
		evidence.capturePublicObservation(t, "restored-failure", daemon.baseURL)
		shutdownPlainBoard(t, daemon)
	}
}

func plainRestartSelectedRecording(t *testing.T, repo string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repo, ".you-agent-factory", "current-board.json"))
	if err != nil {
		t.Fatal(err)
	}
	var selected struct{ ArtifactReference string }
	if err := json.Unmarshal(contents, &selected); err != nil || selected.ArtifactReference == "" {
		t.Fatal("shutdown did not publish its recording reference")
	}
	return selected.ArtifactReference
}
