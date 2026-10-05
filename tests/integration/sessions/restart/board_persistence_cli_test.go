package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

func runBoardPersistenceCLI(
	ctx context.Context,
	binaryPath, factoryDir, homeDir, baseURL string,
	args ...string,
) ([]byte, error) {
	commandArgs := append([]string{"--server", baseURL}, args...)
	command := exec.CommandContext(ctx, binaryPath, commandArgs...)
	command.Dir = factoryDir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	return command.CombinedOutput()
}

func submitBatchThroughCLI(
	t *testing.T,
	ctx context.Context,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir, batchJSON, requestID string,
	wantWorkCount int,
) {
	t.Helper()
	output, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "submit", "batch", batchJSON)
	if err != nil {
		t.Fatalf("you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	var acknowledgement struct {
		RequestID string `json:"requestId"`
		WorkCount int    `json:"workCount"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &acknowledgement); err != nil {
		t.Fatalf("decode you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	if acknowledgement.RequestID != requestID || acknowledgement.WorkCount != wantWorkCount {
		t.Fatalf("you submit batch acknowledgement = %#v, want requestId %q and workCount %d", acknowledgement, requestID, wantWorkCount)
	}
}

// One prebuilt-artifact cell covers OS CLI success across two real host
// generations. The exhaustive diagnostic matrix belongs to functional tests.
func TestHistoryListingResilienceAfterRestart(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	home := t.TempDir()
	factory := scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig())
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", "---\ntype: SCRIPT_WORKER\ncommand: unused-history-worker\n---\n")
	recordingRoot := filepath.Join(home, ".you-agent-factory", "recordings", "2026", "10", "03")
	fixtures := seedRestartHistory(t, recordingRoot)
	for generation := range 2 {
		daemon := startBoardPersistenceDaemon(t, binary, factory, home, filepath.Join(t.TempDir(), "host.json"), "")
		output, err := runHistoryListingCLI(t, binary, factory, home, daemon.baseURL)
		if err != nil {
			t.Fatalf("you --json session list --history-only exit failure: %v: %s", err, output)
		}
		assertRestartHistory(t, output, home)
		t.Logf("artifact SHA256=%s generation=%d command=you --json session list --history-only exit=0 stdout=%s", restartCLIArtifact.SHA256, generation, output)
		daemon.stop(t)
	}
	for path, expected := range fixtures {
		actual, err := os.ReadFile(filepath.Join(recordingRoot, path))
		if err != nil || string(actual) != expected {
			t.Fatalf("history mutated %s: %q %v", path, actual, err)
		}
	}
}

func seedRestartHistory(t *testing.T, recordingRoot string) map[string]string {
	t.Helper()
	if err := os.MkdirAll(recordingRoot, 0700); err != nil {
		t.Fatal(err)
	}
	fixtures := map[string]string{
		"healthy.json": `{"schemaVersion":"replay.v1","events":[{"context":{"sessionId":"00000000-0000-4000-8000-000000000001"},"id":"event-1","payload":{}}]}`,
		"00000000-0000-4000-8000-000000000002.jsonl": `{"recordType":"header","schemaVersion":"agent-factory.replay.v2","recordedAt":"2026-10-03T00:00:00Z","sessionId":"00000000-0000-4000-8000-000000000002","factoryIdentity":{"id":"factory","name":"factory","factoryDirectory":"factory","sourceDirectory":"factory"},"hashes":{"factory_hash":"sha256:factory","workers_hash":"sha256:workers","workstations_hash":"sha256:workstations","runtime_config_hash":"sha256:runtime"}}` + "\n",
		"empty.json": "", "truncated.json": `{"schemaVersion":"replay.v1","events":[`,
	}
	for path, data := range fixtures {
		if err := os.WriteFile(filepath.Join(recordingRoot, path), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return fixtures
}

func assertRestartHistory(t *testing.T, output []byte, home string) {
	t.Helper()
	var result factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytes.TrimSpace(output), &result); err != nil {
		t.Fatalf("history JSON: %v: %s", err, output)
	}
	if result.RecordedSessions == nil || len(*result.RecordedSessions) != 2 || result.Warnings == nil || len(*result.Warnings) != 2 {
		t.Fatalf("mixed history = %s", output)
	}
	ids := []string{(*result.RecordedSessions)[0].SessionId, (*result.RecordedSessions)[1].SessionId}
	if !reflect.DeepEqual(ids, []string{"00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002"}) {
		t.Fatalf("healthy order = %v", ids)
	}
	paths := []string{(*result.Warnings)[0].ArtifactReference, (*result.Warnings)[1].ArtifactReference}
	if !reflect.DeepEqual(paths, []string{"2026/10/03/empty.json", "2026/10/03/truncated.json"}) {
		t.Fatalf("warning paths = %v", paths)
	}
	for _, warning := range *result.Warnings {
		if warning.Code != "UNREADABLE_RECORDING" || warning.Reason == "" {
			t.Fatalf("warning = %#v", warning)
		}
	}
	if strings.Contains(string(output), home) {
		t.Fatal("history leaked absolute home path")
	}
}

func runHistoryListingCLI(t *testing.T, binary, factory, home, endpoint string) ([]byte, error) {
	t.Helper()
	args := []string{"--server", endpoint, "--json", "session", "list", "--history-only"}
	command := exec.CommandContext(t.Context(), binary, args...)
	command.Dir = factory
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	exitCode := -1
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	t.Logf("CLI argv=%q OS exit=%d stdout=%s stderr=%s", args, exitCode, stdout.String(), stderr.String())
	return stdout.Bytes(), err
}
