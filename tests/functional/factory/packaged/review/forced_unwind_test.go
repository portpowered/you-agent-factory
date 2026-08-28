package review

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	packagedReviewForcedUnwindEnv       = "YOU_FUNCTIONAL_PACKAGED_REVIEW_FORCED_UNWIND"
	packagedReviewForcedUnwindReportEnv = "YOU_FUNCTIONAL_PACKAGED_REVIEW_FORCED_UNWIND_REPORT"
)

type packagedReviewForcedUnwindReport struct {
	ProcessCloseError string `json:"process_close_error,omitempty"`
	ListenerClosed    bool   `json:"listener_closed"`
	RootDir           string `json:"root_dir,omitempty"`
	RootAbsent        bool   `json:"root_absent"`
	SelectorsZero     bool   `json:"selectors_zero"`
	CensusClean       bool   `json:"census_clean"`
}

func failPackagedReviewForcedUnwindAfterAssertion(t *testing.T) {
	t.Helper()
	if os.Getenv(packagedReviewForcedUnwindEnv) == "1" {
		t.Fatal("intentional packaged Review forced-unwind characterization failure")
	}
}

func writePackagedReviewForcedUnwindReport(
	fixture *packagedReviewSharedFixture,
	closeErr error,
) error {
	path := strings.TrimSpace(os.Getenv(packagedReviewForcedUnwindReportEnv))
	if path == "" {
		return nil
	}
	report := packagedReviewForcedUnwindReport{}
	if closeErr != nil {
		report.ProcessCloseError = closeErr.Error()
	}
	if fixture != nil {
		report.ListenerClosed = assertPackagedReviewPortClosed(fixture.baseURL) == nil
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
