package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestFunctionalCoverageSelectionSubtractsOnlyDeclaredSelectors(t *testing.T) {
	packageA := modulePath + "/tests/functional/alpha"
	packageB := modulePath + "/tests/functional/beta"
	packageEmpty := modulePath + "/tests/functional/empty"
	inventory := functionalTestInventory{
		Packages: []string{packageA, packageB, packageEmpty},
		Tests: map[string][]string{
			packageA:     {"TestAdded", "TestKeep", "TestDrop"},
			packageB:     {"TestNeverRuns"},
			packageEmpty: nil,
		},
	}
	manifest := functionalQuarantine{
		Version: functionalQuarantineVersion,
		Suite:   functionalSuiteName,
		Entries: []functionalQuarantineEntry{
			{Package: packageA, Test: "TestDrop", Bucket: functionalBucketEnvironment, Reason: "requires an opt-in dependency"},
			{Package: packageB, Bucket: functionalBucketEnvironment, Reason: "package has no Linux precondition"},
		},
	}

	if err := validateFunctionalQuarantine(manifest, inventory); err != nil {
		t.Fatalf("validateFunctionalQuarantine() error = %v", err)
	}
	selection, err := buildFunctionalCoverageSelection(manifest, inventory)
	if err != nil {
		t.Fatalf("buildFunctionalCoverageSelection() error = %v", err)
	}

	if selection.QuarantinedPackageCount != 1 || selection.QuarantinedTestSelectors != 1 {
		t.Fatalf("quarantine counts = packages:%d tests:%d, want 1/1", selection.QuarantinedPackageCount, selection.QuarantinedTestSelectors)
	}
	if selection.PackageExcludedTestCount != 1 {
		t.Fatalf("PackageExcludedTestCount = %d, want 1", selection.PackageExcludedTestCount)
	}
	if selection.TestExcludedPackageCount != 0 {
		t.Fatalf("TestExcludedPackageCount = %d, want 0", selection.TestExcludedPackageCount)
	}
	if selection.SelectedPackageCount != 2 || selection.SelectedTestCount != 2 {
		t.Fatalf("selected counts = packages:%d tests:%d, want 2/2", selection.SelectedPackageCount, selection.SelectedTestCount)
	}
	if got := selectedFunctionalPackages(selection); !slices.Equal(got, []string{packageA, packageEmpty}) {
		t.Fatalf("selected packages = %v, want package A and empty package", got)
	}
	if len(selection.Groups) != 2 {
		t.Fatalf("run groups = %+v, want one precise and one unrestricted group", selection.Groups)
	}
	precise := selection.Groups[1]
	if precise.RunPattern != "^(?:TestAdded|TestKeep)$" || !slices.Equal(precise.Packages, []string{packageA}) {
		t.Fatalf("precise run group = %+v, want package A with only retained tests", precise)
	}
}

func TestFunctionalQuarantineValidationFailsClosed(t *testing.T) {
	packagePath := modulePath + "/tests/functional/alpha"
	inventory := functionalTestInventory{
		Packages: []string{packagePath},
		Tests:    map[string][]string{packagePath: {"TestKnown"}},
	}
	tests := []struct {
		name    string
		entries []functionalQuarantineEntry
		want    string
	}{
		{
			name:    "unknown package",
			entries: []functionalQuarantineEntry{{Package: modulePath + "/tests/functional/missing", Bucket: functionalBucketEnvironment, Reason: "missing precondition"}},
			want:    "not discoverable",
		},
		{
			name:    "unknown test",
			entries: []functionalQuarantineEntry{{Package: packagePath, Test: "TestMissing", Bucket: functionalBucketEnvironment, Reason: "missing precondition"}},
			want:    "not discoverable",
		},
		{
			name: "duplicate",
			entries: []functionalQuarantineEntry{
				{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment, Reason: "one"},
				{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment, Reason: "two"},
			},
			want: "duplicate selector",
		},
		{
			name: "unsorted",
			entries: []functionalQuarantineEntry{
				{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment, Reason: "one"},
				{Package: packagePath, Bucket: functionalBucketEnvironment, Reason: "two"},
			},
			want: "sorted",
		},
		{
			name:    "unsupported bucket",
			entries: []functionalQuarantineEntry{{Package: packagePath, Test: "TestKnown", Bucket: "GREEN", Reason: "not a quarantine"}},
			want:    "unsupported bucket",
		},
		{
			name:    "unsupported measurement",
			entries: []functionalQuarantineEntry{{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment, Reason: "not measurable", Measurement: "randomized"}},
			want:    "unsupported measurement",
		},
		{
			name:    "package measurement requires test",
			entries: []functionalQuarantineEntry{{Package: packagePath, Bucket: functionalBucketEnvironment, Reason: "package context", Measurement: functionalMeasurementPackageContext}},
			want:    "requires a test selector",
		},
		{
			name:    "empty reason",
			entries: []functionalQuarantineEntry{{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment}},
			want:    "non-empty reason",
		},
		{
			name:    "failure follow up",
			entries: []functionalQuarantineEntry{{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketFailure, Reason: "known defect"}},
			want:    "followUp",
		},
		{
			name: "overlapping package and test",
			entries: []functionalQuarantineEntry{
				{Package: packagePath, Bucket: functionalBucketEnvironment, Reason: "whole package precondition"},
				{Package: packagePath, Test: "TestKnown", Bucket: functionalBucketEnvironment, Reason: "test precondition"},
			},
			want: "overlaps",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := functionalQuarantine{Version: functionalQuarantineVersion, Suite: functionalSuiteName, Entries: test.entries}
			err := validateFunctionalQuarantine(manifest, inventory)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateFunctionalQuarantine() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestReadFunctionalQuarantineRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "unknown field", data: `{"version":1,"suite":"functional","entries":[],"unexpected":true}`, want: "unknown field"},
		{name: "missing entries", data: `{"version":1,"suite":"functional"}`, want: "entries must be an array"},
		{name: "null entries", data: `{"version":1,"suite":"functional","entries":null}`, want: "entries must be an array"},
		{name: "trailing value", data: `{"version":1,"suite":"functional","entries":[]} {}`, want: "multiple JSON values"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "quarantine.json")
			if err := os.WriteFile(path, []byte(test.data), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			_, err := readFunctionalQuarantineFile(path)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readFunctionalQuarantineFile() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestDiscoverFunctionalTestInventoryUsesGoListProcessResult(t *testing.T) {
	originalRunner := commandRunner
	defer func() { commandRunner = originalRunner }()

	packageA := modulePath + "/tests/functional/alpha"
	packageB := modulePath + "/tests/functional/beta"
	var gotInvocation commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		gotInvocation = invocation
		return strings.Join([]string{
			marshalFunctionalListEvent(goTestListEvent{Action: "start", Package: packageB}),
			marshalFunctionalListEvent(goTestListEvent{Action: "output", Package: packageB, Output: "TestBeta\n"}),
			marshalFunctionalListEvent(goTestListEvent{Action: "skip", Package: packageB}),
			marshalFunctionalListEvent(goTestListEvent{Action: "start", Package: packageA}),
			marshalFunctionalListEvent(goTestListEvent{Action: "output", Package: packageA, Output: "TestAlpha\nTestAdded\n"}),
			marshalFunctionalListEvent(goTestListEvent{Action: "pass", Package: packageA}),
		}, "\n"), "", nil
	}

	inventory, err := discoverFunctionalTestInventory([]string{packageB, packageA}, 2*time.Minute, true, 4, t.TempDir())
	if err != nil {
		t.Fatalf("discoverFunctionalTestInventory() error = %v", err)
	}
	if !slices.Equal(inventory.Packages, []string{packageA, packageB}) {
		t.Fatalf("inventory packages = %v, want sorted package paths", inventory.Packages)
	}
	if !slices.Equal(inventory.Tests[packageA], []string{"TestAdded", "TestAlpha"}) || !slices.Equal(inventory.Tests[packageB], []string{"TestBeta"}) {
		t.Fatalf("inventory tests = %+v, want both package test lists", inventory.Tests)
	}
	if gotInvocation.name != "go" || gotInvocation.dir == "" || !slices.Contains(gotInvocation.args, "-list=^Test") || !slices.Contains(gotInvocation.args, "-json") || !slices.Contains(gotInvocation.args, "-short") {
		t.Fatalf("discovery invocation = %+v, want go test list/json/short process", gotInvocation)
	}
}

func TestBuildFunctionalCoverageInvocationPlanKeepsRunGroupsSeparate(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	plan, err := buildFunctionalCoverageInvocationPlan(
		[]string{"test"},
		[]coverageRunGroup{
			{Packages: []string{modulePath + "/tests/functional/alpha"}},
			{Packages: []string{modulePath + "/tests/functional/beta"}, RunPattern: "^(?:TestKeep)$"},
		},
		profilePath,
		false,
		"linux",
	)
	if err != nil {
		t.Fatalf("buildFunctionalCoverageInvocationPlan() error = %v", err)
	}
	defer plan.cleanup()
	if len(plan.invocations) != 2 {
		t.Fatalf("invocations = %+v, want one per run group", plan.invocations)
	}
	if slices.Contains(plan.invocations[0].args, "-run=^(?:TestKeep)$") {
		t.Fatalf("unrestricted group unexpectedly has a run selector: %+v", plan.invocations[0].args)
	}
	if !slices.Contains(plan.invocations[1].args, "-run=^(?:TestKeep)$") {
		t.Fatalf("precise group missing its run selector: %+v", plan.invocations[1].args)
	}
	if plan.profilePaths[0] == profilePath || plan.profilePaths[1] == profilePath {
		t.Fatalf("multi-group plan should isolate intermediate profiles: %v", plan.profilePaths)
	}
}

func marshalFunctionalListEvent(event goTestListEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func TestRenderFunctionalQuarantineFixtureIsStable(t *testing.T) {
	manifest := functionalQuarantine{
		Version: functionalQuarantineVersion,
		Suite:   functionalSuiteName,
		Entries: []functionalQuarantineEntry{{Package: modulePath + "/tests/functional/alpha", Test: "TestOne", Bucket: functionalBucketEnvironment, Reason: "requires an opt-in dependency"}},
	}
	first, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	second, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest second time: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("manifest JSON is not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestFunctionalQuarantineRatchetAcceptsExpectedOutcomesIndependently(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packageSkip := modulePath + "/tests/functional/alpha"
	packageTestSkip := modulePath + "/tests/functional/beta"
	packageFail := modulePath + "/tests/functional/gamma"
	manifest := functionalQuarantine{Entries: []functionalQuarantineEntry{
		{Package: packageSkip, Bucket: functionalBucketEnvironment, Reason: "requires an unavailable runtime"},
		{Package: packageTestSkip, Test: "TestOptIn", Bucket: functionalBucketEnvironment, Reason: "requires an opt-in credential"},
		{Package: packageFail, Bucket: functionalBucketFailure, Reason: "known defect", FollowUp: "issue-123"},
	}}

	var stdout bytes.Buffer
	stdoutWriter = &stdout
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		invocations = append(invocations, invocation)
		packagePath := invocation.args[len(invocation.args)-1]
		switch packagePath {
		case packageSkip:
			return marshalFunctionalTimingEvents(
				goTestTimingEvent{Action: "start", Package: packagePath},
				goTestTimingEvent{Action: timingOutcomeSkip, Package: packagePath},
			), "", nil
		case packageTestSkip:
			return marshalFunctionalTimingEvents(
				goTestTimingEvent{Action: "start", Package: packagePath},
				goTestTimingEvent{Action: "skip", Package: packagePath, Test: "TestOptIn"},
				goTestTimingEvent{Action: timingOutcomeSkip, Package: packagePath},
			), "", nil
		case packageFail:
			return marshalFunctionalTimingEvents(
				goTestTimingEvent{Action: "start", Package: packagePath},
				goTestTimingEvent{Action: "fail", Package: packagePath, Test: "TestKnown", Output: "--- FAIL: TestKnown (0.01s)\n"},
				goTestTimingEvent{Action: timingOutcomeFail, Package: packagePath, Output: "--- FAIL: TestKnown (0.01s)\n"},
			), "assertion failed", errors.New("exit status 1")
		default:
			return "", "", errors.New("unexpected package")
		}
	}

	if err := runFunctionalQuarantineRatchet(manifest, time.Minute, true, t.TempDir()); err != nil {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v", err)
	}
	if len(invocations) != len(manifest.Entries) {
		t.Fatalf("ratchet invocations = %d, want one independent invocation per entry", len(invocations))
	}
	if !slices.Contains(invocations[1].args, "-run=^(?:TestOptIn)$") {
		t.Fatalf("test selector invocation = %v, want exact -run selector", invocations[1].args)
	}
	for _, want := range []string{
		`selector="` + packageSkip + `" bucket=ENVIRONMENT-DEPENDENT expected=skip observed=skip status=expected`,
		`selector="` + packageTestSkip + `#TestOptIn" bucket=ENVIRONMENT-DEPENDENT expected=skip observed=skip status=expected`,
		`selector="` + packageFail + `" bucket=GENUINELY FAILING expected=fail observed=fail status=expected`,
		"Functional quarantine ratchet: selectors=3 observed-pass=0 observed-fail=1 observed-skip=2 execution-errors=0",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("ratchet stdout = %q, want substring %q", stdout.String(), want)
		}
	}
}

func TestFunctionalQuarantineRatchetTreatsShortSkipAsUnmeasurableForFailureBucket(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/shortskipped"
	entry := functionalQuarantineEntry{
		Package:  packagePath,
		Test:     "TestKnownFailureGuardedByShort",
		Bucket:   functionalBucketFailure,
		Reason:   "known defect; reproduced on the same machine at tip and parent",
		FollowUp: "fix the defect and remove this entry",
	}

	var stdout bytes.Buffer
	stdoutWriter = &stdout
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		return marshalFunctionalTimingEvents(
			goTestTimingEvent{Action: "start", Package: packagePath},
			goTestTimingEvent{Action: "skip", Package: packagePath, Test: entry.Test},
			goTestTimingEvent{Action: timingOutcomeSkip, Package: packagePath},
		), "", nil
	}

	manifest := functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}

	// short=true is the pull-request tier. The selector self-skips under
	// testing.Short(), so whether it fails cannot be observed and must not be
	// reported as a bucket violation.
	if err := runFunctionalQuarantineRatchet(manifest, time.Minute, true, t.TempDir()); err != nil {
		t.Fatalf("runFunctionalQuarantineRatchet(short=true) error = %v, want a GENUINELY FAILING entry to survive an unmeasurable short-mode skip", err)
	}
	wantStatus := `bucket=GENUINELY FAILING expected=fail observed=skip status=unmeasurable-under-short`
	if !strings.Contains(stdout.String(), wantStatus) {
		t.Fatalf("ratchet stdout = %q, want substring %q", stdout.String(), wantStatus)
	}

	// short=false is the push tier, where the selector does execute. A skip
	// there is a genuine bucket mismatch and must still fail closed.
	stdout.Reset()
	err := runFunctionalQuarantineRatchet(manifest, time.Minute, false, t.TempDir())
	if err == nil {
		t.Fatal("runFunctionalQuarantineRatchet(short=false) error = nil, want an unexpected-outcome failure when the selector skips on the full tier")
	}
	if !strings.Contains(err.Error(), "observed skip, expected fail") {
		t.Fatalf("runFunctionalQuarantineRatchet(short=false) error = %v, want an observed/expected mismatch diagnostic", err)
	}
	if !strings.Contains(stdout.String(), "status=unexpected-outcome") {
		t.Fatalf("ratchet stdout = %q, want status=unexpected-outcome on the full tier", stdout.String())
	}
}

func TestFunctionalQuarantineRatchetTreatsFlakyPassAsUnmeasurable(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/flaky"
	entry := functionalQuarantineEntry{
		Package:     packagePath,
		Test:        "TestProbabilisticFailure",
		Bucket:      functionalBucketFailure,
		Reason:      "repeated Linux evidence shows isolated flakiness",
		FollowUp:    "repair the flaky test and remove this entry",
		Measurement: functionalMeasurementFlaky,
	}
	var stdout bytes.Buffer
	stdoutWriter = &stdout
	var gotInvocation commandInvocation
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		gotInvocation = invocation
		return marshalFunctionalTimingEvents(
			goTestTimingEvent{Action: "start", Package: packagePath},
			goTestTimingEvent{Action: "run", Package: packagePath, Test: entry.Test},
			goTestTimingEvent{Action: timingOutcomePass, Package: packagePath, Test: entry.Test},
			goTestTimingEvent{Action: timingOutcomePass, Package: packagePath},
		), "", nil
	}

	err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, false, t.TempDir())
	if err != nil {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want a flaky pass to remain unmeasurable", err)
	}
	if !slices.Contains(gotInvocation.args, "-run=^(?:TestProbabilisticFailure)$") {
		t.Fatalf("flaky invocation = %v, want an exact isolated selector", gotInvocation.args)
	}
	wantStatus := `bucket=GENUINELY FAILING expected=fail observed=pass status=unmeasurable measurement=flaky`
	if !strings.Contains(stdout.String(), wantStatus) {
		t.Fatalf("ratchet stdout = %q, want substring %q", stdout.String(), wantStatus)
	}
	if strings.Contains(stdout.String(), "unexpected-pass") {
		t.Fatalf("flaky pass was reported as unexpected-pass: %q", stdout.String())
	}
}

func TestFunctionalQuarantineRatchetPackageContextUsesTargetOutcome(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/package-context"
	entry := functionalQuarantineEntry{
		Package:     packagePath,
		Test:        "TestTarget",
		Bucket:      functionalBucketFailure,
		Reason:      "failure depends on package interaction",
		FollowUp:    "repair the interaction and remove this entry",
		Measurement: functionalMeasurementPackageContext,
	}
	var stdout bytes.Buffer
	stdoutWriter = &stdout

	t.Run("target pass remains fatal despite sibling failure", func(t *testing.T) {
		var gotInvocation commandInvocation
		commandRunner = func(invocation commandInvocation) (string, string, error) {
			gotInvocation = invocation
			return marshalFunctionalTimingEvents(
				goTestTimingEvent{Action: "start", Package: packagePath},
				goTestTimingEvent{Action: "run", Package: packagePath, Test: "TestSibling"},
				goTestTimingEvent{Action: timingOutcomeFail, Package: packagePath, Test: "TestSibling", Output: "sibling failed"},
				goTestTimingEvent{Action: "run", Package: packagePath, Test: entry.Test},
				goTestTimingEvent{Action: timingOutcomePass, Package: packagePath, Test: entry.Test},
				goTestTimingEvent{Action: timingOutcomeFail, Package: packagePath, Output: "package failed"},
			), "", errors.New("exit status 1")
		}
		stdout.Reset()
		err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, false, t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "passed unexpectedly") {
			t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want target unexpected-pass", err)
		}
		if slices.ContainsFunc(gotInvocation.args, func(arg string) bool { return strings.HasPrefix(arg, "-run=") }) {
			t.Fatalf("package-context invocation unexpectedly filtered the package: %v", gotInvocation.args)
		}
		if !strings.Contains(stdout.String(), "status=unexpected-pass measurement=package-context") {
			t.Fatalf("ratchet stdout = %q, want package-context unexpected-pass", stdout.String())
		}
	})

	t.Run("target failure is expected", func(t *testing.T) {
		commandRunner = func(commandInvocation) (string, string, error) {
			return marshalFunctionalTimingEvents(
				goTestTimingEvent{Action: "start", Package: packagePath},
				goTestTimingEvent{Action: "run", Package: packagePath, Test: "TestSibling"},
				goTestTimingEvent{Action: timingOutcomePass, Package: packagePath, Test: "TestSibling"},
				goTestTimingEvent{Action: "run", Package: packagePath, Test: entry.Test},
				goTestTimingEvent{Action: timingOutcomeFail, Package: packagePath, Test: entry.Test, Output: "target failed"},
				goTestTimingEvent{Action: timingOutcomeFail, Package: packagePath, Output: "package failed"},
			), "", errors.New("exit status 1")
		}
		stdout.Reset()
		if err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, false, t.TempDir()); err != nil {
			t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want target failure to be expected", err)
		}
		if !strings.Contains(stdout.String(), "observed=fail status=expected measurement=package-context") {
			t.Fatalf("ratchet stdout = %q, want expected target failure", stdout.String())
		}
	})
}

func TestFunctionalQuarantineRatchetRejectsInvalidMeasurementBeforeExecution(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	entry := functionalQuarantineEntry{
		Package:     modulePath + "/tests/functional/invalid-measurement",
		Test:        "TestKnown",
		Bucket:      functionalBucketFailure,
		Reason:      "invalid metadata fixture",
		FollowUp:    "remove the invalid metadata",
		Measurement: "unsupported",
	}
	stdoutWriter = &bytes.Buffer{}
	commandRunner = func(commandInvocation) (string, string, error) {
		t.Fatal("invalid measurement metadata unexpectedly executed go test")
		return "", "", nil
	}

	err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, false, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "invalid measurement metadata") || !strings.Contains(err.Error(), "unsupported measurement") {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want fail-closed invalid metadata diagnostic", err)
	}
}

func TestFunctionalQuarantineRatchetRejectsUnexpectedPass(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/recovered"
	entry := functionalQuarantineEntry{Package: packagePath, Test: "TestRecovered", Bucket: functionalBucketEnvironment, Reason: "requires an unavailable runtime"}
	var stdout bytes.Buffer
	stdoutWriter = &stdout
	commandRunner = func(commandInvocation) (string, string, error) {
		return marshalFunctionalTimingEvents(
			goTestTimingEvent{Action: "start", Package: packagePath},
			goTestTimingEvent{Action: "run", Package: packagePath, Test: entry.Test},
			goTestTimingEvent{Action: timingOutcomePass, Package: packagePath, Test: entry.Test},
			goTestTimingEvent{Action: timingOutcomePass, Package: packagePath},
		), "", nil
	}

	err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, true, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "passed unexpectedly") || !strings.Contains(err.Error(), "remove or narrow") {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want actionable unexpected-pass failure", err)
	}
	if !strings.Contains(stdout.String(), "status=unexpected-pass") {
		t.Fatalf("ratchet stdout = %q, want unexpected-pass diagnostic", stdout.String())
	}
}

func TestFunctionalQuarantineRatchetRejectsStaleSelector(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/stale"
	entry := functionalQuarantineEntry{Package: packagePath, Test: "TestMissingAtRuntime", Bucket: functionalBucketEnvironment, Reason: "requires an unavailable runtime"}
	stdoutWriter = &bytes.Buffer{}
	commandRunner = func(commandInvocation) (string, string, error) {
		return marshalFunctionalTimingEvents(
			goTestTimingEvent{Action: "start", Package: packagePath},
			goTestTimingEvent{Action: timingOutcomePass, Package: packagePath},
		), "", nil
	}

	err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, true, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no terminal outcome") || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want stale-selector failure", err)
	}
}

func TestFunctionalQuarantineRatchetRejectsExecutionError(t *testing.T) {
	originalRunner := commandRunner
	originalStdout := stdoutWriter
	t.Cleanup(func() {
		commandRunner = originalRunner
		stdoutWriter = originalStdout
	})

	packagePath := modulePath + "/tests/functional/infra-error"
	entry := functionalQuarantineEntry{Package: packagePath, Bucket: functionalBucketEnvironment, Reason: "requires an unavailable runtime"}
	stdoutWriter = &bytes.Buffer{}
	commandRunner = func(commandInvocation) (string, string, error) {
		return "", "compiler unavailable", errors.New("exit status 2")
	}

	err := runFunctionalQuarantineRatchet(functionalQuarantine{Entries: []functionalQuarantineEntry{entry}}, time.Minute, true, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "execution error") || !strings.Contains(err.Error(), "fail-closed") || !strings.Contains(err.Error(), "compiler unavailable") {
		t.Fatalf("runFunctionalQuarantineRatchet() error = %v, want fail-closed execution diagnostic", err)
	}
}

func marshalFunctionalTimingEvents(events ...goTestTimingEvent) string {
	lines := make([]string, 0, len(events))
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			panic(err)
		}
		lines = append(lines, string(data))
	}
	return strings.Join(lines, "\n")
}
