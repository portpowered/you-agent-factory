package functionaltestviz

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

// FunctionalTimingSummaryVersion is the schema version this package decodes.
// It must match cmd/gocoveragecheck's functionalTimingSummaryVersion.
const FunctionalTimingSummaryVersion = 1

const (
	timingOutcomePass = "pass"
	timingOutcomeFail = "fail"
	timingOutcomeSkip = "skip"
)

// FunctionalPackageTiming is one functional-test package's elapsed duration
// and terminal outcome from a single go test -json run.
type FunctionalPackageTiming struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
	Outcome string  `json:"outcome"`
}

// FunctionalTimingSummary mirrors the machine-readable functional-timing
// summary JSON owned by cmd/gocoveragecheck. WallSeconds is measured around
// the single go test invocation; PackageElapsedSecondsSum is the sum of
// per-package elapsed durations and can exceed WallSeconds because packages
// run concurrently. Complete is false whenever gocoveragecheck could not
// trust every package's captured timing, so downstream rendering must label
// the summary as an incomplete diagnostic rather than a successful report.
type FunctionalTimingSummary struct {
	Version                  int                       `json:"version"`
	Complete                 bool                      `json:"complete"`
	WallSeconds              float64                   `json:"wallSeconds"`
	PackageElapsedSecondsSum float64                   `json:"packageElapsedSecondsSum"`
	PackageCount             int                       `json:"packageCount"`
	Packages                 []FunctionalPackageTiming `json:"packages"`
}

// LoadFunctionalTimingSummary reads and decodes a functional-timing-summary
// JSON file. Missing paths and malformed documents fail with actionable
// errors; a document with complete=false decodes successfully so its partial
// diagnostics stay inspectable.
func LoadFunctionalTimingSummary(path string) (FunctionalTimingSummary, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return FunctionalTimingSummary{}, fmt.Errorf("functional-timing-summary path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FunctionalTimingSummary{}, fmt.Errorf("functional-timing-summary file not found: %s", path)
		}
		return FunctionalTimingSummary{}, fmt.Errorf("read functional-timing-summary %s: %w", path, err)
	}
	summary, err := DecodeFunctionalTimingSummary(data)
	if err != nil {
		return FunctionalTimingSummary{}, fmt.Errorf("decode functional-timing-summary %s: %w", path, err)
	}
	return summary, nil
}

// DecodeFunctionalTimingSummary decodes functional-timing-summary JSON bytes.
// It fails closed on structurally corrupt documents (malformed JSON,
// unsupported schema version, negative/non-finite durations, duplicate or
// unidentified packages, or a packageCount that disagrees with the packages
// array) but accepts complete=false as a legitimate partial-capture state.
func DecodeFunctionalTimingSummary(data []byte) (FunctionalTimingSummary, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return FunctionalTimingSummary{}, fmt.Errorf("functional-timing-summary JSON is empty")
	}
	var summary FunctionalTimingSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return FunctionalTimingSummary{}, fmt.Errorf("invalid functional-timing-summary JSON: %w", err)
	}
	if err := validateFunctionalTimingSummary(summary); err != nil {
		return FunctionalTimingSummary{}, err
	}
	return summary, nil
}

func validateFunctionalTimingSummary(summary FunctionalTimingSummary) error {
	if summary.Version != FunctionalTimingSummaryVersion {
		return fmt.Errorf(
			"unsupported functional-timing-summary version %d (want %d)",
			summary.Version,
			FunctionalTimingSummaryVersion,
		)
	}
	if err := validateTimingSeconds("wallSeconds", summary.WallSeconds); err != nil {
		return err
	}
	if err := validateTimingSeconds("packageElapsedSecondsSum", summary.PackageElapsedSecondsSum); err != nil {
		return err
	}
	if summary.PackageCount != len(summary.Packages) {
		return fmt.Errorf(
			"functional-timing-summary JSON packageCount (%d) disagrees with packages array length (%d)",
			summary.PackageCount,
			len(summary.Packages),
		)
	}

	seen := make(map[string]struct{}, len(summary.Packages))
	for i, pkg := range summary.Packages {
		if strings.TrimSpace(pkg.Package) == "" {
			return fmt.Errorf("functional-timing-summary JSON packages[%d] missing package identity", i)
		}
		if _, exists := seen[pkg.Package]; exists {
			return fmt.Errorf("functional-timing-summary JSON packages[%d] (%s) is a duplicate package entry", i, pkg.Package)
		}
		seen[pkg.Package] = struct{}{}
		if err := validateTimingSeconds(fmt.Sprintf("packages[%d] (%s) seconds", i, pkg.Package), pkg.Seconds); err != nil {
			return err
		}
		switch pkg.Outcome {
		case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		default:
			return fmt.Errorf("functional-timing-summary JSON packages[%d] (%s) has invalid outcome %q", i, pkg.Package, pkg.Outcome)
		}
	}
	return nil
}

func validateTimingSeconds(label string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("functional-timing-summary JSON %s is non-finite", label)
	}
	if value < 0 {
		return fmt.Errorf("functional-timing-summary JSON %s is negative", label)
	}
	return nil
}
