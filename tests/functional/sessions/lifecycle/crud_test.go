package lifecycle_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactorySessionCreateListShowDelete proves the public CLI Factory Session
// boundary supports a full create → list → show → delete lifecycle: a newly
// created session returns a stable public session ID, appears in session list
// and session show with matching folder identity, and session delete removes it
// so subsequent list/show no longer treat it as an open session.
func TestFactorySessionCreateListShowDelete(t *testing.T) {
	primaryFactoryDir := support.ScaffoldFactory(t, sessionLifecycleCRUDFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     primaryFactoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	baseURL := server.URL()
	serverPort := portFromServerURL(t, baseURL)
	binaryPath := buildYouCLIBinary(t)

	newFactoryDir := filepath.Join(t.TempDir(), "session-lifecycle-crud-factory")
	if err := os.Mkdir(newFactoryDir, 0o755); err != nil {
		t.Fatalf("create factory directory: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	createOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "create",
		"--dir", newFactoryDir,
		"--init-new-factory",
	)
	if err != nil {
		t.Fatalf("you session create: %v\noutput:\n%s", err, createOut)
	}

	var created factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(bytesTrimSpace(createOut), &created); err != nil {
		t.Fatalf("decode session create JSON: %v\noutput:\n%s", err, createOut)
	}
	if created.Session == nil || strings.TrimSpace(created.Session.Id) == "" {
		t.Fatalf("session create response missing session id: %#v", created)
	}
	if created.Session.FolderPath != newFactoryDir {
		t.Fatalf("session folder path = %q, want %q", created.Session.FolderPath, newFactoryDir)
	}
	sessionID := created.Session.Id

	listOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL, "session", "list")
	if err != nil {
		t.Fatalf("you session list: %v\noutput:\n%s", err, listOut)
	}
	listHuman := string(listOut)
	for _, marker := range []string{sessionID, newFactoryDir} {
		if !strings.Contains(listHuman, marker) {
			t.Fatalf("session list output missing %q:\n%s", marker, listHuman)
		}
	}

	listJSONOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "list",
	)
	if err != nil {
		t.Fatalf("you session list --json: %v\noutput:\n%s", err, listJSONOut)
	}
	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytesTrimSpace(listJSONOut), &listed); err != nil {
		t.Fatalf("decode session list JSON: %v\noutput:\n%s", err, listJSONOut)
	}
	if !sessionListContains(listed.Sessions, sessionID, newFactoryDir) {
		t.Fatalf("session list JSON missing created session %q at %q: %#v", sessionID, newFactoryDir, listed.Sessions)
	}

	showOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionID,
	)
	if err != nil {
		t.Fatalf("you session show: %v\noutput:\n%s", err, showOut)
	}
	var shown factoryapi.FactorySession
	if err := json.Unmarshal(bytesTrimSpace(showOut), &shown); err != nil {
		t.Fatalf("decode session show JSON: %v\noutput:\n%s", err, showOut)
	}
	if shown.Id != sessionID {
		t.Fatalf("session show id = %q, want %q", shown.Id, sessionID)
	}
	if shown.FolderPath != newFactoryDir {
		t.Fatalf("session show folder path = %q, want %q", shown.FolderPath, newFactoryDir)
	}
	if shown.Runtime.Status == "" {
		t.Fatalf("session show missing runtime status markers: %#v", shown)
	}

	deleteOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, "",
		"--json",
		"session", "delete", sessionID,
		"--port", fmt.Sprintf("%d", serverPort),
	)
	if err != nil {
		t.Fatalf("you session delete: %v\noutput:\n%s", err, deleteOut)
	}
	var deleted struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(bytesTrimSpace(deleteOut), &deleted); err != nil {
		t.Fatalf("decode session delete JSON: %v\noutput:\n%s", err, deleteOut)
	}
	if deleted.SessionID != sessionID {
		t.Fatalf("session delete confirmation = %q, want %q", deleted.SessionID, sessionID)
	}

	showAfterDeleteOut, err := runYouCLI(ctx, binaryPath, primaryFactoryDir, baseURL,
		"--json",
		"session", "show", sessionID,
	)
	if err == nil {
		t.Fatalf("you session show after delete unexpectedly succeeded:\n%s", showAfterDeleteOut)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("you session show after delete error = %v, want *exec.ExitError", err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("you session show after delete exit code = 0, want non-zero")
	}
	showAfterDelete := string(showAfterDeleteOut)
	if !strings.Contains(showAfterDelete, "not found") {
		t.Fatalf("session show after delete missing not-found diagnostic:\n%s", showAfterDelete)
	}
	if strings.Contains(showAfterDelete, sessionID) && strings.Contains(showAfterDelete, `"id"`) {
		t.Fatalf("session show after delete must not emit a success session payload:\n%s", showAfterDelete)
	}
}

func sessionLifecycleCRUDFactoryConfig() map[string]any {
	return map[string]any{
		"name": "session-lifecycle-crud",
		"workTypes": []map[string]any{
			{
				"name": "task",
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
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
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

func runYouCLI(
	ctx context.Context,
	binaryPath string,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	cmdArgs := []string{}
	if strings.TrimSpace(serverURL) != "" {
		cmdArgs = append(cmdArgs, "--server", serverURL)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, binaryPath, cmdArgs...)
	cmd.Dir = workingDir
	return cmd.CombinedOutput()
}

func portFromServerURL(t *testing.T, serverURL string) int {
	t.Helper()

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse server URL %q: %v", serverURL, err)
	}
	port := parsed.Port()
	if port == "" {
		t.Fatalf("server URL %q missing port", serverURL)
	}
	var portNumber int
	if _, err := fmt.Sscanf(port, "%d", &portNumber); err != nil {
		t.Fatalf("parse server port %q: %v", port, err)
	}
	return portNumber
}

func sessionListContains(
	sessions []factoryapi.FactorySessionSummary,
	sessionID string,
	folderPath string,
) bool {
	for _, session := range sessions {
		if session.Id == sessionID && session.FolderPath == folderPath {
			return true
		}
	}
	return false
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
