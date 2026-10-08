package restart_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The invoking build owns the CLI. This one cell crosses OS stderr, real
// quarantine storage and loopback HTTP; functional tests own the full matrix.
func TestUnreadableDefaultBoardStartsEmpty(t *testing.T) {
	t.Parallel()
	binary := requireRestartCLIArtifact(t)
	repo, home := t.TempDir(), t.TempDir()
	factory := filepath.Join(repo, "factory")
	if err := os.Rename(scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig()), factory); err != nil {
		t.Fatal(err)
	}
	writeBoardPersistenceAgentConfig(t, factory, "restart-blocker", "---\ntype: SCRIPT_WORKER\ncommand: unused-empty-board-worker\n---\n")
	snapshot := filepath.Join(repo, ".you-agent-factory", "durable-sessions", "~default.json")
	if err := os.MkdirAll(filepath.Dir(snapshot), 0700); err != nil {
		t.Fatal(err)
	}
	damaged := []byte(`{"secret":"fixture-private-prompt",`)
	if err := os.WriteFile(snapshot, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	// The only extra option is an isolated loopback listen address; there is no
	// selector, --debug, mock worker or explicit recording.
	daemon := startBoardPersistenceDaemonProcess(t, binary, factory, home, "", "")
	waitForBoardDaemonReady(t, daemon, 30*time.Second)
	condition := assertUnreadableBoardStatuses(t, daemon.baseURL, snapshot)
	works, err := readBoardWorkList(t.Context(), daemon.baseURL)
	if err != nil || len(works.Results) != 0 {
		t.Fatalf("empty Work list: %#v %v", works, err)
	}
	archive := condition.StartupRecovery.QuarantinedFile
	if !strings.HasPrefix(archive, snapshot+".unreadable.") {
		t.Fatal("quarantine path missing")
	}
	got, err := os.ReadFile(archive)
	if err != nil || !bytes.Equal(got, damaged) {
		t.Fatal("archive did not retain exact damaged bytes")
	}
	shutdownPlainBoard(t, daemon)
	want := fmt.Sprintf("Durable state %q quarantined as %q: INVALID_JSON. Started an empty board.\n", snapshot, archive)
	if strings.Count(daemon.stderr.String(), want) != 1 || strings.Contains(daemon.stdout.String(), "Started an empty board.") {
		t.Fatal("expected exactly one no-debug stderr recovery line")
	}
	if strings.Contains(daemon.stdout.String()+daemon.stderr.String(), "fixture-private-prompt") {
		t.Fatal("process output exposed damaged content")
	}
	assertUnreadableBoardLogsPrivate(t, daemon.logDir)
	t.Logf("prebuilt SHA256=%s argv=%q: ready empty HTTP, byte-preserving archive, safe one-line stderr, clean shutdown", restartCLIArtifact.SHA256, daemon.cmd.Args[1:])
}

func readUnreadableBoardStatus(t *testing.T, url string) factoryapi.StatusResponse {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || bytes.Contains(data, []byte("fixture-private-prompt")) {
		t.Fatal("status failed or exposed damaged content")
	}
	var status factoryapi.StatusResponse
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatal(err)
	}
	return status
}

func assertUnreadableBoardLogsPrivate(t *testing.T, root string, sentinels ...string) {
	sentinels = append(sentinels, "fixture-private-prompt")
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, sentinel := range sentinels {
			if bytes.Contains(data, []byte(sentinel)) {
				return fmt.Errorf("runtime log exposed private content")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertUnreadableBoardStatuses(t *testing.T, baseURL, snapshot string) *factoryapi.StatusResponse {
	t.Helper()
	var condition *factoryapi.StatusResponse
	for _, route := range []string{"/status", "/factory-sessions/~default/status"} {
		status := readUnreadableBoardStatus(t, baseURL+route)
		if status.TotalTokens != 0 || status.StartupRecovery == nil || status.StartupRecovery.Code != "DURABLE_STATE_QUARANTINED" || status.StartupRecovery.Cause != "INVALID_JSON" || status.StartupRecovery.File != snapshot {
			t.Fatalf("%s did not report healthy empty recovery: %#v", route, status)
		}
		if condition != nil && *condition.StartupRecovery != *status.StartupRecovery {
			t.Fatal("current and session diagnostics differ")
		}
		condition = &status
	}
	return condition
}
