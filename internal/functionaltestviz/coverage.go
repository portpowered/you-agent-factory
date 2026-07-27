package functionaltestviz

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// CoverageSummary mirrors the machine-readable coverage-summary JSON owned by
// cmd/gocoveragecheck. Downstream visualizers decode this artifact instead of
// re-parsing coverage profiles.
type CoverageSummary struct {
	CoveredStatements    int                   `json:"coveredStatements"`
	MeasurableStatements int                   `json:"measurableStatements"`
	CoveragePercent      float64               `json:"coveragePercent"`
	Packages             []PackageCoverage     `json:"packages"`
}

// PackageCoverage reports one measured production package, including the
// package-gate floor or measurement exception used for that run.
type PackageCoverage struct {
	Package              string                `json:"package"`
	CoveredStatements    int                   `json:"coveredStatements"`
	MeasurableStatements int                   `json:"measurableStatements"`
	CoveragePercent      float64               `json:"coveragePercent"`
	PackageFloor         *float64              `json:"packageFloor"`
	MeasurementException *MeasurementException `json:"measurementException"`
}

// MeasurementException mirrors the structured exception object emitted by
// gocoveragecheck when a package is ungated for measurement reasons.
type MeasurementException struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification"`
	Owner         string `json:"owner"`
	Deadline      string `json:"deadline"`
	RemovalGate   string `json:"removalGate"`
}

// LoadCoverageSummary reads and decodes a coverage-summary JSON file.
// Missing paths and malformed documents fail with actionable errors.
func LoadCoverageSummary(path string) (CoverageSummary, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return CoverageSummary{}, fmt.Errorf("coverage-summary path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CoverageSummary{}, fmt.Errorf("coverage-summary file not found: %s", path)
		}
		return CoverageSummary{}, fmt.Errorf("read coverage-summary %s: %w", path, err)
	}
	summary, err := DecodeCoverageSummary(data)
	if err != nil {
		return CoverageSummary{}, fmt.Errorf("decode coverage-summary %s: %w", path, err)
	}
	return summary, nil
}

// DecodeCoverageSummary decodes coverage-summary JSON bytes.
func DecodeCoverageSummary(data []byte) (CoverageSummary, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return CoverageSummary{}, fmt.Errorf("coverage-summary JSON is empty")
	}
	var summary CoverageSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return CoverageSummary{}, fmt.Errorf("invalid coverage-summary JSON: %w", err)
	}
	if err := validateCoverageSummary(summary); err != nil {
		return CoverageSummary{}, err
	}
	return summary, nil
}

func validateCoverageSummary(summary CoverageSummary) error {
	if summary.Packages == nil {
		return fmt.Errorf("coverage-summary JSON missing required packages array")
	}
	if summary.MeasurableStatements < 0 || summary.CoveredStatements < 0 {
		return fmt.Errorf("coverage-summary JSON has negative statement totals")
	}
	if summary.CoveredStatements > summary.MeasurableStatements {
		return fmt.Errorf(
			"coverage-summary JSON coveredStatements (%d) exceeds measurableStatements (%d)",
			summary.CoveredStatements,
			summary.MeasurableStatements,
		)
	}
	for i, pkg := range summary.Packages {
		if strings.TrimSpace(pkg.Package) == "" {
			return fmt.Errorf("coverage-summary JSON packages[%d] missing package path", i)
		}
		if pkg.MeasurableStatements < 0 || pkg.CoveredStatements < 0 {
			return fmt.Errorf("coverage-summary JSON packages[%d] (%s) has negative statement totals", i, pkg.Package)
		}
		if pkg.CoveredStatements > pkg.MeasurableStatements {
			return fmt.Errorf(
				"coverage-summary JSON packages[%d] (%s) coveredStatements (%d) exceeds measurableStatements (%d)",
				i,
				pkg.Package,
				pkg.CoveredStatements,
				pkg.MeasurableStatements,
			)
		}
	}
	return nil
}
