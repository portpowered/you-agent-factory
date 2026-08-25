package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// renderFunctionalFailureDetail reduces one captured go test -json stream to
// the packages that actually failed. Successful packages can share the same
// invocation as a failing package, so replaying the whole stream would put
// their debug output back into the failure diagnostic.
func renderFunctionalFailureDetail(jsonOutput string) string {
	records := parseFunctionalFailureRecords(jsonOutput)
	failedPackages := make(map[string]struct{})
	failedTests := make(map[string]map[string]struct{})
	for _, record := range records {
		if record.event.Action != timingOutcomeFail || record.event.Package == "" {
			continue
		}
		failedPackages[record.event.Package] = struct{}{}
		if record.event.Test == "" {
			continue
		}
		if failedTests[record.event.Package] == nil {
			failedTests[record.event.Package] = make(map[string]struct{})
		}
		failedTests[record.event.Package][record.event.Test] = struct{}{}
	}

	if len(failedPackages) == 0 {
		return renderFunctionalFailureFallback(jsonOutput)
	}

	packages := make([]string, 0, len(failedPackages))
	for packageName := range failedPackages {
		packages = append(packages, packageName)
	}
	sort.Strings(packages)

	var detail strings.Builder
	for _, packageName := range packages {
		testNames := sortedFunctionalFailureTests(failedTests[packageName])
		reason := firstFunctionalFailureReason(records, packageName, failedTests[packageName])
		fmt.Fprintf(&detail, "functional test failure: package=%s", packageName)
		if len(testNames) > 0 {
			fmt.Fprintf(&detail, " test=%s", strings.Join(testNames, ","))
		}
		if reason != "" {
			fmt.Fprintf(&detail, " reason=%s", reason)
		}
		if reason == "" {
			detail.WriteString(" reason=package failure reported without diagnostic output")
		}
		detail.WriteByte('\n')
	}
	return strings.TrimSpace(detail.String())
}

type functionalFailureRecord struct {
	event goTestTimingEvent
}

func parseFunctionalFailureRecords(jsonOutput string) []functionalFailureRecord {
	records := make([]functionalFailureRecord, 0)
	for _, line := range strings.Split(jsonOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event goTestTimingEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Package == "" {
			continue
		}
		records = append(records, functionalFailureRecord{event: event})
	}
	return records
}

func sortedFunctionalFailureTests(tests map[string]struct{}) []string {
	if len(tests) == 0 {
		return nil
	}
	result := make([]string, 0, len(tests))
	for testName := range tests {
		result = append(result, testName)
	}
	sort.Strings(result)
	return result
}

func firstFunctionalFailureReason(records []functionalFailureRecord, packageName string, failedTests map[string]struct{}) string {
	for _, record := range records {
		if record.event.Package != packageName || record.event.Action != "output" {
			continue
		}
		if record.event.Test != "" && len(failedTests) > 0 {
			if _, failed := failedTests[record.event.Test]; !failed {
				continue
			}
		}
		if reason := compactFunctionalFailureReason(record.event.Output); reason != "" {
			return reason
		}
	}
	return ""
}

func compactFunctionalFailureReason(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=== RUN") || strings.HasPrefix(line, "=== PAUSE") || strings.HasPrefix(line, "=== CONT") {
			continue
		}
		if strings.HasPrefix(line, "coverage: ") || strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "ok\t") {
			continue
		}
		if len(line) > maxTimingFailureReasonLength {
			return line[:maxTimingFailureReasonLength] + "..."
		}
		return line
	}
	return ""
}

func renderFunctionalFailureFallback(output string) string {
	rendered := renderGoTestEventOutput(output)
	lines := make([]string, 0)
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !isFunctionalFailureDiagnosticLine(line) {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func isFunctionalFailureDiagnosticLine(line string) bool {
	for _, prefix := range []string{
		"--- FAIL:",
		"FAIL",
		"panic:",
		"fatal error:",
		"test timed out",
		"timeout",
		"error:",
		"syntax error",
		"undefined:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}
