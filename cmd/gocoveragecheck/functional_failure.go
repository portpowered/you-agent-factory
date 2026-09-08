package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const functionalFailureNoDiagnosticReason = "package failure reported without diagnostic output"

const (
	functionalFailureReasonFallback = iota
	functionalFailureReasonFailMarker
	functionalFailureReasonAssertion
	functionalFailureReasonTerminal
)

// functionalFailureReasonCandidate is deliberately independent of the output
// event shape. Both the terminal error and timing projections feed the same
// candidates into the selector, so a benign first log cannot win merely
// because it arrived first.
type functionalFailureReasonCandidate struct {
	reason        string
	rank          int
	eventPosition int
	linePosition  int
}

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
		if record.event.Test == "" || strings.Contains(record.event.Test, "/") {
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
		reason := selectFunctionalFailureReason(records, packageName, failedTests[packageName])
		fmt.Fprintf(&detail, "functional test failure: package=%s", packageName)
		if len(testNames) > 0 {
			fmt.Fprintf(&detail, " test=%s", strings.Join(testNames, ","))
		}
		if reason != "" {
			fmt.Fprintf(&detail, " reason=%s", reason)
		}
		if reason == "" {
			detail.WriteString(" reason=" + functionalFailureNoDiagnosticReason)
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

func selectFunctionalFailureReason(records []functionalFailureRecord, packageName string, failedTests map[string]struct{}) string {
	terminalPosition := -1
	for position, record := range records {
		if record.event.Package == packageName && record.event.Action == timingOutcomeFail {
			terminalPosition = position
		}
	}

	candidates := make([]functionalFailureReasonCandidate, 0)
	for position, record := range records {
		if record.event.Package != packageName || record.event.Action != "output" {
			continue
		}
		if record.event.Test != "" && len(failedTests) > 0 {
			if _, failed := failedTests[record.event.Test]; !failed || strings.Contains(record.event.Test, "/") {
				continue
			}
		}
		if terminalPosition >= 0 && position > terminalPosition {
			continue
		}
		candidates = append(candidates, functionalFailureReasonCandidates(record.event.Output, position)...)
	}
	return selectFunctionalFailureReasonCandidates(candidates)
}

func functionalFailureReasonCandidates(output string, eventPosition int) []functionalFailureReasonCandidate {
	lines := strings.Split(output, "\n")
	candidates := make([]functionalFailureReasonCandidate, 0, len(lines))
	for linePosition, rawLine := range lines {
		line := normalizeFunctionalFailureReasonLine(rawLine)
		if line == "" || isFunctionalFailureReasonBoilerplate(line) || isFunctionalFailureStackLine(line) {
			continue
		}
		if containsRawFunctionalSecret(line) {
			continue
		}
		candidates = append(candidates, functionalFailureReasonCandidate{
			reason:        line,
			rank:          rankFunctionalFailureReason(line),
			eventPosition: eventPosition,
			linePosition:  linePosition,
		})
	}
	return candidates
}

func selectFunctionalFailureReasonCandidates(candidates []functionalFailureReasonCandidate) string {
	var selected *functionalFailureReasonCandidate
	for index := range candidates {
		candidate := candidates[index]
		candidate.reason = boundFunctionalFailureReason(candidate.reason)
		if candidate.reason == "" || containsRawFunctionalSecret(candidate.reason) {
			continue
		}
		if selected == nil || betterFunctionalFailureReasonCandidate(candidate, *selected) {
			selected = &candidate
		}
	}
	if selected == nil {
		return ""
	}
	return selected.reason
}

func betterFunctionalFailureReasonCandidate(left, right functionalFailureReasonCandidate) bool {
	if left.rank != right.rank {
		return left.rank > right.rank
	}
	if left.eventPosition != right.eventPosition {
		return left.eventPosition > right.eventPosition
	}
	if left.linePosition != right.linePosition {
		return left.linePosition > right.linePosition
	}
	return left.reason < right.reason
}

func normalizeFunctionalFailureReasonLine(line string) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(line, "�")), " ")
}

// boundFunctionalFailureReason keeps the public diagnostic within the
// existing 240-byte contract. The ellipsis is included in that bound and the
// cut is made on a UTF-8 rune boundary.
func boundFunctionalFailureReason(reason string) string {
	reason = normalizeFunctionalFailureReasonLine(reason)
	if reason == "" {
		return ""
	}
	if len([]byte(reason)) <= maxTimingFailureReasonLength {
		return reason
	}
	const ellipsis = "..."
	limit := maxTimingFailureReasonLength - len(ellipsis)
	bounded := []byte(reason)[:limit]
	for len(bounded) > 0 && !utf8.Valid(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return string(bounded) + ellipsis
}

func rankFunctionalFailureReason(line string) int {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "panic:") ||
		strings.HasPrefix(lower, "fatal error:") ||
		strings.HasPrefix(lower, "runtime error:") ||
		strings.Contains(lower, "test timed out") ||
		strings.Contains(lower, "timed out after") ||
		strings.HasPrefix(lower, "timeout") ||
		strings.Contains(lower, "fatal assertion") {
		return functionalFailureReasonTerminal
	}
	if isFunctionalFailureFailMarker(line) {
		return functionalFailureReasonFailMarker
	}
	for _, marker := range []string{
		"assert",
		"expected",
		"actual",
		"got ",
		"want ",
		"wanted",
		"mismatch",
		"failed",
		"failure",
		"error",
		"invalid",
		"unexpected",
		"not found",
	} {
		if strings.Contains(lower, marker) {
			return functionalFailureReasonAssertion
		}
	}
	return functionalFailureReasonFallback
}

func isFunctionalFailureReasonBoilerplate(line string) bool {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "=== run") || strings.HasPrefix(lower, "=== pause") || strings.HasPrefix(lower, "=== cont") {
		return true
	}
	if strings.HasPrefix(lower, "--- pass:") || strings.HasPrefix(lower, "--- skip:") {
		return true
	}
	if lower == "pass" || strings.HasPrefix(lower, "ok ") || strings.HasPrefix(lower, "ok\t") || strings.HasPrefix(lower, "coverage: ") || strings.HasPrefix(lower, "? ") {
		return true
	}
	return false
}

func isFunctionalFailureStackLine(line string) bool {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "goroutine ") || strings.HasPrefix(lower, "created by ") || strings.HasPrefix(lower, "runtime/") || strings.HasPrefix(lower, "testing.") {
		return true
	}
	return strings.Contains(lower, ".go:") && strings.Contains(lower, " +0x")
}

func isFunctionalFailureFailMarker(line string) bool {
	return strings.HasPrefix(line, "--- FAIL:") || line == "FAIL" || strings.HasPrefix(line, "FAIL ") || strings.HasPrefix(line, "FAIL\t")
}

func containsRawFunctionalSecret(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"secret-sentinel",
		"secret_sentinel",
		"synthetic-secret",
		"synthetic_secret",
		"credential-sentinel",
		"credential_sentinel",
		"raw-secret",
		"raw_secret",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, key := range []string{
		"hf_token",
		"api_key",
		"api-key",
		"access_token",
		"auth_token",
		"token",
		"password",
		"secret",
		"credential",
	} {
		if hasRawFunctionalSecretAssignment(lower, key) {
			return true
		}
	}
	return hasRawFunctionalBearerToken(lower)
}

func hasRawFunctionalSecretAssignment(line, key string) bool {
	searchFrom := 0
	for searchFrom < len(line) {
		relative := strings.Index(line[searchFrom:], key)
		if relative < 0 {
			return false
		}
		start := searchFrom + relative
		end := start + len(key)
		if end < len(line) && (line[end] == '=' || line[end] == ':') {
			value := strings.TrimSpace(line[end+1:])
			if value != "" && !strings.HasPrefix(value, "<redacted>") && !strings.HasPrefix(value, "[redacted]") {
				return true
			}
		}
		searchFrom = end
	}
	return false
}

func hasRawFunctionalBearerToken(line string) bool {
	for _, marker := range []string{"authorization: bearer ", "authorization=bearer ", "bearer "} {
		if index := strings.Index(line, marker); index >= 0 {
			value := strings.TrimSpace(line[index+len(marker):])
			if value != "" && !strings.HasPrefix(value, "<redacted>") && !strings.HasPrefix(value, "[redacted]") {
				return true
			}
		}
	}
	return false
}

func renderFunctionalFailureFallback(output string) string {
	rendered := renderGoTestEventOutput(output)
	lines := make([]string, 0)
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !isFunctionalFailureDiagnosticLine(line) || containsRawFunctionalSecret(line) {
			continue
		}
		lines = append(lines, boundFunctionalFailureReason(line))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func isFunctionalFailureDiagnosticLine(line string) bool {
	line = strings.ToLower(line)
	for _, prefix := range []string{
		"--- fail:",
		"fail",
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
