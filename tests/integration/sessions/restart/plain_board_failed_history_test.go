package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
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
		if plainRestartSelectedRecording(t, repo) == source {
			t.Fatal("relaunch did not publish a successor recording")
		}
		assertPlainRestartFileUnchanged(t, source, contents)
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

// Removing a recorded state creates a structural conflict without private
// recordings or malformed JSON. Rejection must preserve the sole recovery input.
func TestPlainBoardRestoreFailurePreservesRecording(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
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
	batch := `{"requestId":"conflict-history","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"synthetic-conflict","name":"synthetic-conflict","workTypeName":"task","state":"init","payload":"private-synthetic-sentinel"}]}`
	submitBoardPersistenceBatchThroughCLI(t, first, binary, factory, home, batch, "conflict-history", 1)
	waitForBoardStates(t, first.baseURL, map[string]string{"synthetic-conflict": "to-complete"}, time.Minute)
	if _, err := runBoardPersistenceCLIWithFreshContext(t, binary, factory, home, first.baseURL, "--json", "work", "move", "synthetic-conflict", "failed"); err != nil {
		t.Fatal(err)
	}
	waitForBoardStates(t, first.baseURL, map[string]string{"synthetic-conflict": "failed"}, time.Minute)
	shutdownPlainBoard(t, first)
	source := plainRestartSelectedRecording(t, repo)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	refPath := filepath.Join(repo, ".you-agent-factory", "current-board.json")
	reference, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(factory, "factory.json")
	definition, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	definition = bytes.ReplaceAll(definition, []byte("failed"), []byte("replacement"))
	if err := os.WriteFile(path, definition, 0600); err != nil {
		t.Fatal(err)
	}
	for _, debug := range []bool{false, true} {
		args := []string{"--json", "run", "--dir", factory, "--continuously", "--with-server"}
		if debug {
			args = append(args, "--debug")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		command := exec.CommandContext(ctx, binary, args...)
		command.Dir, command.Env = repo, builtcliacceptance.ProcessEnvForIsolatedHome(home)
		output, err := command.CombinedOutput()
		cancel()
		exit := assertPlainRestartConflictOutput(t, output, err)
		assertPlainRestartFileUnchanged(t, source, before)
		assertPlainRestartFileUnchanged(t, refPath, reference)
		assertUnreadableBoardLogsPrivate(t, filepath.Join(home, ".you-agent-factory", "logs"), "private-synthetic-sentinel")
		t.Logf("I-RESTART SHA256=%s head=%s debug=%t exit=%d: source/reference byte-identical, structural provenance present", restartCLIArtifact.SHA256, restartCLIArtifact.SourceHead, debug, exit.ExitCode())
	}
}

func assertPlainRestartFileUnchanged(t *testing.T, path string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("restore changed protected recording or reference")
	}
}

func assertPlainRestartConflictOutput(t *testing.T, output []byte, err error) *exec.ExitError {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("conflict start did not fail: %v", err)
	}
	for _, want := range []string{"synthetic-conflict", "task:failed", "event sequence:", "WORK_REQUEST"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("conflict diagnostic lacks %q: %s", want, output)
		}
	}
	if bytes.Contains(output, []byte("private-synthetic-sentinel")) {
		t.Fatal("diagnostic exposed private payload")
	}
	return exit
}
