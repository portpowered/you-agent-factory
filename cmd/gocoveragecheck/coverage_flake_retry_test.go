package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func flakeEvent(action, pkg, test, output string) string {
	event := map[string]string{"Action": action, "Package": pkg}
	if test != "" {
		event["Test"] = test
	}
	if output != "" {
		event["Output"] = output
	}
	data, _ := json.Marshal(event)
	return string(data) + "\n"
}

func failingStream(pkg string, tests ...string) string {
	var stream strings.Builder
	stream.WriteString(flakeEvent("start", pkg, "", ""))
	for _, test := range tests {
		stream.WriteString(flakeEvent("run", pkg, test, ""))
		stream.WriteString(flakeEvent("output", pkg, test, "    assertion failed in "+test+"\n"))
		stream.WriteString(flakeEvent("fail", pkg, test, ""))
	}
	stream.WriteString(flakeEvent("fail", pkg, "", ""))
	return stream.String()
}

func TestDecideFlakeRetryRetriesTopLevelTestForSubtestFailures(t *testing.T) {
	stream := failingStream("example/tests/functional/a", "TestParent/sub_one", "TestParent/sub_two", "TestParent")
	decision := decideFlakeRetry(stream, "", 5)
	if !decision.retry || len(decision.failures) != 1 {
		t.Fatalf("decision = %+v, want one retried top-level test", decision)
	}
	failure := decision.failures[0]
	if failure.Test != "TestParent" || len(failure.Subtests) != 2 || !strings.Contains(failure.Excerpt, "assertion failed") {
		t.Fatalf("failure = %+v, want TestParent with two subtests and an excerpt", failure)
	}
}

func TestDecideFlakeRetryRefusals(t *testing.T) {
	tooMany := failingStream("p/a", "T1", "T2", "T3") + failingStream("p/b", "T4", "T5", "T6")
	zeroTests := flakeEvent("output", "p/z", "", "FAIL p/z 0.1s\n") + flakeEvent("fail", "p/z", "", "")
	mixed := failingStream("p/a", "T1") + zeroTests
	panicked := failingStream("p/a", "T1") + flakeEvent("output", "p/a", "T1", "panic: boom\n")
	cases := map[string]struct {
		stream, stderr string
		max            int
		want           string
	}{
		"disabled":              {failingStream("p/a", "T1"), "", 0, "disabled"},
		"mass failure":          {tooMany, "", 5, "mass failure"},
		"package without tests": {mixed, "", 5, "zero failing tests"},
		"panic in output":       {panicked, "", 5, "binary death"},
		"stderr timeout":        {failingStream("p/a", "T1"), "panic: test timed out after 10m0s\n", 5, "binary death"},
		"no failing tests":      {flakeEvent("pass", "p/a", "", ""), "", 5, "no failing tests"},
	}
	for name, tc := range cases {
		decision := decideFlakeRetry(tc.stream, tc.stderr, tc.max)
		if decision.retry || !strings.Contains(decision.reason, tc.want) {
			t.Errorf("%s: decision = %+v, want refusal containing %q", name, decision, tc.want)
		}
	}
}

func TestDecideFlakeRetryAllowsExactlyTheLimit(t *testing.T) {
	stream := failingStream("p/a", "T1", "T2") + failingStream("p/b", "T3", "T4", "T5")
	if decision := decideFlakeRetry(stream, "", 5); !decision.retry || len(decision.failures) != 5 {
		t.Fatalf("decision = %+v, want five retried tests", decision)
	}
	if decision := decideFlakeRetry(stream, "", 4); decision.retry {
		t.Fatalf("decision = %+v, want refusal above the limit", decision)
	}
}

func TestBuildFlakeRetryInvocationsKeepsFlagsAndAnchorsNames(t *testing.T) {
	template := commandInvocation{
		name: "go",
		args: []string{"test", "-coverpkg=a,b", "-p=4", "-covermode=count", "-run=^TestOther$", "-coverprofile=/tmp/x.out", "-json", "example/p1", "example/p2"},
		env:  []string{"A=1"},
	}
	invocations, profiles := buildFlakeRetryInvocations(template, []flakeFailure{
		{Package: "example/p2", Test: "TestB"},
		{Package: "example/p1", Test: "TestA.v1"},
		{Package: "example/p1", Test: "TestC"},
	}, "/profiles")
	if len(invocations) != 2 || len(profiles) != 2 {
		t.Fatalf("invocations = %d profiles = %d, want one per package", len(invocations), len(profiles))
	}
	got := strings.Join(invocations[0].args, " ")
	want := "test -coverpkg=a,b -p=4 -covermode=count -json -run=^(TestA\\.v1|TestC)$ -coverprofile=" + profiles[0] + " example/p1"
	if got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if strings.Contains(strings.Join(invocations[1].args, " "), "TestOther") || invocations[1].args[len(invocations[1].args)-1] != "example/p2" {
		t.Fatalf("args = %v, want only the retried package and test", invocations[1].args)
	}
}

func TestStripRetriedEventsKeepsUnrelatedResults(t *testing.T) {
	stream := flakeEvent("pass", "p/ok", "TestOK", "") + flakeEvent("pass", "p/ok", "", "") +
		flakeEvent("pass", "p/a", "TestKeep", "") + failingStream("p/a", "TestFlaky/sub", "TestFlaky")
	stripped := stripRetriedEvents(stream, []flakeFailure{{Package: "p/a", Test: "TestFlaky"}})
	if strings.Contains(stripped, "TestFlaky") || strings.Contains(stripped, `"Action":"fail"`) {
		t.Fatalf("stripped stream still has the retried failure: %s", stripped)
	}
	if !strings.Contains(stripped, "TestOK") || !strings.Contains(stripped, "TestKeep") || !strings.Contains(stripped, `"Package":"p/ok"`) {
		t.Fatalf("stripped stream lost unrelated results: %s", stripped)
	}
}

type flakeRunHarness struct {
	calls   []commandInvocation
	stderr  *bytes.Buffer
	ledger  string
	profile string
}

func newFlakeRunHarness(t *testing.T, retryOutcomes map[string]error) *flakeRunHarness {
	t.Helper()
	originalRunner, originalStderr := commandRunner, stderrWriter
	t.Cleanup(func() { commandRunner, stderrWriter = originalRunner, originalStderr })
	harness := &flakeRunHarness{stderr: &bytes.Buffer{}}
	stderrWriter = harness.stderr
	dir := t.TempDir()
	harness.ledger = filepath.Join(dir, "flake-ledger.json")
	harness.profile = filepath.Join(dir, "coverage.out")
	t.Setenv(flakeRetryMaxEnv, "5")
	t.Setenv(flakeLedgerEnv, harness.ledger)
	t.Setenv(flakeHeadSHAEnv, "abc123")
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		harness.calls = append(harness.calls, invocation)
		pkg := invocation.args[len(invocation.args)-1]
		if len(harness.calls) == 1 {
			return flakeEvent("pass", "p/ok", "TestOK", "") + flakeEvent("pass", "p/ok", "", "") + failingStream("p/a", "TestFlaky/sub", "TestFlaky"), "FAIL\n", errors.New("exit status 1")
		}
		retryErr := retryOutcomes[pkg]
		stream := flakeEvent("start", pkg, "", "") + flakeEvent("pass", pkg, "TestFlaky", "")
		if retryErr != nil {
			stream += flakeEvent("fail", pkg, "TestFlaky", "") + flakeEvent("fail", pkg, "", "")
		} else {
			stream += flakeEvent("pass", pkg, "", "")
		}
		return stream, "", retryErr
	}
	return harness
}

func flakePlan() coverageInvocationPlan {
	return coverageInvocationPlan{
		invocations: []commandInvocation{{name: "go", args: []string{"test", "-json", "-covermode=count", "p/ok", "p/a"}}},
		cleanup:     func() error { return nil },
	}
}

func TestExecuteCoverageInvocationPlanRecordsRecoveredFlake(t *testing.T) {
	harness := newFlakeRunHarness(t, map[string]error{})
	err := executeCoverageInvocationPlan(config{}, flakePlan(), []string{"p/ok", "p/a"}, harness.profile, ".", nil, "run", nil)
	if err != nil {
		t.Fatalf("executeCoverageInvocationPlan() error = %v, want recovered flake to pass", err)
	}
	if len(harness.calls) != 2 {
		t.Fatalf("go invocations = %d, want the lane plus one retry", len(harness.calls))
	}
	retry := strings.Join(harness.calls[1].args, " ")
	if !strings.Contains(retry, "-run=^(TestFlaky)$") || !strings.Contains(retry, "-covermode=count") || strings.HasSuffix(retry, "p/ok") {
		t.Fatalf("retry args = %q, want same flags and only the flaky test", retry)
	}
	var ledger flakeLedger
	data, readErr := os.ReadFile(harness.ledger)
	if readErr != nil || json.Unmarshal(data, &ledger) != nil {
		t.Fatalf("ledger unreadable: %v %s", readErr, data)
	}
	if ledger.Outcome != flakeOutcomeRecovered || ledger.HeadSHA != "abc123" || len(ledger.Entries) != 1 ||
		ledger.Entries[0].Test != "TestFlaky" || ledger.Entries[0].RetryOutcome != "pass" || ledger.Entries[0].Excerpt == "" {
		t.Fatalf("ledger = %+v, want one recovered TestFlaky entry with an excerpt", ledger)
	}
}

func TestExecuteCoverageInvocationPlanKeepsFailureWhenRetryFailsAgain(t *testing.T) {
	harness := newFlakeRunHarness(t, map[string]error{"p/a": errors.New("exit status 1")})
	err := executeCoverageInvocationPlan(config{}, flakePlan(), []string{"p/ok", "p/a"}, harness.profile, ".", nil, "run", nil)
	if err == nil {
		t.Fatal("executeCoverageInvocationPlan() error = nil, want the failure to stand")
	}
	var testFailure *coverageTestFailureError
	if !errors.As(err, &testFailure) {
		t.Fatalf("error = %v, want an ordinary test failure", err)
	}
	var ledger flakeLedger
	data, _ := os.ReadFile(harness.ledger)
	if json.Unmarshal(data, &ledger) != nil || ledger.Outcome != flakeOutcomePersistent || ledger.Entries[0].RetryOutcome != "fail" {
		t.Fatalf("ledger = %s, want failed-after-retry", data)
	}
}

func TestExecuteCoverageInvocationPlanDoesNotRetryWhenDisabled(t *testing.T) {
	harness := newFlakeRunHarness(t, map[string]error{})
	t.Setenv(flakeRetryMaxEnv, "")
	err := executeCoverageInvocationPlan(config{}, flakePlan(), []string{"p/ok", "p/a"}, harness.profile, ".", nil, "run", nil)
	if err == nil || len(harness.calls) != 1 {
		t.Fatalf("error = %v calls = %d, want the original failure and no retry", err, len(harness.calls))
	}
	if _, statErr := os.Stat(harness.ledger); statErr == nil {
		t.Fatal("ledger written while the retry is disabled")
	}
}
