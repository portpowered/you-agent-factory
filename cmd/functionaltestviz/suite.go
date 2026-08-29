package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type suiteExitError struct {
	code int
	err  error
}

func (e suiteExitError) Error() string { return e.err.Error() }
func (e suiteExitError) Unwrap() error { return e.err }

type consoleCoverageSummary struct {
	Packages []struct {
		Package         string  `json:"package"`
		CoveragePercent float64 `json:"coveragePercent"`
	} `json:"packages"`
}

type consoleTimingSummary struct {
	Packages []struct {
		Package string  `json:"package"`
		Seconds float64 `json:"seconds"`
	} `json:"packages"`
}

func runFunctionalSuite(cfg config, stdout, stderr io.Writer) error {
	if err := validateSuiteConfig(cfg); err != nil {
		return err
	}
	ownedPaths := []string{
		cfg.logPath,
		functionalDiagnosticStatusPath(cfg),
		cfg.timingSummaryPath,
		cfg.coverageSummaryPath,
		cfg.profilePath,
		cfg.coverageBuildDiagnosticsPath,
		cfg.outputPath,
		cfg.verdictPath,
		cfg.exitCodePath,
	}
	if err := resetSuiteArtifacts(ownedPaths); err != nil {
		return err
	}
	logFile, err := os.OpenFile(cfg.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open functional test command log %s: %w", cfg.logPath, err)
	}
	defer logFile.Close()
	_, _ = fmt.Fprintf(
		logFile,
		"Functional tier: name=%s trigger=%s short=%t budget=%s selection=subtractive quarantine=%s jobs=%d\n",
		cfg.tier,
		cfg.trigger,
		cfg.short,
		cfg.budget,
		cfg.quarantinePath,
		cfg.jobs,
	)

	status, runErr := runLoggedCommand(cfg.goBinary, []string{"run", "./cmd/functionalboundarycheck"}, logFile)
	coverageStatus := 0
	if runErr == nil {
		status, runErr = runLoggedCommand(cfg.goBinary, coverageCommandArguments(cfg), logFile)
		coverageStatus = status
		if cfg.exitCodePath != "" {
			if err := writeTextFile(cfg.exitCodePath, strconv.Itoa(status)+"\n"); err != nil {
				return err
			}
			if status == 1 {
				runErr = nil
			}
		}
	}
	if runErr == nil {
		status, runErr = runLoggedCommand(cfg.goBinary, []string{
			"run", "./cmd/functionaltestviz",
			"-coverage-summary", cfg.coverageSummaryPath,
			"-timing-summary", cfg.timingSummaryPath,
			"-output", cfg.outputPath,
		}, logFile)
	}

	if err := renderConsoleSummary(cfg.coverageSummaryPath, cfg.timingSummaryPath, stdout); err != nil {
		_, _ = fmt.Fprintf(logFile, "Functional console summary unavailable: %v\n", err)
		if runErr == nil {
			status, runErr = 1, err
		}
	}
	if err := publishFunctionalJobSummary(cfg); err != nil {
		_, _ = fmt.Fprintf(logFile, "Functional job summary unavailable: %v\n", err)
	}
	if runErr != nil {
		return suiteExitError{code: normalizedExitCode(status), err: runErr}
	}
	if cfg.exitCodePath != "" {
		if err := writeCompactVerdict(cfg.logPath, cfg.verdictPath, coverageStatus); err != nil {
			return suiteExitError{code: 1, err: err}
		}
	}
	return nil
}

func publishFunctionalJobSummary(cfg config) error {
	summaryPath := strings.TrimSpace(os.Getenv("GITHUB_STEP_SUMMARY"))
	if summaryPath == "" {
		return nil
	}

	var summary bytes.Buffer
	markdown, markdownErr := os.ReadFile(cfg.outputPath)
	if markdownErr == nil {
		summary.Write(markdown)
		if len(markdown) > 0 && markdown[len(markdown)-1] != '\n' {
			summary.WriteByte('\n')
		}
	} else if !errors.Is(markdownErr, os.ErrNotExist) {
		return fmt.Errorf("read functional test Markdown: %w", markdownErr)
	}

	diagnosticStatusPath := functionalDiagnosticStatusPath(cfg)
	diagnosticStatus, diagnosticErr := os.ReadFile(diagnosticStatusPath)
	if diagnosticErr != nil && !errors.Is(diagnosticErr, os.ErrNotExist) {
		return fmt.Errorf("read functional diagnostic status: %w", diagnosticErr)
	}
	if markdownErr != nil && diagnosticErr != nil && !anyFileExists(cfg.timingSummaryPath, cfg.coverageSummaryPath, cfg.profilePath) {
		return nil
	}
	if markdownErr != nil {
		summary.WriteString("## Functional test diagnostics\n\n")
		summary.WriteString("The Markdown catalog is unavailable because the tier ended before complete coverage and timing inputs were rendered.\n")
	}
	if diagnosticErr == nil || markdownErr != nil {
		summary.WriteString("\n## Functional diagnostics availability\n\n")
		if diagnosticErr == nil {
			summary.Write(diagnosticStatus)
			if len(diagnosticStatus) > 0 && diagnosticStatus[len(diagnosticStatus)-1] != '\n' {
				summary.WriteByte('\n')
			}
		} else {
			writeDiagnosticAvailability(&summary, "timing", cfg.timingSummaryPath, "incomplete-or-partial", "no timing snapshot was published before the lane ended")
			writeDiagnosticAvailability(&summary, "coverage-summary", cfg.coverageSummaryPath, "incomplete-or-partial", "no trustworthy complete or partial coverage measurement was published")
			writeDiagnosticAvailability(&summary, "coverage-profile", cfg.profilePath, "raw-profile", "go test did not flush a coverage profile")
			writeDiagnosticAvailability(&summary, "markdown", cfg.outputPath, "", "the catalog renderer did not receive complete inputs")
		}
	}
	if summary.Len() == 0 {
		return nil
	}
	file, err := os.OpenFile(summaryPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open GitHub step summary: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(summary.Bytes()); err != nil {
		return fmt.Errorf("append GitHub step summary: %w", err)
	}
	return nil
}

func functionalDiagnosticStatusPath(cfg config) string {
	return filepath.Join(filepath.Dir(cfg.logPath), "diagnostic-status.txt")
}

func anyFileExists(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func writeDiagnosticAvailability(output io.Writer, name, path, status, missingReason string) {
	if _, err := os.Stat(path); err == nil {
		if status == "" {
			_, _ = fmt.Fprintf(output, "available: name=%s path=%s\n", name, path)
		} else {
			_, _ = fmt.Fprintf(output, "available: name=%s path=%s status=%s\n", name, path, status)
		}
		return
	}
	_, _ = fmt.Fprintf(output, "missing: name=%s path=%s reason=%s\n", name, path, missingReason)
}

func validateSuiteConfig(cfg config) error {
	required := map[string]string{
		"coverage-summary": cfg.coverageSummaryPath,
		"timing-summary":   cfg.timingSummaryPath,
		"output":           cfg.outputPath,
		"log":              cfg.logPath,
		"profile":          cfg.profilePath,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s path is required with -run-suite", name)
		}
	}
	if cfg.jobs < 1 {
		return fmt.Errorf("jobs must be positive")
	}
	return nil
}

func resetSuiteArtifacts(paths []string) error {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale functional test artifact %s: %w", path, err)
		}
	}
	return os.MkdirAll(filepath.Dir(paths[0]), 0o755)
}

func coverageCommandArguments(cfg config) []string {
	args := []string{
		"run", "./cmd/gocoveragecheck",
		"-suite", "functional",
		"-stream",
		"-jobs", strconv.Itoa(cfg.jobs),
		"-min", strconv.FormatFloat(cfg.minimumCoverage, 'f', -1, 64),
		"-package-manifest", cfg.packageManifestPath,
		"-package-floor-policy", cfg.packageFloorPolicy,
		"-functional-quarantine", cfg.quarantinePath,
		"-timeout", cfg.testTimeout,
		"-profile", cfg.profilePath,
		"-json-output", cfg.coverageSummaryPath,
		"-timing-output", cfg.timingSummaryPath,
	}
	if strings.TrimSpace(cfg.coverageBuildDiagnosticsPath) != "" {
		args = append(args, "-coverage-build-diagnostics-output", cfg.coverageBuildDiagnosticsPath)
	}
	if !cfg.short {
		args = append(args, "-short=false")
	}
	return args
}

func runLoggedCommand(name string, args []string, log io.Writer) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = log
	cmd.Stderr = log
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), fmt.Errorf("%s exited with status %d", strings.Join(append([]string{name}, args...), " "), exitErr.ExitCode())
	}
	return 1, fmt.Errorf("run %s: %w", name, err)
}

func renderConsoleSummary(coveragePath, timingPath string, output io.Writer) error {
	var coverage consoleCoverageSummary
	if err := decodeJSONFile(coveragePath, &coverage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var timing consoleTimingSummary
	if err := decodeJSONFile(timingPath, &timing); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	slices.SortFunc(coverage.Packages, func(left, right struct {
		Package         string  `json:"package"`
		CoveragePercent float64 `json:"coveragePercent"`
	}) int {
		return strings.Compare(left.Package, right.Package)
	})
	slices.SortFunc(timing.Packages, func(left, right struct {
		Package string  `json:"package"`
		Seconds float64 `json:"seconds"`
	}) int {
		return strings.Compare(left.Package, right.Package)
	})

	wroteCoverage := false
	for _, pkg := range coverage.Packages {
		if !strings.Contains(pkg.Package, "/pkg/") {
			continue
		}
		if !wroteCoverage {
			_, _ = fmt.Fprintln(output, "Functional coverage for pkg/:")
			wroteCoverage = true
		}
		_, _ = fmt.Fprintf(output, "%s %.1f%%\n", displayImportPath(pkg.Package, "/pkg/"), pkg.CoveragePercent)
	}
	wroteTiming := false
	for _, pkg := range timing.Packages {
		if !strings.Contains(pkg.Package, "/tests/functional/") {
			continue
		}
		if !wroteTiming {
			if wroteCoverage {
				_, _ = fmt.Fprintln(output)
			}
			_, _ = fmt.Fprintln(output, "Functional package latencies:")
			wroteTiming = true
		}
		_, _ = fmt.Fprintf(output, "%s %.3fs\n", displayImportPath(pkg.Package, "/tests/functional/"), pkg.Seconds)
	}
	return nil
}

func decodeJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func displayImportPath(importPath, marker string) string {
	index := strings.Index(importPath, marker)
	if index < 0 {
		return importPath
	}
	return importPath[index+1:]
}

func writeCompactVerdict(logPath, verdictPath string, exitCode int) error {
	logFile, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("open functional coverage log: %w", err)
	}
	defer logFile.Close()
	lines := make([]string, 0)
	scanner := bufio.NewScanner(logFile)
	for scanner.Scan() {
		line := scanner.Text()
		if compactVerdictLine(line) {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read functional coverage log: %w", err)
	}
	if len(lines) == 0 {
		return fmt.Errorf("functional coverage verdict extract is empty")
	}
	outcome := "green"
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "COVERAGE FLOOR POLICY: advisory") && strings.Contains(joined, "package coverage regression:") {
		outcome = "advisory"
	} else if exitCode != 0 && strings.Contains(joined, "package floors were NOT checked") {
		outcome = "test-failure"
	} else if exitCode != 0 {
		outcome = "coverage-gate-failure"
	}
	return writeTextFile(verdictPath, fmt.Sprintf("Functional coverage outcome: %s\n%s\n", outcome, joined))
}

func compactVerdictLine(line string) bool {
	return line == "!!! COVERAGE FLOOR POLICY: advisory !!!" ||
		strings.HasPrefix(line, "Package floors and missing-manifest findings are report-only") ||
		strings.HasPrefix(line, "Set -package-floor-policy=blocking") ||
		strings.HasPrefix(line, "Functional suite inventory:") ||
		strings.HasPrefix(line, "total: (statements)") ||
		strings.HasPrefix(line, "Functional package coverage verdict:") ||
		strings.HasPrefix(line, "  floor violation:") ||
		line == "  floor violations: none" ||
		strings.HasPrefix(line, "  package=") ||
		strings.HasPrefix(line, "  tally:") ||
		strings.HasPrefix(line, "package coverage regression:") ||
		strings.HasPrefix(line, "coverage manifest missing entry:") ||
		strings.HasPrefix(line, "coverage not evaluated:") ||
		strings.HasPrefix(line, "Go coverage ") ||
		strings.HasPrefix(line, "go coverage ")
}

func writeTextFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create artifact directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func normalizedExitCode(code int) int {
	if code < 1 || code > 255 {
		return 1
	}
	return code
}
