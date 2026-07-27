package submit_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const functionalBatch = `{
	"requestId": "functional-submit-batch",
	"type": "FACTORY_REQUEST_BATCH",
	"works": [
		{"name": "review", "workTypeName": "task", "payload": {"title": "Review"}}
	]
}`

// TestSubmitFamilyExecutesThroughRootBuiltProcess proves batch dry-run and unary
// submit commands execute through root.BuildProcess with the expected customer output.
func TestSubmitFamilyExecutesThroughRootBuiltProcess(t *testing.T) {
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	t.Run("batch dry-run", func(t *testing.T) {
		var stdout bytes.Buffer
		if err := process.Execute(functionalInput(
			t,
			[]string{
				"you", "--server", "http://127.0.0.1:1",
				"submit", "batch", "--dry-run", functionalBatch,
			},
			&stdout,
		)); err != nil {
			t.Fatalf("Process.Execute(batch dry-run) error = %v", err)
		}
		for _, marker := range []string{
			"requestId: functional-submit-batch",
			"batchSource: inline",
			"dry-run: no request sent",
		} {
			if !strings.Contains(stdout.String(), marker) {
				t.Fatalf("batch dry-run output omitted %q: %q", marker, stdout.String())
			}
		}
	})

	t.Run("unary named session", func(t *testing.T) {
		payloadPath := filepath.Join(t.TempDir(), "request.md")
		if err := os.WriteFile(payloadPath, []byte("# Review\n\nCheck the release."), 0o600); err != nil {
			t.Fatalf("write unary payload: %v", err)
		}
		var method, path string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method, path = r.Method, r.URL.Path
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{
				"accepted": true,
				"requestId": "functional-unary-request",
				"traceId": "functional-unary-trace",
				"workId": "functional-unary-work",
				"name": "release-review",
				"workTypeName": "task"
			}`)
		}))
		t.Cleanup(server.Close)

		var stdout bytes.Buffer
		if err := process.Execute(functionalInput(
			t,
			[]string{
				"you", "--server", server.URL, "--json",
				"submit", "--name", "release-review", "--work-type-name", "task",
				"--payload", payloadPath, "--session", "functional-session",
			},
			&stdout,
		)); err != nil {
			t.Fatalf("Process.Execute(unary submit) error = %v", err)
		}
		if method != http.MethodPost || path != "/factory-sessions/functional-session/work" {
			t.Fatalf("unary request = %s %s", method, path)
		}
		for _, marker := range []string{
			`"sessionId":"functional-session"`,
			`"name":"release-review"`,
			`"workTypeName":"task"`,
		} {
			if !strings.Contains(stdout.String(), marker) {
				t.Fatalf("unary output omitted %q: %q", marker, stdout.String())
			}
		}
	})
}

// TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure proves live
// unary submit enqueues Work before a downstream structured-output failure surfaces.
func TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure(t *testing.T) {
	factoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, factoryDir)
	support.WriteWorkstationConfig(t, factoryDir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
---
Return structured JSON.
`)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("execute live submit"), 0o600); err != nil {
		t.Fatalf("write unary payload: %v", err)
	}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := process.Execute(functionalInput(
		t,
		[]string{
			"you", "--server", server.URL(),
			"submit", "--name", "live-submit", "--work-type-name", "task",
			"--payload", payloadPath,
		},
		&stdout,
	)); err != nil {
		t.Fatalf("Process.Execute(live unary submit) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Submitted: live-submit (task)") {
		t.Fatalf("live unary output = %q", stdout.String())
	}

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed CLI-submitted work = %d, want 1 after invalid mock structured output: %#v", got, listed)
	}
}

func functionalInput(t *testing.T, args []string, stdout io.Writer) root.Input {
	t.Helper()
	home := t.TempDir()
	stdinIsTTY := true
	return root.Input{
		Args:             args,
		Env:              functionalHomeEnvironment(home),
		Stdin:            strings.NewReader(""),
		Stdout:           stdout,
		Stderr:           io.Discard,
		Context:          context.Background(),
		WorkingDirectory: home,
		StdinIsTTY:       &stdinIsTTY,
	}
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}
