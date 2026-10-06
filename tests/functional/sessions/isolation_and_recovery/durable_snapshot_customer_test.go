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
	t.Run("consumed branch stays retired while its sibling resumes", testDurableCustomerBranchRecovery)
	t.Run("last good Work stays readable after a live writer failure", testDurableCustomerLiveWriterFailure)
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

// The API owns this cell: admission and reads share the still-running host.
// The filesystem edge signals the actual failed write; a public read then
// proves that the saved Work remains available without reconstructing the host.
func testDurableCustomerLiveWriterFailure(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	files := &durableCustomerFaultFiles{failure: errors.New("injected live writer failure"), rejected: make(chan struct{}, 1)}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, WorkingDirectory: dir, WaitForServiceModeRuntime: true,
		Args: []string{"--provider", "CODEX", "--model", "gpt-5-codex", "--no-record"},
		Edges: serviceedges.Edges{
			FactorySessionRuntimePersistenceFileSystem: files,
			ProviderCommandRunner:                      &durableCustomerFaultRunner{files: files},
		},
	})
	typeName := "task"
	for _, id := range []string{"last-good-live-work", "writer-fault-live-work"} {
		support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
			RequestId: id, Type: factoryapi.WorkRequestTypeFactoryRequestBatch,
			Works: &[]factoryapi.Work{{WorkId: &id, Name: id, WorkTypeName: &typeName, Payload: map[string]string{"instruction": "complete this Work"}}},
		})
		if id == "last-good-live-work" {
			support.WaitForStatus(t, server.URL(), 20*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 1 })
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	select {
	case <-files.rejected:
	case <-ctx.Done():
		t.Fatal("live writer failure was not reached")
	}
	works := support.ListDefaultSessionWork(t, server.URL())
	if !support.HasWorkAtCustomerState(works, "last-good-live-work", "task:complete") {
		t.Fatalf("live writer failure lost last-good Work: %#v", works.Results)
	}
	for _, item := range works.Results {
		if item.WorkId != nil && *item.WorkId == "last-good-live-work" {
			assertDurableLastGoodOutput(t, factoryapi.ListWorkResponse{Results: []factoryapi.Work{item}})
		}
	}
	files.mu.Lock()
	path, lastGood := files.path, append([]byte(nil), files.lastGood...)
	files.mu.Unlock()
	persisted, err := os.ReadFile(path)
	if err != nil || len(lastGood) == 0 || !bytes.Equal(lastGood, persisted) {
		t.Fatalf("live writer failure replaced last-good snapshot: %v", err)
	}
	server.Stop(t)
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

// Public routing creates a task continuation and an audit Work with distinct
// identities. Finishing the checking path consumes its token while review
// remains blocked at the provider edge. Same-WorkID legacy branches have
// separate private retention coverage; this scenario does not claim that edge.
// Public Work locations and provider calls distinguish retirement from loss
// of the still-reachable sibling after reconstruction.
func testDurableCustomerBranchRecovery(t *testing.T) {
	t.Parallel()
	config := durableBranchFactoryConfig()
	dir := support.ScaffoldFactory(t, config)
	support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
	support.WriteWorkstationConfig(t, dir, "review", "---\ntype: MODEL_WORKSTATION\n---\nFEEDBACK_START{{ (index .Inputs 0).PreviousOutput }}FEEDBACK_END\n")
	support.WriteWorkstationConfig(t, dir, "retire", "---\ntype: MODEL_WORKSTATION\n---\nRETIRE_BRANCH\n")
	feedback := strings.Repeat("b", 50<<10)
	workPath, recordPath := filepath.Join(dir, "work.json"), filepath.Join(dir, "source.jsonl")
	payload := `{"requestId":"branch","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"branch-work","name":"branch","workTypeName":"task","state":"init","content":[{"type":"text","text":"fork this Work"}]}]}`
	if err := os.WriteFile(workPath, []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	initial := &durableFeedbackRunner{firstOutput: feedback, requests: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
	first := startDurableFeedbackServer(t, dir, initial, "--work", workPath, "--record", recordPath)
	assertDurableReviewFeedback(t, initial, feedback)
	// These reads observe asynchronous public projections; waiting for the
	// terminal checking branch ensures its completion is recorded before stop.
	support.WaitForStatus(t, first.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 1 })
	assertDurableBranchLocations(t, first.URL(), "task:processing")
	first.Stop(t)
	first.Close(t)
	prefix := mustReadSeededReplayArtifact(t, recordPath)
	resumed := &durableFeedbackRunner{requests: make(chan platformprocess.CommandRequest, 4), release: make(chan struct{})}
	second := startDurableFeedbackServer(t, dir, resumed, "--resume", recordPath, "--record", filepath.Join(dir, "successor.jsonl"))
	assertDurableReviewFeedback(t, resumed, feedback)
	assertDurableBranchLocations(t, second.URL(), "task:processing")
	close(resumed.release)
	support.WaitForStatus(t, second.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool { return status.Categories.Terminal == 2 })
	assertDurableBranchLocations(t, second.URL(), "task:complete")
	second.Stop(t)
	resumed.mu.Lock()
	calls, retired := resumed.calls, resumed.retired
	resumed.mu.Unlock()
	if calls != 1 || retired != 0 {
		t.Fatalf("recovery dispatched review=%d retired=%d, want 1/0", calls, retired)
	}
	if after := mustReadSeededReplayArtifact(t, recordPath); !bytes.Equal(prefix, after) {
		t.Fatal("branch recovery modified canonical source")
	}
}

func durableBranchFactoryConfig() map[string]any {
	config := seededReplayResumeFactoryConfig()
	config["workTypes"] = append(config["workTypes"].([]map[string]any), map[string]any{
		"name": "audit", "states": []map[string]string{
			{"name": "checking", "type": "INITIAL"}, {"name": "checked", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"},
		},
	})
	stations := config["workstations"].([]map[string]any)
	stations[0]["outputs"] = []map[string]string{{"workType": "task", "state": "processing"}, {"workType": "audit", "state": "checking"}}
	config["workstations"] = append(stations, map[string]any{
		"name": "review", "worker": "worker-a",
		"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
		"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
	}, map[string]any{
		"name": "retire", "worker": "worker-a",
		"inputs":    []map[string]string{{"workType": "audit", "state": "checking"}},
		"outputs":   []map[string]string{{"workType": "audit", "state": "checked"}},
		"onFailure": []map[string]string{{"workType": "audit", "state": "failed"}},
	})
	return config
}

func assertDurableBranchLocations(t *testing.T, baseURL, siblingLocation string) {
	t.Helper()
	works := support.ListDefaultSessionWork(t, baseURL)
	if len(works.Results) != 2 || support.CountWorkAtCustomerState(works, "audit:checked") != 1 || support.CountWorkAtCustomerState(works, siblingLocation) != 1 {
		t.Fatalf("branch locations lost or resurrected: %#v", works.Results)
	}
	if !support.HasWorkAtCustomerState(works, "branch-work", siblingLocation) {
		t.Fatalf("recovery changed the original task identity: %#v", works.Results)
	}
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
	retired     int
}

func (runner *durableFeedbackRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	if strings.Contains(string(request.Stdin), "RETIRE_BRANCH") {
		runner.retired++
		runner.mu.Unlock()
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("branch COMPLETE")}, nil
	}
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
	mu       sync.Mutex
	failure  error
	armed    bool
	path     string
	lastGood []byte
	rejected chan struct{}
}

func (*durableCustomerFaultFiles) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (*durableCustomerFaultFiles) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (files *durableCustomerFaultFiles) WriteFile(path string, payload []byte, mode fs.FileMode) error {
	files.mu.Lock()
	defer files.mu.Unlock()
	if files.armed {
		select {
		case files.rejected <- struct{}{}:
		default:
		}
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
