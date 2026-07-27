package commands_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	submitWiringInlineRequestID = "cli-submit-wiring-inline"
	submitWiringInlineWorkName  = "inline-task"
	submitWiringInlineWorkType  = "task"

	submitWiringFileRequestID = "cli-submit-wiring-file"
	submitWiringFileWorkName  = "file-task"
	submitWiringFileWorkType  = "task"

	submitWiringUnavailableRequestID = "cli-submit-wiring-unavailable"
	submitWiringUnavailableWorkName  = "unavailable-task"
	submitWiringUnavailableWorkType  = "task"
	submitWiringUnavailableServer      = "http://127.0.0.1:1"

	submitWiringBackendErrorRequestID        = "cli-submit-wiring-backend-error"
	submitWiringBackendErrorWorkName         = "backend-error-task"
	submitWiringBackendErrorWorkType         = "task"
	submitWiringBackendErrorUnsafeMessage    = "payload-secret access-token-secret"
	submitWiringBackendErrorUnsafeCredential = "sk-proj-secret123"
)

// TestCLISubmitBatchInlineJSON proves you submit batch accepts inline canonical
// FACTORY_REQUEST_BATCH JSON against a running Factory Session server and prints
// transport acknowledgment markers without staging a file.
func TestCLISubmitBatchInlineJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit wiring")
	}

	factoryDir := support.ScaffoldFactory(t, submitWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	binaryPath := buildYouCLIBinary(t)
	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Inline submit wiring"}}]}`,
		submitWiringInlineRequestID,
		submitWiringInlineWorkName,
		submitWiringInlineWorkType,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	submitCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", server.URL(),
		"submit", "batch",
		inlineBatch,
	)
	submitCmd.Dir = factoryDir

	submitOut, err := submitCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}

	output := string(submitOut)
	for _, marker := range []string{
		"requestId: " + submitWiringInlineRequestID,
		"traceId:",
		"work count: 1",
		submitWiringInlineWorkName + " (" + submitWiringInlineWorkType + ")",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, output)
		}
	}
}

// TestCLISubmitBatchFile proves you submit batch accepts a filesystem path to
// canonical FACTORY_REQUEST_BATCH JSON against a running Factory Session server
// and prints transport acknowledgment markers for the staged batch.
func TestCLISubmitBatchFile(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit wiring")
	}

	factoryDir := support.ScaffoldFactory(t, submitWiringFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	batchPath := filepath.Join(t.TempDir(), "submit-wiring-batch.json")
	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"File submit wiring"}}]}`,
		submitWiringFileRequestID,
		submitWiringFileWorkName,
		submitWiringFileWorkType,
	)
	if err := os.WriteFile(batchPath, []byte(batchJSON), 0o600); err != nil {
		t.Fatalf("write batch file: %v", err)
	}

	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	submitCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", server.URL(),
		"submit", "batch",
		batchPath,
	)
	submitCmd.Dir = factoryDir

	submitOut, err := submitCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}

	output := string(submitOut)
	for _, marker := range []string{
		"requestId: " + submitWiringFileRequestID,
		"traceId:",
		"work count: 1",
		submitWiringFileWorkName + " (" + submitWiringFileWorkType + ")",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, output)
		}
	}
}

// TestCLISubmitUnavailableServer proves you submit batch exits with the documented
// failure code and an actionable unreachable Factory diagnostic when the server
// address cannot be reached, without printing a success acknowledgment payload.
func TestCLISubmitUnavailableServer(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit wiring")
	}

	binaryPath := buildYouCLIBinary(t)
	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Unavailable server wiring"}}]}`,
		submitWiringUnavailableRequestID,
		submitWiringUnavailableWorkName,
		submitWiringUnavailableWorkType,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	submitCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", submitWiringUnavailableServer,
		"submit", "batch",
		inlineBatch,
	)
	submitCmd.Dir = t.TempDir()

	submitOut, err := submitCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("you submit batch unexpectedly succeeded against unavailable server:\n%s", submitOut)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you submit batch error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("you submit batch exit code = %d, want 1", exitErr.ExitCode())
	}

	output := string(submitOut)
	if !strings.Contains(output, "factory not reachable at") {
		t.Fatalf("submit batch output missing unreachable factory diagnostic:\n%s", output)
	}

	for _, marker := range []string{
		"requestId: " + submitWiringUnavailableRequestID,
		"traceId:",
		"work count:",
	} {
		if strings.Contains(output, marker) {
			t.Fatalf("submit batch output must not contain success acknowledgment marker %q:\n%s", marker, output)
		}
	}
}

// TestCLISubmitBackendErrorPreservesPublicMessage proves you submit batch exits
// non-success and preserves the public backend error's safe typed fields when
// the Factory Session server rejects the batch request.
func TestCLISubmitBackendErrorPreservesPublicMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit wiring")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("request method = %s, want PUT", r.Method)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, fmt.Sprintf(`{
			"message":%q,
			"code":"BAD_REQUEST",
			"family":"BAD_REQUEST",
			"workId":%q
		}`, submitWiringBackendErrorUnsafeMessage, submitWiringBackendErrorUnsafeCredential))
	}))
	t.Cleanup(server.Close)

	binaryPath := buildYouCLIBinary(t)
	inlineBatch := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workTypeName":%q,"payload":{"title":"Backend error wiring"}}]}`,
		submitWiringBackendErrorRequestID,
		submitWiringBackendErrorWorkName,
		submitWiringBackendErrorWorkType,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	submitCmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", server.URL,
		"submit", "batch",
		inlineBatch,
	)
	submitCmd.Dir = t.TempDir()

	submitOut, err := submitCmd.CombinedOutput()
	if err == nil {
		t.Fatalf("you submit batch unexpectedly succeeded against backend error:\n%s", submitOut)
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you submit batch error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("you submit batch exit code = %d, want 1", exitErr.ExitCode())
	}

	output := string(submitOut)
	for _, marker := range []string{
		"batch submission failed (400)",
		"code=BAD_REQUEST",
		"family=BAD_REQUEST",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("submit batch output missing public backend error marker %q:\n%s", marker, output)
		}
	}

	for _, leaked := range []string{
		submitWiringBackendErrorUnsafeMessage,
		"access-token",
		submitWiringBackendErrorUnsafeCredential,
		"requestId: " + submitWiringBackendErrorRequestID,
		"traceId:",
		"work count:",
	} {
		if strings.Contains(output, leaked) {
			t.Fatalf("submit batch output must not contain %q:\n%s", leaked, output)
		}
	}
}

func submitWiringFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cli-submit-wiring",
		"workTypes": []map[string]any{
			{
				"name": submitWiringInlineWorkType,
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
				"inputs":    []map[string]string{{"workType": submitWiringInlineWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": submitWiringInlineWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": submitWiringInlineWorkType, "state": "failed"}},
			},
		},
	}
}

func buildYouCLIBinary(t *testing.T) string {
	t.Helper()

	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, string(output))
	}
	return binaryPath
}
