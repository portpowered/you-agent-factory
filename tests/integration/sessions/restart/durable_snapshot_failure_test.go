package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

var durableArtifactFeedback = flag.String("durable-feedback", "", "delivered SCRIPT worker feedback fixture")

// The invoking build supplies one CLI for two real serving generations and
// a finite batch that crosses the persistence filesystem and OS exit boundary.
func TestPrebuiltDurableSnapshotRecoveryAndFailure(t *testing.T) {
	t.Parallel()
	t.Run("serving and recovered feedback", testPrebuiltDurableFeedbackRecovery)
	t.Run("safe writer failure exit", testPrebuiltDurableWriterFailure)
}

func testPrebuiltDurableWriterFailure(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	factory := scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig())
	home := t.TempDir()
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", boardPersistenceWorkerConfig(currentRestartWorkerExecutable(t)))
	release := filepath.Join(t.TempDir(), "released")
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(factory, ".you-agent-factory", "durable-sessions")
	if err := os.MkdirAll(filepath.Dir(directory), 0700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "preserve obstructing file"
	if err := os.WriteFile(directory, []byte(sentinel), 0600); err != nil {
		t.Fatal(err)
	}
	batch := `{"requestId":"snapshot-writer-fault","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"snapshot-writer-fault","name":"persist","workTypeName":"task","state":"processing","content":[{"type":"text","text":"persist this Work"}]}]}`
	workPath := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(workPath, []byte(batch), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	args := []string{"--json", "run", "--dir", factory, "--work", workPath, "--no-record"}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = factory
	command.Env = append(builtcliacceptance.ProcessEnvForIsolatedHome(home), boardPersistenceHelperEnv+"="+boardPersistenceHelperEnvValue, boardPersistenceReleaseEnv+"="+release)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || ctx.Err() != nil {
		t.Fatalf("filesystem failure exit=%v context=%v stdout=%q stderr=%q", err, ctx.Err(), stdout.String(), stderr.String())
	}
	assertDurableSnapshotFailureOutput(t, stdout.Bytes(), stderr.Bytes())
	contents, readErr := os.ReadFile(directory)
	if readErr != nil || string(contents) != sentinel {
		t.Fatalf("failure changed obstruction: %q %v", contents, readErr)
	}
	t.Logf("I2 artifact SHA256=%s argv=%q exit=1 stdout_bytes=%d stderr=%q", restartCLIArtifact.SHA256, args, stdout.Len(), stderr.String())
}

// This helper is a real local SCRIPT worker. It sends its templated feedback
// argument to the parent's gate and returns output through the real OS pipe.
func TestDurableSnapshotFeedbackWorkerHelper(t *testing.T) {
	if os.Getenv(boardPersistenceHelperEnv) != boardPersistenceHelperEnvValue {
		return
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, os.Getenv(boardPersistenceWorkerReadyEnv), strings.NewReader(*durableArtifactFeedback))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("worker gate status=%d", response.StatusCode)
	}
	if _, err := io.Copy(os.Stdout, response.Body); err != nil {
		t.Fatal(err)
	}
}

func testPrebuiltDurableFeedbackRecovery(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	config := boardPersistenceFactoryConfig()
	stations := config["workstations"].([]map[string]any)
	config["workstations"] = append(stations, map[string]any{
		"name": "process", "worker": "restart-blocker",
		"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	factory := scaffoldBoardPersistenceFactory(t, config)
	writeDurableFeedbackWorkstations(t, factory)
	worker := strings.Replace(boardPersistenceWorkerConfig(currentRestartWorkerExecutable(t)), "TestBoardPersistenceWorkerHelper", "TestDurableSnapshotFeedbackWorkerHelper", 1)
	worker = strings.Replace(worker, "timeout: 2m", "  - '-durable-feedback={{ (index .Inputs 0).PreviousOutput }}'\ntimeout: 2m", 1)
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", worker)
	// SCRIPT arguments cross Windows' command-line boundary. The 50 KiB case
	// lives in the functional Codex stdin cell; this real-edge cell uses 8 KiB.
	feedback := strings.Repeat("x", 8<<10)
	requests, release := make(chan string, 4), make(chan struct{})
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			select {
			case requests <- string(body):
			case <-r.Context().Done():
				return
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_, _ = fmt.Fprint(w, "review COMPLETE")
			return
		}
		_, _ = fmt.Fprint(w, feedback)
	}))
	t.Cleanup(gate.Close)
	home, record := t.TempDir(), filepath.Join(factory, "source.jsonl")
	first := startBoardPersistenceDaemonProcessWithResumeOutput(t, binary, factory, home, "", record, "", false, false, gate.URL)
	waitForBoardDaemonReady(t, first, 30*time.Second)
	batch := `{"requestId":"feedback","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"feedback-work","name":"feedback","workTypeName":"task","state":"init","content":[{"type":"text","text":"write a draft"}]}]}`
	submitBatchThroughCLI(t, t.Context(), first, binary, factory, home, batch, "feedback", 1)
	savedFeedback := waitDurableArtifactFeedback(t, requests)
	if !strings.Contains(savedFeedback, feedback) {
		t.Fatal("initial delivered review lost 8 KiB output")
	}
	first.stop(t)
	prefix := readDurableArtifactSource(t, record)
	second := startBoardPersistenceDaemonProcessWithResumeOutput(t, binary, factory, home, record, filepath.Join(factory, "successor.jsonl"), "", false, false, gate.URL)
	waitForBoardDaemonReady(t, second, 30*time.Second)
	if recovered := waitDurableArtifactFeedback(t, requests); recovered != savedFeedback {
		t.Fatal("recovered delivered review feedback differs from saved output")
	}
	waitForBoardStates(t, second.baseURL, map[string]string{"feedback-work": "processing"}, 20*time.Second)
	close(release)
	waitForBoardStates(t, second.baseURL, map[string]string{"feedback-work": "complete"}, 20*time.Second)
	second.stop(t)
	if !bytes.Equal(prefix, readDurableArtifactSource(t, record)) {
		t.Fatal("delivered resume modified canonical source recording")
	}
	t.Logf("I1 artifact SHA256=%s feedback_bytes=%d two real serving generations, terminal Work complete", restartCLIArtifact.SHA256, len(savedFeedback))
}

func writeDurableFeedbackWorkstations(t *testing.T, factory string) {
	t.Helper()
	for station, prompt := range map[string]string{"process": "Write a draft.", "hold-processing": "FEEDBACK_START{{ (index .Inputs 0).PreviousOutput }}FEEDBACK_END"} {
		path := filepath.Join(factory, "workstations", station, "AGENTS.md")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\ntype: MODEL_WORKSTATION\n---\n"+prompt+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func waitDurableArtifactFeedback(t *testing.T, requests <-chan string) string {
	t.Helper()
	select {
	case body := <-requests:
		return body
	case <-time.After(30 * time.Second):
		t.Fatal("delivered review worker boundary not reached")
		return ""
	}
}

func readDurableArtifactSource(t *testing.T, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertDurableSnapshotFailureOutput(t *testing.T, stdout, stderr []byte) {
	t.Helper()
	if strings.Count(string(stderr), "create durable session persistence directory") != 1 {
		t.Fatalf("terminal persistence diagnostic missing or duplicated: %q", stderr)
	}
	var diagnostic struct{ Code, Message string }
	if err := json.Unmarshal(stderr, &diagnostic); err != nil || diagnostic.Code != "DURABLE_SESSION_PERSISTENCE_FAILED" || diagnostic.Message != "create durable session persistence directory failed" {
		t.Fatalf("unsafe or malformed terminal diagnostic: %q (%v)", stderr, err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(stdout), []byte("\n")) {
		if len(line) > 0 && !json.Valid(line) {
			t.Fatalf("non-JSON stdout on --json failure: %q", line)
		}
	}
}
