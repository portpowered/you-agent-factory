package contentmaterialization_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSubmittedWorkContentReachesWorkerAsMaterializedFiles exercises the
// customer path through root.BuildProcess and Process.Execute. The command
// edge reads both local and inline content after Work admission has resolved
// them, while the second Work proves an unsafe URL fails before that edge.
func TestSubmittedWorkContentReachesWorkerAsMaterializedFiles(t *testing.T) {
	factoryDir, localURL, inlineURL, localBytes, inlineBytes := prepareContentMaterializationFixture(t)

	runner := &contentCommandRunner{
		want: map[string][]byte{
			"part0": localBytes,
			"part1": inlineBytes,
		},
	}
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ScriptCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	requestPath := writeContentMaterializationRequest(t, localURL, inlineURL)
	submitContentMaterializationRequest(t, server, factoryDir, requestPath)

	support.WaitForTerminalStatus(t, server.URL(), 15*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("script command calls = %d, want one successful Work invocation", runner.CallCount())
	}
	runner.AssertObserved(t)

	if _, err := os.Stat(runner.path("part0")); err != nil {
		t.Fatalf("local materialized path = %q, want retained source file: %v", runner.path("part0"), err)
	}
	if _, err := os.Stat(runner.path("part1")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inline materialized path = %q, stat error = %v, want cleanup removal", runner.path("part1"), err)
	}
}

func prepareContentMaterializationFixture(t *testing.T) (string, string, string, []byte, []byte) {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, contentMaterializationFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "content-worker", `---
type: SCRIPT_WORKER
command: capture-content
args:
  - "part0-file={{ (index (index .Inputs 0).Content 0).File }}"
  - "part0-url={{ (index (index .Inputs 0).Content 0).URL }}"
  - "part0-type={{ (index (index .Inputs 0).Content 0).Type }}"
  - "part0-content-type={{ (index (index .Inputs 0).Content 0).ContentType }}"
  - "part0-metadata={{ (index (index .Inputs 0).Content 0).Metadata }}"
  - "part1-file={{ (index (index .Inputs 0).Content 1).File }}"
  - "part1-url={{ (index (index .Inputs 0).Content 1).URL }}"
  - "part1-type={{ (index (index .Inputs 0).Content 1).Type }}"
  - "part1-content-type={{ (index (index .Inputs 0).Content 1).ContentType }}"
  - "part1-metadata={{ (index (index .Inputs 0).Content 1).Metadata }}"
---
Read the submitted binary content.
`)
	localBytes := []byte("local-content-bytes")
	localPath := filepath.Join(t.TempDir(), "local-content.bin")
	if err := os.WriteFile(localPath, localBytes, 0o600); err != nil {
		t.Fatalf("write local content: %v", err)
	}
	localURL, err := work.FilesystemPathToContentURL(localPath)
	if err != nil {
		t.Fatalf("build local content URL: %v", err)
	}
	inlineBytes := []byte("inline-content-bytes")
	inlineURL := "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(inlineBytes)
	return factoryDir, localURL, inlineURL, localBytes, inlineBytes
}

func writeContentMaterializationRequest(t *testing.T, localURL, inlineURL string) string {
	t.Helper()
	requestPath := filepath.Join(t.TempDir(), "content-request.json")
	request := map[string]any{
		"requestId": "functional-content-materialization",
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{
			{
				"name":         "materialized-content",
				"workTypeName": "task",
				"content": []map[string]any{
					{
						"type":        "BINARY",
						"url":         localURL,
						"contentType": "application/octet-stream",
						"label":       "local",
						"metadata":    map[string]any{"source": "local-submission"},
					},
					{
						"type":        "BINARY",
						"url":         inlineURL,
						"contentType": "application/octet-stream",
						"label":       "inline",
						"metadata":    map[string]any{"source": "inline-submission"},
					},
				},
			},
			{
				"name":         "unsafe-content",
				"workTypeName": "task",
				"content": []map[string]any{{
					"type":        "BINARY",
					"url":         "http://127.0.0.1/private.bin",
					"contentType": "application/octet-stream",
				}},
			},
		},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal content request: %v", err)
	}
	if err := os.WriteFile(requestPath, payload, 0o600); err != nil {
		t.Fatalf("write content request: %v", err)
	}
	return requestPath
}

func submitContentMaterializationRequest(
	t *testing.T,
	server *support.FunctionalAPIServer,
	factoryDir, requestPath string,
) {
	t.Helper()
	// This is the ordinary customer-facing batch transport. The host process
	// was built by StartFunctionalAPIServer through the same root composition;
	// this separate client process enters the public CLI through Process.Execute.
	client := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--server", server.URL(),
		"submit", "batch",
		"--file", requestPath,
	})
	inputs.Input.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"USERPROFILE="+t.TempDir(),
	)
	inputs.Input.WorkingDirectory = factoryDir
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := client.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(submit batch) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
}

func contentMaterializationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "functional-content-materialization",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "content-worker",
		}},
		"workstations": []map[string]any{{
			"name":      "process-content",
			"worker":    "content-worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

type contentCommandRunner struct {
	mu       sync.Mutex
	calls    int
	args     []string
	paths    map[string]string
	want     map[string][]byte
	observed map[string][]byte
}

func (runner *contentCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	paths := map[string]string{}
	for _, arg := range request.Args {
		for _, part := range []string{"part0-file=", "part1-file="} {
			if strings.HasPrefix(arg, part) {
				key := strings.TrimSuffix(strings.TrimPrefix(part, "part"), "-file=")
				paths["part"+key] = strings.TrimPrefix(arg, part)
			}
		}
	}
	observed := make(map[string][]byte, len(paths))
	for key, path := range paths {
		if strings.TrimSpace(path) == "" {
			return platformprocess.CommandResult{}, errors.New(key + " did not receive a materialized file")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		observed[key] = content
	}

	runner.mu.Lock()
	runner.calls++
	runner.args = append([]string(nil), request.Args...)
	runner.paths = paths
	runner.observed = observed
	runner.mu.Unlock()
	return platformprocess.CommandResult{Stdout: []byte("content materialized")}, nil
}

func (runner *contentCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *contentCommandRunner) path(key string) string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.paths[key]
}

func (runner *contentCommandRunner) AssertObserved(t *testing.T) {
	t.Helper()
	runner.mu.Lock()
	defer runner.mu.Unlock()
	for key, want := range runner.want {
		if !equalBytes(runner.observed[key], want) {
			t.Fatalf("%s bytes = %q, want %q", key, runner.observed[key], want)
		}
	}
	joined := strings.Join(runner.args, "\n")
	for _, marker := range []string{
		"part0-url=",
		"part1-url=",
		"part0-type=BINARY",
		"part1-type=BINARY",
		"part0-content-type=application/octet-stream",
		"part1-content-type=application/octet-stream",
		"local-submission",
		"inline-submission",
	} {
		if !strings.Contains(joined, marker) {
			t.Fatalf("script edge args missing %q: %v", marker, runner.args)
		}
	}
	if strings.Contains(joined, "part0-url=file:") || strings.Contains(joined, "part1-url=data:") {
		t.Fatalf("script edge received unresolved content URL: %v", runner.args)
	}
}

func equalBytes(got, want []byte) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

var _ platformprocess.CommandRunner = (*contentCommandRunner)(nil)
