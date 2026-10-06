package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The two process graphs represent the customer closing a failed invocation
// and reopening its project. Default-board recording recovery is intentionally
// serialized within this isolated project; the scenario is parallel with other
// projects. The private snapshot's byte preservation and the canonical recording's
// public Work recovery are separate assertions, not interchangeable evidence.
func TestDurableSnapshotCustomerBehavior(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	t.Run("saved process feedback reaches the resumed review provider", testDurableCustomerFeedbackRecovery)
	t.Run("ordinary writer failure preserves the last durable result", func(t *testing.T) {
		t.Parallel()
		dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
		support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
		files := &durableCustomerFaultFiles{failure: errors.New("injected atomic writer failure")}
		process := support.BuildProcess(t, serviceedges.Edges{
			FactorySessionsWorkingDirectory:            platformfilesystem.Local{WorkingDirectory: dir},
			FactorySessionRuntimePersistenceFileSystem: files,
			ProviderCommandRunner:                      &durableCustomerFaultRunner{files: files},
		})
		batch := `{"requestId":"writer-fault","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"writer-fault-work","name":"persist","workTypeName":"task","state":"init","content":[{"type":"text","text":"preserve this Work"}]}]}`
		workPath := filepath.Join(dir, "work.json")
		recordPath := filepath.Join(dir, "last-good.jsonl")
		inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", dir, "--work", workPath, "--provider", "CODEX", "--model", "gpt-5-codex", "--quiet", "--record", recordPath})
		home := t.TempDir()
		inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
		inputs.Input.WorkingDirectory = dir
		if err := os.WriteFile(workPath, []byte(strings.ReplaceAll(batch, "writer-fault", "last-good")), 0600); err != nil {
			t.Fatal(err)
		}
		if err := process.Execute(inputs.Input); err != nil {
			t.Fatalf("initial successful Work: %v; stderr=%s", err, inputs.Stderr())
		}
		files.mu.Lock()
		lastGood := append([]byte(nil), files.lastGood...)
		path := files.path
		files.mu.Unlock()
		prefix, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(workPath, []byte(batch), 0600); err != nil {
			t.Fatal(err)
		}
		inputs.Input.Args = []string{"you", "run", "--dir", dir, "--work", workPath, "--provider", "CODEX", "--model", "gpt-5-codex", "--quiet", "--no-record"}
		if err := process.Execute(inputs.Input); !errors.Is(err, files.failure) {
			t.Fatalf("ordinary writer failure = %v, want original error identity; stderr=%s", err, inputs.Stderr())
		}
		if err := process.Close(t.Context()); err != nil {
			t.Fatalf("close failed invocation: %v", err)
		}
		persisted, err := os.ReadFile(path)
		if err != nil || len(lastGood) == 0 || !bytes.Equal(persisted, lastGood) {
			t.Fatalf("writer failure changed last-good file: bytes=%d err=%v", len(persisted), err)
		}
		server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
			FactoryDir: dir, WorkingDirectory: dir, WaitForServiceModeRuntime: true,
			Args:  []string{"--provider", "CODEX", "--model", "gpt-5-codex", "--resume", recordPath, "--record", filepath.Join(dir, "successor.jsonl")},
			Edges: serviceedges.Edges{ProviderCommandRunner: support.NewStaticSuccessCommandRunner("unexpected recovery dispatch COMPLETE")},
		})
		session := support.GetDefaultSession(t, server.URL())
		works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(server.URL(), session.Id, "/work"))
		if len(works.Results) != 1 || !support.HasWorkAtCustomerState(works, "last-good-work", "task:complete") {
			t.Fatalf("last-good recorded Work not recovered exactly: %#v", works.Results)
		}
		assertDurableLastGoodOutput(t, works)
		server.Stop(t)
		after, err := os.ReadFile(recordPath)
		if err != nil || !bytes.Equal(prefix, after) {
			t.Fatalf("recovery changed canonical source: %v", err)
		}
	})
}

func assertDurableLastGoodOutput(t *testing.T, works factoryapi.ListWorkResponse) {
	t.Helper()
	encoded, err := json.Marshal(works)
	if err != nil || !strings.Contains(string(encoded), "saved completion COMPLETE") || strings.Contains(string(encoded), "unsaved completion") {
		t.Fatalf("recovered Work lost the saved output or published failed output: %s err=%v", encoded, err)
	}
}

// Each graph owns one isolated default project. Reconstructing the graph is
// the behavior under test: the successor must read the saved recording rather
// than inheriting the initial provider's in-memory request or output.
func testDurableCustomerFeedbackRecovery(t *testing.T) {
	t.Parallel()
	config := seededReplayResumeFactoryConfig()
	config["workTypes"] = append(config["workTypes"].([]map[string]any), map[string]any{
		"name": "witness", "states": []map[string]string{{"name": "init", "type": "INITIAL"}},
	})
	stations := config["workstations"].([]map[string]any)
	stations[0]["outputs"] = []map[string]string{{"workType": "task", "state": "processing"}}
	config["workstations"] = append(stations, map[string]any{
		"name": "review", "worker": "worker-a",
		"inputs":    []map[string]string{{"workType": "task", "state": "processing"}, {"workType": "witness", "state": "init"}},
		"guards":    []map[string]any{{"type": "MATCHES_FIELDS", "matchConfig": map[string]string{"inputKey": `.Tags["_last_output"]`}}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	})
	dir := support.ScaffoldFactory(t, config)
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	support.WriteWorkstationConfig(t, dir, "review", "---\ntype: MODEL_WORKSTATION\n---\nFEEDBACK_START{{ (index .Inputs 0).PreviousOutput }}FEEDBACK_END\n")
	feedback := strings.Repeat("x", 50<<10)
	batch := `{"requestId":"feedback","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"feedback-work","name":"feedback","workTypeName":"task","state":"init","content":[{"type":"text","text":"write a draft"}]}]}`
	var request map[string]any
	if err := json.Unmarshal([]byte(batch), &request); err != nil {
		t.Fatal(err)
	}
	request["works"] = append(request["works"].([]any), map[string]any{
		"workId": "feedback-witness", "name": "witness", "workTypeName": "witness", "state": "init",
		"tags": map[string]string{"_last_output": feedback},
	})
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	workPath, recordPath := filepath.Join(dir, "work.json"), filepath.Join(dir, "source.jsonl")
	if err := os.WriteFile(workPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	initial := &durableFeedbackRunner{firstOutput: feedback, requests: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
	first := startDurableFeedbackServer(t, dir, initial, "--work", workPath, "--record", recordPath)
	assertDurableReviewFeedback(t, initial, feedback)
	first.Stop(t)
	first.Close(t)
	prefix := mustReadSeededReplayArtifact(t, recordPath)
	resumed := &durableFeedbackRunner{requests: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
	second := startDurableFeedbackServer(t, dir, resumed, "--resume", recordPath, "--record", filepath.Join(dir, "successor.jsonl"))
	assertDurableReviewFeedback(t, resumed, feedback)
	session := support.GetDefaultSession(t, second.URL())
	works := support.GetJSON[factoryapi.ListWorkResponse](t, support.SessionWorkURL(second.URL(), session.Id, "/work"))
	if !support.HasWorkAtCustomerState(works, "feedback-work", "task:processing") {
		t.Fatalf("resumed review lost recorded Work state: %#v", works.Results)
	}
	close(resumed.release)
	support.WaitForStatus(t, second.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 1 })
	second.Stop(t)
	resumed.mu.Lock()
	calls := resumed.calls
	resumed.mu.Unlock()
	if calls != 1 {
		t.Fatalf("recovered feedback guard routed %d review attempts, want one", calls)
	}
	if after := mustReadSeededReplayArtifact(t, recordPath); !bytes.Equal(prefix, after) {
		t.Fatal("resume modified canonical source recording")
	}
}

func startDurableFeedbackServer(t *testing.T, dir string, runner *durableFeedbackRunner, args ...string) *support.FunctionalAPIServer {
	t.Helper()
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WorkingDirectory: dir, WaitForServiceModeRuntime: true,
		Args:  append([]string{"--provider", "CODEX", "--model", "gpt-5-codex"}, args...),
		Edges: serviceedges.Edges{ProviderCommandRunner: runner},
	})
}

func assertDurableReviewFeedback(t *testing.T, runner *durableFeedbackRunner, feedback string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	select {
	case request := <-runner.requests:
		prompt := strings.Join(request.Args, " ") + string(request.Stdin)
		if !strings.Contains(prompt, "FEEDBACK_START"+feedback+"FEEDBACK_END") {
			t.Fatalf("review provider did not receive exact %d-byte feedback; prompt bytes=%d", len(feedback), len(prompt))
		}
	case <-ctx.Done():
		t.Fatal("review provider boundary not reached")
	}
}

type durableFeedbackRunner struct {
	firstOutput string
	requests    chan platformprocess.CommandRequest
	release     chan struct{}
	mu          sync.Mutex
	calls       int
}

func (runner *durableFeedbackRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.calls++
	first := runner.calls == 1 && runner.firstOutput != ""
	runner.mu.Unlock()
	if first {
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(runner.firstOutput)}, nil
	}
	select {
	case runner.requests <- request:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	select {
	case <-runner.release:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("review COMPLETE")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

type durableCustomerFaultFiles struct {
	platformfilesystem.Local
	mu       sync.Mutex
	failure  error
	armed    bool
	path     string
	lastGood []byte
}

func (*durableCustomerFaultFiles) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (*durableCustomerFaultFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (files *durableCustomerFaultFiles) ReadFileBounded(path string, limit int64) ([]byte, error) {
	return platformfilesystem.NewRecovery(files.Local, files.Local).ReadFileBounded(path, limit)
}
func (files *durableCustomerFaultFiles) RenameNoReplace(source, destination string) error {
	return platformfilesystem.NewRecovery(files.Local, files.Local).RenameNoReplace(source, destination)
}
func (files *durableCustomerFaultFiles) WriteFile(path string, payload []byte, mode fs.FileMode) error {
	files.mu.Lock()
	defer files.mu.Unlock()
	if files.armed {
		return files.failure
	}
	if err := os.WriteFile(path, payload, mode); err != nil {
		return err
	}
	files.path, files.lastGood = path, append([]byte(nil), payload...)
	return nil
}

type durableCustomerFaultRunner struct {
	files *durableCustomerFaultFiles
	calls int
}

func (runner *durableCustomerFaultRunner) Run(_ context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.files.mu.Lock()
	defer runner.files.mu.Unlock()
	runner.calls++
	runner.files.armed = runner.calls > 1
	output := "saved completion COMPLETE"
	if runner.files.armed {
		output = "unsaved completion COMPLETE"
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}, nil
}
