package restart_test

import (
	"context"
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

func waitForBoardDaemonReady(t *testing.T, daemon *boardPersistenceDaemon, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// The real child process exposes no parent readiness channel. This bounded
	// public /status observation is the unavoidable process-bound synchronization
	// for the isolated-daemon acceptance test.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-daemon.done:
			dumpBoardPersistenceDiagnostics(t, daemon)
			t.Fatalf("isolated you daemon exited before readiness: %v\nstdout=%s\nstderr=%s", daemon.waitError(), daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, daemon.baseURL+"/status", nil)
			if err != nil {
				t.Fatalf("build daemon readiness request: %v", err)
			}
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && status.RuntimeStatus != "" {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for isolated you daemon readiness at %s\nstdout=%s\nstderr=%s", daemon.baseURL, daemon.stdout.String(), daemon.stderr.String())
		}
	}
}

func dumpBoardPersistenceDiagnostics(t *testing.T, daemon *boardPersistenceDaemon) {
	t.Helper()
	t.Logf("daemon diagnostic paths: factory=%q home=%q record=%q", daemon.factoryDir, daemon.homeDir, daemon.recordPath)
	logsRoot := filepath.Join(daemon.homeDir, ".you-agent-factory", "logs")
	_ = filepath.WalkDir(logsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if len(contents) > 8192 {
			contents = contents[len(contents)-8192:]
		}
		t.Logf("daemon runtime log tail %q (%d bytes): %s", path, len(contents), contents)
		return nil
	})
}

func waitForBoardPersistenceSnapshot(t *testing.T, path, wantSessionID string, timeout time.Duration) []byte {
	t.Helper()
	// The durable snapshot is committed by the isolated daemon child, and the
	// parent has no synchronization channel for that filesystem write. Polling
	// the file is the only deterministic observation of the commit boundary;
	// the bounded timeout turns a failed child write into a useful test failure.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		contents, err := os.ReadFile(path)
		if err == nil && len(contents) > 0 {
			var snapshot struct {
				Session struct {
					SessionID string `json:"sessionId"`
				} `json:"session"`
			}
			if err := json.Unmarshal(contents, &snapshot); err == nil && snapshot.Session.SessionID == wantSessionID {
				return contents
			}
			lastErr = fmt.Errorf("durable snapshot was empty or session identity was not %q", wantSessionID)
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for valid durable snapshot %q: %v", path, lastErr)
		}
	}
}

func waitForBoardPersistenceLogMessage(
	t *testing.T,
	daemon *boardPersistenceDaemon,
	fragments []string,
	timeout time.Duration,
) {
	t.Helper()
	// The recovery warning is appended by the isolated child process after boot,
	// with no test-owned logging edge back to the parent. Polling the runtime log
	// is therefore the required process-boundary observation; the timeout keeps
	// a missing warning actionable without using a fixed sleep.
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		found := false
		_ = filepath.WalkDir(daemon.logDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for _, fragment := range fragments {
				if !strings.Contains(string(contents), fragment) {
					return nil
				}
			}
			found = true
			return filepath.SkipAll
		})
		if found {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for recovery warning in runtime logs under %q", daemon.logDir)
		}
	}
}

func waitForBoardStates(t *testing.T, baseURL string, want map[string]string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// Work has no process-parent notification, so state convergence is observed
	// through the public session list with a bounded ticker rather than a fixed
	// sleep. This is the process-bound wait required by this test only.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListWorkResponse
	var lastErr error
	for {
		listed, err := readBoardWorkList(t.Context(), baseURL)
		if err == nil {
			last = listed
			if boardStatesMatch(listed, want) {
				return listed
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Work states %#v; last list=%#v, last error=%v", want, last.Results, lastErr)
		}
	}
}

func readBoardWorkList(ctx context.Context, baseURL string) (factoryapi.ListWorkResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/factory-sessions/~default/work", nil)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListWorkResponse{}, fmt.Errorf("GET /work status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return listed, nil
}

func boardStatesMatch(listed factoryapi.ListWorkResponse, want map[string]string) bool {
	if len(listed.Results) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(listed.Results))
	for _, item := range listed.Results {
		workID := boardPersistenceStringPointerValue(item.WorkId)
		state := ""
		if item.State != nil {
			state = item.State.Name
		}
		if _, duplicate := seen[workID]; duplicate || want[workID] != state {
			return false
		}
		seen[workID] = struct{}{}
	}
	return len(seen) == len(want)
}
