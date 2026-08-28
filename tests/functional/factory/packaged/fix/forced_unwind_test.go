package fix

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	packagedFixForcedUnwindEnv       = "YOU_FUNCTIONAL_PACKAGED_FIX_FORCED_UNWIND"
	packagedFixForcedUnwindReportEnv = "YOU_FUNCTIONAL_PACKAGED_FIX_FORCED_UNWIND_REPORT"
)

type packagedFixForcedUnwindReport struct {
	ProcessCloseError string `json:"process_close_error,omitempty"`
	ListenerClosed    bool   `json:"listener_closed"`
	RootDir           string `json:"root_dir,omitempty"`
	RootAbsent        bool   `json:"root_absent"`
	SelectorsZero     bool   `json:"selectors_zero"`
	CensusClean       bool   `json:"census_clean"`
}

func failPackagedFixForcedUnwindAfterAssertion(t *testing.T) {
	t.Helper()
	if os.Getenv(packagedFixForcedUnwindEnv) == "1" {
		t.Fatal("intentional packaged Fix forced-unwind characterization failure")
	}
}

func writePackagedFixForcedUnwindReport(
	fixture *packagedFixSharedFixture,
	closeErr error,
) error {
	path := strings.TrimSpace(os.Getenv(packagedFixForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := packagedFixForcedUnwindReport{}
	if closeErr != nil {
		report.ProcessCloseError = closeErr.Error()
	}
	if fixture != nil {
		report.ListenerClosed = assertPackagedFixPortClosed(fixture.baseURL) == nil
		report.RootDir = fixture.rootDir
		report.RootAbsent = pathAbsent(fixture.rootDir)
		report.SelectorsZero = fixture.providerRunner.registeredCount() == 0
		report.CensusClean = fixture.census == nil || fixture.census.closedError() == nil
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
