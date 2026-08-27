package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestCollectUnitTimingCapturePreservesOutcomesAndCacheEvidence(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/pkg/alpha"
	pkgB := modulePath + "/pkg/beta"
	pkgC := modulePath + "/pkg/gamma"
	stream := strings.Join([]string{
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "output", Package: pkgB, Output: "ok  \t" + pkgB + "\t(cached)\n"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkgB, Elapsed: 0}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "output", Package: pkgA, Output: "ok  \t" + pkgA + "\t0.750s\n"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "fail", Package: pkgA, Elapsed: 0.75}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "skip", Package: pkgC, Elapsed: 0}),
		"",
	}, "\n")

	var output bytes.Buffer
	capture, err := collectUnitTimingCapture(strings.NewReader(stream), []string{pkgA, pkgB, pkgC}, &output)
	if err != nil {
		t.Fatalf("collectUnitTimingCapture() error = %v", err)
	}
	if !capture.Complete {
		t.Fatalf("capture.Complete = false, want true: %+v", capture)
	}
	if got := len(capture.Packages); got != 3 {
		t.Fatalf("captured package count = %d, want 3", got)
	}
	if capture.Packages[0].Cache != unitCacheExecuted || capture.Packages[0].Outcome != unitTimingOutcomeFail {
		t.Fatalf("first package = %+v, want executed/fail for %s", capture.Packages[0], pkgA)
	}
	if capture.Packages[1].Cache != unitCacheCached || capture.Packages[1].Outcome != unitTimingOutcomePass {
		t.Fatalf("second package = %+v, want cached/pass for %s", capture.Packages[1], pkgB)
	}
	if !strings.Contains(output.String(), "ok  \t"+pkgB+"\t(cached)") || !strings.Contains(output.String(), "ok  \t"+pkgA+"\t0.750s") {
		t.Fatalf("replayed output = %q, want package output preserved", output.String())
	}
}

func TestCollectUnitTimingCaptureRecordsSortedTestInventory(t *testing.T) {
	t.Parallel()

	pkg := modulePath + "/pkg/alpha"
	stream := strings.Join([]string{
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "run", Package: pkg, Test: "TestZulu"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "run", Package: pkg, Test: "TestAlpha/sub"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Test: "TestZulu", Elapsed: 0.2}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Test: "TestAlpha/sub", Elapsed: 0.1}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "output", Package: pkg, Output: "ok  \t" + pkg + "\t0.200s\n"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Elapsed: 0.2}),
		"",
	}, "\n")

	capture, err := collectUnitTimingCapture(strings.NewReader(stream), []string{pkg}, ioDiscard{})
	if err != nil {
		t.Fatalf("collectUnitTimingCapture() error = %v", err)
	}
	if !capture.Complete || len(capture.Packages) != 1 {
		t.Fatalf("capture = %+v, want complete package/test capture", capture)
	}
	if got, want := capture.Packages[0].Tests, []string{"TestAlpha/sub", "TestZulu"}; !slices.Equal(got, want) {
		t.Fatalf("test inventory = %v, want %v", got, want)
	}
}

func TestCollectUnitTimingCaptureDeduplicatesRepeatedTestNamesAcrossTestPackages(t *testing.T) {
	t.Parallel()

	pkg := modulePath + "/pkg/alpha"
	stream := strings.Join([]string{
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Test: "TestSharedName", Elapsed: 0.1}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Test: "TestSharedName", Elapsed: 0.1}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "output", Package: pkg, Output: "ok  \t" + pkg + "\t0.100s\n"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkg, Elapsed: 0.1}),
		"",
	}, "\n")

	capture, err := collectUnitTimingCapture(strings.NewReader(stream), []string{pkg}, ioDiscard{})
	if err != nil {
		t.Fatalf("collectUnitTimingCapture() error = %v", err)
	}
	if !capture.Complete || len(capture.Packages) != 1 {
		t.Fatalf("capture = %+v, want complete package capture", capture)
	}
	if got, want := capture.Packages[0].Tests, []string{"TestSharedName"}; !slices.Equal(got, want) {
		t.Fatalf("test inventory = %v, want one deduplicated test name", got)
	}
}

func TestCollectUnitTimingCaptureMarksMalformedAndTruncatedDataIncomplete(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/pkg/alpha"
	pkgB := modulePath + "/pkg/beta"
	stream := strings.Join([]string{
		"{malformed event",
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "pass", Package: pkgA, Elapsed: 1.25}),
		"",
	}, "\n")

	var output bytes.Buffer
	capture, err := collectUnitTimingCapture(strings.NewReader(stream), []string{pkgA, pkgB}, &output)
	if err != nil {
		t.Fatalf("collectUnitTimingCapture() error = %v", err)
	}
	if capture.Complete {
		t.Fatal("capture.Complete = true, want false for malformed and missing terminal events")
	}
	if len(capture.Packages) != 1 || capture.Packages[0].Package != pkgA {
		t.Fatalf("capture.Packages = %+v, want partial alpha evidence", capture.Packages)
	}
	if !strings.Contains(output.String(), "{malformed event") {
		t.Fatalf("malformed event was not preserved in output: %q", output.String())
	}
}

func TestCollectUnitTimingCaptureRetainsFailedPackageTiming(t *testing.T) {
	t.Parallel()

	pkg := modulePath + "/pkg/failing"
	stream := strings.Join([]string{
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "output", Package: pkg, Output: "--- FAIL: TestFailure\n"}),
		unitTimingEventLine(t, goTestUnitTimingEvent{Action: "fail", Package: pkg, Elapsed: 0.4}),
		"",
	}, "\n")

	capture, err := collectUnitTimingCapture(strings.NewReader(stream), []string{pkg}, ioDiscard{})
	if err != nil {
		t.Fatalf("collectUnitTimingCapture() error = %v", err)
	}
	if !capture.Complete || len(capture.Packages) != 1 {
		t.Fatalf("capture = %+v, want one complete failed package", capture)
	}
	if capture.Packages[0].Outcome != unitTimingOutcomeFail || capture.Packages[0].Seconds != 0.4 {
		t.Fatalf("failed package = %+v, want fail/0.4", capture.Packages[0])
	}
}

func TestUnitTimingAccumulatorRanksMultiBatchPackagesDeterministically(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/pkg/alpha"
	pkgB := modulePath + "/pkg/beta"
	pkgC := modulePath + "/pkg/gamma"
	accumulator := newUnitTimingAccumulator([]string{pkgA, pkgB, pkgC})
	accumulator.add(unitTimingCapture{
		Complete: true,
		Packages: []unitPackageTiming{
			{Package: pkgB, Seconds: 2, Outcome: unitTimingOutcomePass, Cache: unitCacheExecuted},
			{Package: pkgA, Seconds: 2, Outcome: unitTimingOutcomePass, Cache: unitCacheCached},
		},
	})
	accumulator.add(unitTimingCapture{
		Complete: true,
		Packages: []unitPackageTiming{
			{Package: pkgC, Seconds: 0.5, Outcome: unitTimingOutcomeSkip, Cache: unitCacheUnknown},
		},
	})

	summary := accumulator.summaryWithRun(1.75, unitTimingRun{})
	if !summary.Complete || summary.PackageCount != 3 || summary.ExpectedPackageCount != 3 {
		t.Fatalf("summary = %+v, want complete three-package capture", summary)
	}
	if got := []string{summary.Packages[0].Package, summary.Packages[1].Package, summary.Packages[2].Package}; !slices.Equal(got, []string{pkgA, pkgB, pkgC}) {
		t.Fatalf("ranked packages = %v, want deterministic elapsed/tie order", got)
	}
	if summary.PackageElapsedSecondsSum != 4.5 || summary.WallSeconds != 1.75 {
		t.Fatalf("summary durations = wall %v sum %v, want wall 1.75 sum 4.5", summary.WallSeconds, summary.PackageElapsedSecondsSum)
	}
}

func TestRenderUnitTimingSummaryJSONIsDeterministic(t *testing.T) {
	t.Parallel()

	summary := unitTimingSummary{
		Version:                  unitTimingSummaryVersion,
		Complete:                 true,
		WallSeconds:              1.234,
		PackageElapsedSecondsSum: 2.5,
		PackageCount:             1,
		ExpectedPackageCount:     1,
		Packages: []unitPackageTiming{{
			Package: modulePath + "/pkg/alpha",
			Seconds: 2.5,
			Outcome: unitTimingOutcomePass,
			Cache:   unitCacheExecuted,
		}},
	}

	first, err := renderUnitTimingSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderUnitTimingSummaryJSON() error = %v", err)
	}
	second, err := renderUnitTimingSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderUnitTimingSummaryJSON() second error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("summary JSON is not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	var decoded unitTimingSummary
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("summary JSON decode error = %v", err)
	}
	if decoded.Version != unitTimingSummaryVersion || decoded.PackageCount != 1 || decoded.Packages[0].Cache != unitCacheExecuted {
		t.Fatalf("decoded summary = %+v, want versioned cache-aware shape", decoded)
	}
}

func TestRenderUnitTimingSummaryJSONSortsPackagesAndTests(t *testing.T) {
	t.Parallel()

	summary := unitTimingSummary{
		Version:              unitTimingSummaryVersion,
		Complete:             true,
		Run:                  unitTimingRun{Command: "make test-unit-fresh", EnvironmentInvalidations: []string{}},
		WallSeconds:          1,
		PackageCount:         2,
		ExpectedPackageCount: 2,
		Packages: []unitPackageTiming{
			{Package: modulePath + "/pkg/zulu", Tests: []string{"TestZulu", "TestAlpha"}},
			{Package: modulePath + "/pkg/alpha", Tests: []string{"TestZulu"}},
		},
	}

	data, err := renderUnitTimingSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderUnitTimingSummaryJSON() error = %v", err)
	}
	var decoded unitTimingSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode rendered JSON: %v", err)
	}
	if got := []string{decoded.Packages[0].Package, decoded.Packages[1].Package}; !slices.IsSorted(got) {
		t.Fatalf("JSON package order = %v, want sorted order", got)
	}
	if got := decoded.Packages[1].Tests; !slices.IsSorted(got) {
		t.Fatalf("JSON test order = %v, want sorted order", got)
	}
}

func TestWriteUnitTimingSummaryJSONReportsFilesystemFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	parent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	err := writeUnitTimingSummaryJSON(filepath.Join(parent, "timing.json"), unitTimingSummary{Version: unitTimingSummaryVersion})
	if err == nil || !strings.Contains(err.Error(), "create unit timing output directory") {
		t.Fatalf("writeUnitTimingSummaryJSON() error = %v, want actionable directory failure", err)
	}
}

func TestRunUnitTestsWritesCompleteTimingArtifactAndReadableOutput(t *testing.T) {
	restoreExecCommand(t)
	originalStdout := stdoutWriter
	t.Cleanup(func() { stdoutWriter = originalStdout })

	var output bytes.Buffer
	stdoutWriter = &output
	execCommand = fakeUnitLaneCommand
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_TIMING_JSON", "1")
	t.Setenv("UNIT_TIMING_COMMIT", strings.Repeat("a", 40))
	t.Setenv("UNIT_RUNNER_PROVIDER", "test-provider")
	t.Setenv("UNIT_RUNNER_IMAGE", "test-image")
	t.Setenv("UNIT_RUNNER_IMAGE_VERSION", "test-image-version")
	t.Setenv("UNIT_RUNNER_CPU_MODEL", "test-cpu")
	t.Setenv("UNIT_TIMING_INVALIDATIONS", "z-invalid,a-invalid,z-invalid")
	outputPath := filepath.Join(t.TempDir(), "unit-timing.json")

	err := runUnitTests(config{
		jobs:               2,
		short:              true,
		timeout:            2 * time.Minute,
		timingOutput:       outputPath,
		timingCommand:      "make test-unit-fresh UNIT_TIMING_OUTPUT=timing.json",
		computedLaneBudget: 2,
	}, []string{modulePath + "/pkg/alpha"})
	if err != nil {
		t.Fatalf("runUnitTests() error = %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read timing artifact: %v", err)
	}
	var summary unitTimingSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode timing artifact: %v\n%s", err, data)
	}
	if !summary.Complete || summary.PackageCount != 1 || summary.TestCount != 1 || summary.Packages[0].Cache != unitCacheExecuted {
		t.Fatalf("summary = %+v, want complete executed package", summary)
	}
	if summary.Version != 2 || summary.Run.Commit != strings.Repeat("a", 40) || summary.Run.GoVersion == "" || summary.Run.Command != "make test-unit-fresh UNIT_TIMING_OUTPUT=timing.json" || summary.Run.UnitDefaultJobs != 2 || summary.Run.ComputedLaneBudget != 2 || summary.Run.Runner.Provider != "test-provider" || summary.Run.Runner.Image != "test-image" || summary.Run.Runner.ImageVersion != "test-image-version" || summary.Run.Runner.OS == "" || summary.Run.Runner.Architecture == "" || summary.Run.Runner.CPUModel != "test-cpu" || summary.Run.EnvironmentInvalidations == nil || !slices.Equal(summary.Run.EnvironmentInvalidations, []string{"a-invalid", "z-invalid"}) {
		t.Fatalf("summary identity = %+v, want complete v2 run identity", summary.Run)
	}
	if !slices.Equal(summary.Packages[0].Tests, []string{"TestHelper"}) {
		t.Fatalf("package test inventory = %v, want TestHelper", summary.Packages[0].Tests)
	}
	if !strings.Contains(output.String(), "ok  \t"+modulePath+"/pkg/alpha") || !strings.Contains(output.String(), "Unit lane timing summary") {
		t.Fatalf("readable output = %q, want test output and timing summary", output.String())
	}
}

func TestWriteUnitTimingSummaryJSONReplacesOutputAtomically(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "unit-timing.json")
	first := unitTimingSummary{Version: unitTimingSummaryVersion, WallSeconds: 1}
	second := unitTimingSummary{Version: unitTimingSummaryVersion, WallSeconds: 2}
	if err := writeUnitTimingSummaryJSON(path, first); err != nil {
		t.Fatalf("write first summary: %v", err)
	}
	if err := writeUnitTimingSummaryJSON(path, second); err != nil {
		t.Fatalf("replace summary: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replaced summary: %v", err)
	}
	var decoded unitTimingSummary
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode replaced summary: %v", err)
	}
	if decoded.WallSeconds != second.WallSeconds {
		t.Fatalf("replaced wallSeconds = %v, want %v", decoded.WallSeconds, second.WallSeconds)
	}
}

func TestRunUnitTestsWritesFailedTimingArtifactAndReturnsFailure(t *testing.T) {
	restoreExecCommand(t)
	originalStdout := stdoutWriter
	t.Cleanup(func() { stdoutWriter = originalStdout })

	var output bytes.Buffer
	stdoutWriter = &output
	execCommand = fakeUnitLaneCommand
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_TIMING_JSON", "1")
	t.Setenv("UNITLANE_HELPER_TEST_FAIL", "1")
	outputPath := filepath.Join(t.TempDir(), "unit-timing.json")

	err := runUnitTests(config{jobs: 2, short: true, timeout: 2 * time.Minute, timingOutput: outputPath}, []string{modulePath + "/pkg/failing"})
	if err == nil || !strings.Contains(err.Error(), "exit status 2") {
		t.Fatalf("runUnitTests() error = %v, want test failure", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read failed timing artifact: %v", err)
	}
	var summary unitTimingSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode failed timing artifact: %v", err)
	}
	if !summary.Complete || summary.Packages[0].Outcome != unitTimingOutcomeFail {
		t.Fatalf("summary = %+v, want complete failed package timing", summary)
	}
	if !strings.Contains(output.String(), "FAIL") {
		t.Fatalf("failed test output = %q, want failure output preserved", output.String())
	}
}

func TestRunUnitTestsReportsTimingWriteFailure(t *testing.T) {
	restoreExecCommand(t)
	originalStdout := stdoutWriter
	t.Cleanup(func() { stdoutWriter = originalStdout })

	var output bytes.Buffer
	stdoutWriter = &output
	execCommand = fakeUnitLaneCommand
	t.Setenv("GO_WANT_UNITLANE_HELPER", "1")
	t.Setenv("UNITLANE_HELPER_TIMING_JSON", "1")
	root := t.TempDir()
	parent := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}

	err := runUnitTests(config{jobs: 2, short: true, timeout: 2 * time.Minute, timingOutput: filepath.Join(parent, "unit-timing.json")}, []string{modulePath + "/pkg/alpha"})
	if err == nil || !strings.Contains(err.Error(), "create unit timing output directory") {
		t.Fatalf("runUnitTests() error = %v, want actionable timing-write failure", err)
	}
}

func unitTimingEventLine(t *testing.T, event goTestUnitTimingEvent) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal unit timing event: %v", err)
	}
	return string(data)
}

type ioDiscard struct{}

func (ioDiscard) Write(data []byte) (int, error) {
	return len(data), nil
}
