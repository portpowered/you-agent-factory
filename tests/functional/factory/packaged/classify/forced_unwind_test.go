package classify

import (
	"encoding/json"
	"fmt"
	"net/http"
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
		report.ListenerClosed = classifyListenerClosed(fixture.baseURL)
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

func classifyListenerClosed(baseURL string) bool {
	return classifyListenerError(baseURL) == nil
}

func classifyListenerError(baseURL string) error {
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("classify API listener URL is empty")
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/status")
	if err != nil {
		return nil
	}
	_ = response.Body.Close()
	return fmt.Errorf("classify API listener %s remained reachable with status %d", baseURL, response.StatusCode)
}

func classifyPathAbsent(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}
