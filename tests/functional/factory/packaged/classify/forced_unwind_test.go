package classify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	classifyForcedUnwindEnv       = "YOU_FUNCTIONAL_CLASSIFY_FORCED_UNWIND"
	classifyForcedUnwindReportEnv = "YOU_FUNCTIONAL_CLASSIFY_FORCED_UNWIND_REPORT"
)

type classifyForcedUnwindReport struct {
	ProcessCloseError string `json:"process_close_error,omitempty"`
	ListenerURL       string `json:"listener_url,omitempty"`
	ListenerClosed    bool   `json:"listener_closed"`
	RootDir           string `json:"root_dir,omitempty"`
	RootAbsent        bool   `json:"root_absent"`
}

func failClassifyForcedUnwindAfterAssertion(t *testing.T) {
	t.Helper()
	if os.Getenv(classifyForcedUnwindEnv) == "1" {
		t.Fatal("intentional packaged classify forced-unwind characterization failure")
	}
}

func writeClassifyForcedUnwindReport(
	fixture *classifySharedFixture,
	closeErr error,
) error {
	path := strings.TrimSpace(os.Getenv(classifyForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := classifyForcedUnwindReport{}
	if closeErr != nil {
		report.ProcessCloseError = closeErr.Error()
	}
	if fixture != nil {
		report.ListenerURL = fixture.baseURL
		report.ListenerClosed = classifyListenerClosed(fixture.listenerDone)
		report.RootDir = fixture.rootDir
		report.RootAbsent = classifyPathAbsent(fixture.rootDir)
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func classifyListenerClosed(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func classifyListenerError(done <-chan struct{}) error {
	if !classifyListenerClosed(done) {
		return errors.New("classify API listener did not report shutdown")
	}
	return nil
}

func classifyPathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func assertClassifySessionAbsent(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(endpoint)
	if err != nil {
		t.Errorf("GET deleted Factory Session %q: %v", sessionID, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return
	}
	body, _ := io.ReadAll(response.Body)
	t.Errorf(
		"GET deleted Factory Session %q status = %d, want 404: %s",
		sessionID,
		response.StatusCode,
		strings.TrimSpace(string(body)),
	)
}

func cleanupClassifyScenarioRoot(t testing.TB, rootDir string) {
	t.Helper()
	if err := os.RemoveAll(rootDir); err != nil {
		t.Errorf("remove classify scenario root %q: %v", rootDir, err)
		return
	}
	if !classifyPathAbsent(rootDir) {
		t.Errorf("classify scenario root %q remains after cleanup", rootDir)
	}
}
