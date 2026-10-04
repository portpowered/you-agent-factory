package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Flake retry: when a functional coverage run fails because of a few ordinary
// test failures, rerun only those top-level tests once on the same commit with
// the same flags. A test that passes on retry is recorded as a flake in a
// machine-readable ledger and the lane passes; a test that fails again keeps
// the lane failed exactly as before. Binary deaths (panic, timeout, build
// failure) and mass failures are never retried.
//
// The behaviour is opt-in through environment variables so local runs are
// unchanged:
//
//	FUNCTIONAL_FLAKE_RETRY_MAX   maximum distinct failing top-level tests to
//	                             retry; unset or 0 disables the retry
//	FUNCTIONAL_FLAKE_LEDGER      path of the JSON ledger (optional)
//	FUNCTIONAL_FLAKE_HEAD_SHA    commit recorded in the ledger (optional)
const (
	flakeRetryMaxEnv     = "FUNCTIONAL_FLAKE_RETRY_MAX"
	flakeLedgerEnv       = "FUNCTIONAL_FLAKE_LEDGER"
	flakeHeadSHAEnv      = "FUNCTIONAL_FLAKE_HEAD_SHA"
	flakeLedgerSchema    = 1
	flakeExcerptMaxBytes = 2000
)

const (
	flakeOutcomeRecovered  = "flake-recovered"
	flakeOutcomePersistent = "failed-after-retry"
	flakeOutcomeNotRetried = "not-retried"
)

type flakeRetrySettings struct {
	maxTests   int
	ledgerPath string
	headSHA    string
}

func flakeRetrySettingsFromEnv() flakeRetrySettings {
	maxTests, err := strconv.Atoi(strings.TrimSpace(os.Getenv(flakeRetryMaxEnv)))
	if err != nil || maxTests < 0 {
		maxTests = 0
	}
	head := strings.TrimSpace(os.Getenv(flakeHeadSHAEnv))
	if head == "" {
		head = strings.TrimSpace(os.Getenv("GITHUB_SHA"))
	}
	return flakeRetrySettings{
		maxTests:   maxTests,
		ledgerPath: strings.TrimSpace(os.Getenv(flakeLedgerEnv)),
		headSHA:    head,
	}
}

// flakeFailure is one failing top-level test and what it looked like.
type flakeFailure struct {
	Package  string   `json:"package"`
	Test     string   `json:"test"`
	Subtests []string `json:"failedSubtests,omitempty"`
	Excerpt  string   `json:"firstFailureExcerpt"`
}

type flakeLedgerEntry struct {
	flakeFailure
	RetryOutcome string `json:"retryOutcome"`
}

type flakeLedger struct {
	SchemaVersion int                `json:"schemaVersion"`
	HeadSHA       string             `json:"headSha"`
	Outcome       string             `json:"outcome"`
	Reason        string             `json:"reason"`
	RetryLimit    int                `json:"retryLimit"`
	Entries       []flakeLedgerEntry `json:"entries"`
}

// flakeRetryDecision is the pure result of inspecting a failed run.
type flakeRetryDecision struct {
	retry    bool
	reason   string
	failures []flakeFailure
}

// decideFlakeRetry inspects the go test -json output (and stderr) of the
// failed run. It retries only when every failure is an ordinary test failure:
// each failing package has at least one failing test, nothing panicked,
// timed out or failed to build, and no more than maxTests distinct top-level
// tests failed.
func decideFlakeRetry(jsonOutput string, stderr string, maxTests int) flakeRetryDecision {
	if maxTests <= 0 {
		return flakeRetryDecision{reason: "flake retry disabled"}
	}
	failedPackages := map[string]bool{}
	packagesWithFailedTests := map[string]bool{}
	tests := map[string]*flakeFailure{}
	outputs := map[string]*strings.Builder{}
	var order []string
	binaryDeath := firstBinaryDeathLine(stderr)

	for _, line := range strings.Split(jsonOutput, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var event goTestTimingEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Package == "" {
			continue
		}
		if event.Action == "output" {
			if binaryDeath == "" {
				binaryDeath = firstBinaryDeathLine(event.Output)
			}
			if event.Test != "" {
				key := event.Package + "\x00" + topLevelTestName(event.Test)
				if outputs[key] == nil {
					outputs[key] = &strings.Builder{}
				}
				if outputs[key].Len() < flakeExcerptMaxBytes*4 {
					outputs[key].WriteString(event.Output)
				}
			}
			continue
		}
		if event.Action != timingOutcomeFail {
			continue
		}
		if event.Test == "" {
			failedPackages[event.Package] = true
			continue
		}
		packagesWithFailedTests[event.Package] = true
		top := topLevelTestName(event.Test)
		key := event.Package + "\x00" + top
		failure := tests[key]
		if failure == nil {
			failure = &flakeFailure{Package: event.Package, Test: top}
			tests[key] = failure
			order = append(order, key)
		}
		if event.Test != top {
			failure.Subtests = append(failure.Subtests, event.Test)
		}
	}

	if binaryDeath != "" {
		return flakeRetryDecision{reason: "binary death is not a flake: " + truncateFlakeText(binaryDeath, 200)}
	}
	if len(tests) == 0 {
		return flakeRetryDecision{reason: "no failing tests were observed"}
	}
	for pkg := range failedPackages {
		if !packagesWithFailedTests[pkg] {
			return flakeRetryDecision{reason: fmt.Sprintf("package %s failed with zero failing tests", pkg)}
		}
	}
	if len(tests) > maxTests {
		return flakeRetryDecision{reason: fmt.Sprintf("%d failing tests exceed the retry limit of %d; mass failure is not a flake", len(tests), maxTests)}
	}
	sort.Strings(order)
	failures := make([]flakeFailure, 0, len(order))
	for _, key := range order {
		failure := *tests[key]
		sort.Strings(failure.Subtests)
		if builder := outputs[key]; builder != nil {
			failure.Excerpt = failureExcerpt(builder.String())
		}
		failures = append(failures, failure)
	}
	return flakeRetryDecision{retry: true, failures: failures, reason: fmt.Sprintf("retrying %d failing test(s) once", len(failures))}
}

func firstBinaryDeathLine(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: ") ||
			strings.Contains(line, "[build failed]") || strings.Contains(line, "[setup failed]") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func topLevelTestName(name string) string {
	if index := strings.Index(name, "/"); index >= 0 {
		return name[:index]
	}
	return name
}

// failureExcerpt keeps the lines that explain the failure, not the whole log.
func failureExcerpt(output string) string {
	var kept []string
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- PASS") {
			continue
		}
		kept = append(kept, trimmed)
	}
	return truncateFlakeText(strings.Join(kept, "\n"), flakeExcerptMaxBytes)
}

func truncateFlakeText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return strings.ToValidUTF8(text[:limit], "") + "..."
}

// buildFlakeRetryInvocations makes one go test invocation per failing package,
// reusing the failed invocation's flags (covermode, coverpkg, timeout, -short,
// -p, ...) and replacing only the run selector, profile, and package. It
// returns the invocations and their profile paths in matching order.
func buildFlakeRetryInvocations(template commandInvocation, failures []flakeFailure, profileDir string) ([]commandInvocation, []string) {
	byPackage := map[string][]string{}
	var packages []string
	for _, failure := range failures {
		if _, ok := byPackage[failure.Package]; !ok {
			packages = append(packages, failure.Package)
		}
		byPackage[failure.Package] = append(byPackage[failure.Package], failure.Test)
	}
	sort.Strings(packages)
	var flags []string
	for index, arg := range template.args {
		switch {
		case index == 0 && arg == "test":
		case strings.HasPrefix(arg, "-run="), strings.HasPrefix(arg, "-coverprofile="):
		case strings.HasPrefix(arg, "-"):
			flags = append(flags, arg)
		}
	}
	invocations := make([]commandInvocation, 0, len(packages))
	profiles := make([]string, 0, len(packages))
	for index, pkg := range packages {
		quoted := make([]string, 0, len(byPackage[pkg]))
		for _, name := range byPackage[pkg] {
			quoted = append(quoted, regexp.QuoteMeta(name))
		}
		profile := filepath.Join(profileDir, fmt.Sprintf("flake-retry-%03d.out", index))
		args := append([]string{"test"}, flags...)
		args = append(args, "-run=^("+strings.Join(quoted, "|")+")$", "-coverprofile="+profile, pkg)
		invocations = append(invocations, commandInvocation{name: template.name, args: args, env: template.env, dir: template.dir})
		profiles = append(profiles, profile)
	}
	return invocations, profiles
}

// stripRetriedEvents removes from the original stream the events that the
// retry supersedes: the failing tests and the retried packages' own
// package-level events.
func stripRetriedEvents(jsonOutput string, failures []flakeFailure) string {
	retriedTests := map[string]bool{}
	retriedPackages := map[string]bool{}
	for _, failure := range failures {
		retriedTests[failure.Package+"\x00"+failure.Test] = true
		retriedPackages[failure.Package] = true
	}
	var kept []string
	for _, line := range strings.Split(jsonOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") {
			var event goTestTimingEvent
			if err := json.Unmarshal([]byte(trimmed), &event); err == nil && event.Package != "" {
				if event.Test == "" && retriedPackages[event.Package] {
					continue
				}
				if event.Test != "" && retriedTests[event.Package+"\x00"+topLevelTestName(event.Test)] {
					continue
				}
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

type flakeRetryResult struct {
	attempted bool
	// recovered is true only when every retried test passed.
	recovered bool
	stdout    string
	profiles  []string
	cleanup   func()
}

// runFlakeRetry applies the retry policy to a failed run. It never turns a
// failure into success unless every retried test passed on the second try.
func runFlakeRetry(settings flakeRetrySettings, plan coverageInvocationPlan, result coverageInvocationResult, repoRoot string) flakeRetryResult {
	noop := flakeRetryResult{cleanup: func() {}}
	if settings.maxTests <= 0 || !result.testFailureObserved || len(plan.invocations) == 0 || strings.TrimSpace(plan.covdataDir) != "" {
		return noop
	}
	decision := decideFlakeRetry(result.stdout, result.failureStderr, settings.maxTests)
	ledger := flakeLedger{SchemaVersion: flakeLedgerSchema, HeadSHA: settings.headSHA, RetryLimit: settings.maxTests, Entries: []flakeLedgerEntry{}}
	if !decision.retry {
		ledger.Outcome, ledger.Reason = flakeOutcomeNotRetried, decision.reason
		writeFlakeLedger(settings.ledgerPath, ledger)
		fmt.Fprintf(stderrWriter, "flake retry skipped: %s\n", decision.reason)
		return noop
	}

	profileDir, err := os.MkdirTemp("", "gocoveragecheck-flake-retry-*")
	if err != nil {
		fmt.Fprintf(stderrWriter, "flake retry skipped: %v\n", err)
		return noop
	}
	cleanup := func() { _ = os.RemoveAll(profileDir) }
	template := plan.invocations[0]
	template.stdoutWriter, template.stderrWriter = nil, nil
	if template.dir == "" {
		template.dir = repoRoot
	}
	retries, profiles := buildFlakeRetryInvocations(template, decision.failures, profileDir)
	fmt.Fprintf(stderrWriter, "flake retry: %s\n", decision.reason)

	passedPackages := map[string]bool{}
	var retryStdout strings.Builder
	for _, invocation := range retries {
		stdout, _, runErr := runCommand(invocation)
		retryStdout.WriteString(stdout)
		passedPackages[invocation.args[len(invocation.args)-1]] = runErr == nil
	}
	allPassed := true
	for _, failure := range decision.failures {
		entry := flakeLedgerEntry{flakeFailure: failure, RetryOutcome: "pass"}
		if !passedPackages[failure.Package] {
			entry.RetryOutcome = "fail"
			allPassed = false
		}
		ledger.Entries = append(ledger.Entries, entry)
	}
	if allPassed {
		ledger.Outcome, ledger.Reason = flakeOutcomeRecovered, "every retried test passed on the same commit; recorded as a flake"
	} else {
		ledger.Outcome, ledger.Reason = flakeOutcomePersistent, "a retried test failed again; the lane stays failed"
	}
	writeFlakeLedger(settings.ledgerPath, ledger)
	for _, entry := range ledger.Entries {
		fmt.Fprintf(stderrWriter, "flake retry: package=%s test=%s retry=%s\n", entry.Package, entry.Test, entry.RetryOutcome)
	}
	if !allPassed {
		return flakeRetryResult{attempted: true, cleanup: cleanup}
	}
	return flakeRetryResult{
		attempted: true,
		recovered: true,
		stdout:    appendFlakeRetryStream(stripRetriedEvents(result.stdout, decision.failures), retryStdout.String()),
		profiles:  profiles,
		cleanup:   cleanup,
	}
}

func appendFlakeRetryStream(original string, retry string) string {
	if original != "" && !strings.HasSuffix(original, "\n") {
		original += "\n"
	}
	return original + retry
}

func writeFlakeLedger(path string, ledger flakeLedger) {
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(path), 0o755)
	}
	if err == nil {
		err = os.WriteFile(path, append(data, '\n'), 0o644)
	}
	if err != nil {
		fmt.Fprintf(stderrWriter, "flake ledger not written: %v\n", err)
	}
}

// mergeFlakeRetryProfiles folds the retry profiles into the lane profile so the
// retried tests' coverage still counts toward the floors. The lane profile is
// optional because a failed go test run may not have written one.
func mergeFlakeRetryProfiles(profilePath string, retryProfiles []string, repoRoot string, coverPackages []string) error {
	inputs := make([]string, 0, len(retryProfiles)+1)
	for _, path := range append([]string{profilePath}, retryProfiles...) {
		if _, err := os.Stat(path); err == nil {
			inputs = append(inputs, path)
		}
	}
	if len(inputs) == 0 {
		return nil
	}
	return mergeCoverageProfiles(inputs, profilePath, repoRoot, coverPackages)
}
