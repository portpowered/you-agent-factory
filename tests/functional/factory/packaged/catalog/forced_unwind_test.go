package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
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
				report.ListenerClosed = fixture.apiRouter.listenerClosed()
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
