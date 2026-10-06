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
		encoded, err := json.Marshal(works)
		if err != nil || !strings.Contains(string(encoded), "saved completion COMPLETE") || strings.Contains(string(encoded), "unsaved completion") {
			t.Fatalf("recovered Work lost the saved output or published failed output: %s err=%v", encoded, err)
		}
		server.Stop(t)
		after, err := os.ReadFile(recordPath)
		if err != nil || !bytes.Equal(prefix, after) {
			t.Fatalf("recovery changed canonical source: %v", err)
		}
	})
}

type durableCustomerFaultFiles struct {
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
