package catalog

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
	catalogForcedUnwindEnv       = "YOU_FUNCTIONAL_CATALOG_FORCED_UNWIND"
	catalogForcedUnwindReportEnv = "YOU_FUNCTIONAL_CATALOG_FORCED_UNWIND_REPORT"
)

type catalogForcedUnwindReport struct {
	ProcessCloseError string `json:"process_close_error,omitempty"`
	ListenerURL       string `json:"listener_url,omitempty"`
	ListenerClosed    bool   `json:"listener_closed"`
}

func failCatalogForcedUnwindAfterAssertion(t *testing.T) {
	t.Helper()
	if os.Getenv(catalogForcedUnwindEnv) == "1" {
		t.Fatal("intentional packaged catalog forced-unwind characterization failure")
	}
}

func writeCatalogForcedUnwindReport(
	fixture *catalogSharedProcessFixture,
	closeErr error,
) error {
	path := strings.TrimSpace(os.Getenv(catalogForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := catalogForcedUnwindReport{}
	if closeErr != nil {
		report.ProcessCloseError = closeErr.Error()
	}
	if fixture != nil && fixture.apiRouter != nil {
		server := fixture.apiRouter.current()
		if server != nil {
			if baseURL, ok := server.BaseURL(); ok {
				report.ListenerURL = baseURL
				report.ListenerClosed = catalogListenerClosed(baseURL)
			}
		}
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

func catalogListenerClosed(baseURL string) bool {
	return catalogListenerError(baseURL) == nil
}

func catalogListenerError(baseURL string) error {
	if strings.TrimSpace(baseURL) == "" {
		return fmt.Errorf("catalog API listener URL is empty")
	}
	client := http.Client{Timeout: time.Second}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	deadline := time.Now().Add(2 * time.Second)
	lastStatus := 0
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint)
		if err != nil {
			return nil
		}
		lastStatus = response.StatusCode
		_ = response.Body.Close()
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("catalog API listener %s remained reachable with status %d", baseURL, lastStatus)
}
