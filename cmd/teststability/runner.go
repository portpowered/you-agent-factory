package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	defaultStabilityAttempts = 20
	defaultStabilityBudget   = 15 * time.Minute
)

type testGroup struct {
	Package string
	Tests   []string
}

type attemptExecutor func(context.Context, testGroup, string) (string, error)

type stabilityRunner struct {
	attempts int
	budget   time.Duration
	now      func() time.Time
	run      attemptExecutor
}

type attemptCounts map[string]int

func groupSelectedTests(tests []selectedTest, packageForFile func(string) (string, error)) ([]testGroup, error) {
	byPackage := make(map[string]map[string]struct{})
	for _, test := range tests {
		packagePath, err := packageForFile(test.File)
		if err != nil {
			return nil, fmt.Errorf("resolve package for %q: %w", test.File, err)
		}
		if strings.TrimSpace(packagePath) == "" {
			return nil, fmt.Errorf("resolve package for %q: go list returned an empty package", test.File)
		}
		if byPackage[packagePath] == nil {
			byPackage[packagePath] = make(map[string]struct{})
		}
		byPackage[packagePath][test.Name] = struct{}{}
	}

	groups := make([]testGroup, 0, len(byPackage))
	for packagePath, names := range byPackage {
		tests := make([]string, 0, len(names))
		for name := range names {
			tests = append(tests, name)
		}
		slices.Sort(tests)
		groups = append(groups, testGroup{Package: packagePath, Tests: tests})
	}
	slices.SortFunc(groups, func(left, right testGroup) int {
		return strings.Compare(left.Package, right.Package)
	})
	return groups, nil
}

func (runner stabilityRunner) runAll(groups []testGroup, output io.Writer) error {
	if runner.attempts < 1 {
		return errors.New("stability attempts must be greater than zero")
	}
	if runner.budget <= 0 {
		return errors.New("stability budget must be greater than zero")
	}
	if runner.now == nil {
		runner.now = time.Now
	}
	if runner.run == nil {
		runner.run = func(ctx context.Context, group testGroup, testName string) (string, error) {
			return runGoTestAttempt(ctx, ".", group, testName)
		}
	}
	if len(groups) == 0 {
		_, _ = fmt.Fprintf(output, "Changed-test stability: no qualifying tests; success (attempts=%d budget=%s)\n", runner.attempts, runner.budget)
		return nil
	}

	totalTests := countGroupedTests(groups)
	deadline := runner.now().Add(runner.budget)
	_, _ = fmt.Fprintf(output, "Changed-test stability: selectors=%d attempts=%d expected-attempts=%d budget=%s\n", totalTests, runner.attempts, totalTests*runner.attempts, runner.budget)
	completed := make(attemptCounts)
	for _, group := range groups {
		for _, testName := range group.Tests {
			key := attemptKey(group.Package, testName)
			for attempt := 1; attempt <= runner.attempts; attempt++ {
				if !runner.now().Before(deadline) {
					return budgetExpiredError(groups, completed, runner.attempts, deadline, output)
				}
				ctx, cancel := context.WithDeadline(context.Background(), deadline)
				captured, err := runner.run(ctx, group, testName)
				cancel()
				if !runner.now().Before(deadline) {
					completed[key]++
					return budgetExpiredError(groups, completed, runner.attempts, deadline, output)
				}
				if err != nil {
					return attemptFailureError(group.Package, testName, attempt, runner.attempts, captured, err, output)
				}
				completed[key]++
			}
		}
	}
	_, _ = fmt.Fprintf(output, "Changed-test stability: success selectors=%d attempts=%d measured=%d\n", totalTests, runner.attempts, totalTests*runner.attempts)
	return nil
}

func countGroupedTests(groups []testGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Tests)
	}
	return total
}

func attemptKey(packagePath, testName string) string {
	return packagePath + "\x00" + testName
}

func attemptFailureError(packagePath, testName string, attempt, attempts int, captured string, runErr error, output io.Writer) error {
	if strings.TrimSpace(captured) == "" {
		captured = "<go test produced no output>"
	}
	_, _ = fmt.Fprintf(output, "Changed-test stability: failure package=%s test=%s attempt=%d/%d\nGo test output:\n%s\nFocused reproduction: %s\n", packagePath, testName, attempt, attempts, captured, focusedReproductionCommand(packagePath, testName))
	return fmt.Errorf("changed-test stability failed for package %s test %s attempt %d/%d: %w", packagePath, testName, attempt, attempts, runErr)
}

func budgetExpiredError(groups []testGroup, completed attemptCounts, attempts int, deadline time.Time, output io.Writer) error {
	remaining := make([]string, 0)
	completedTotal := 0
	for _, group := range groups {
		for _, testName := range group.Tests {
			count := completed[attemptKey(group.Package, testName)]
			completedTotal += count
			if count < attempts {
				remaining = append(remaining, fmt.Sprintf("%s %s (%d/%d)", group.Package, testName, count, attempts))
			}
		}
	}
	_, _ = fmt.Fprintf(output, "Changed-test stability: fail-closed budget expired deadline=%s completed-attempts=%d unmeasured=%s\n", deadline.Format(time.RFC3339Nano), completedTotal, strings.Join(remaining, ", "))
	return fmt.Errorf("changed-test stability budget expired before all attempts completed: completed-attempts=%d unmeasured=%s", completedTotal, strings.Join(remaining, ", "))
}

func focusedReproductionCommand(packagePath, testName string) string {
	return fmt.Sprintf("go test -count=1 -run=^%s$ %s", regexp.QuoteMeta(testName), packagePath)
}

func runGoTestAttempt(ctx context.Context, repoRoot string, group testGroup, testName string) (string, error) {
	args := []string{"test", "-count=1", "-run=^" + regexp.QuoteMeta(testName) + "$", group.Package}
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = repoRoot
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	return string(output), err
}
