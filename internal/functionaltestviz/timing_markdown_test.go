package functionaltestviz_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestRenderFunctionalTimingMarkdownCompleteSummary(t *testing.T) {
	t.Parallel()

	summary := functionaltestviz.FunctionalTimingSummary{
		Version:                  functionaltestviz.FunctionalTimingSummaryVersion,
		Complete:                 true,
		WallSeconds:              5.0,
		PackageElapsedSecondsSum: 7.0,
		ExpectedPackageCount:     2,
		PackageCount:             2,
		TestCount:                3,
		TestPassCount:            1,
		TestFailCount:            1,
		TestSkipCount:            1,
		Packages: []functionaltestviz.FunctionalPackageTiming{
			{Package: "github.com/portpowered/infinite-you/tests/functional/beta", Seconds: 4.0, Outcome: "pass"},
			{Package: "github.com/portpowered/infinite-you/tests/functional/alpha", Seconds: 3.0, Outcome: "fail"},
		},
		Tests: []functionaltestviz.FunctionalTestTiming{
			{Package: "github.com/portpowered/infinite-you/tests/functional/alpha", Test: "TestBroken", Seconds: 3.0, Outcome: "fail", Reason: "assertion failed"},
			{Package: "github.com/portpowered/infinite-you/tests/functional/alpha", Test: "TestGreen", Seconds: 0.0, Outcome: "pass"},
			{Package: "github.com/portpowered/infinite-you/tests/functional/alpha", Test: "TestSkipped", Seconds: 0.0, Outcome: "skip"},
		},
	}

	first := functionaltestviz.RenderFunctionalTimingMarkdown(summary)
	second := functionaltestviz.RenderFunctionalTimingMarkdown(summary)
	if first != second {
		t.Fatalf("repeated timing renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if !strings.HasPrefix(first, "## Functional test timings\n\n") {
		t.Fatalf("timing section heading missing:\n%s", first)
	}
	if !strings.Contains(first, "- Total wall-clock duration: 5.000s\n") {
		t.Fatalf("wall-clock duration missing:\n%s", first)
	}
	if !strings.Contains(first, "- Package elapsed total: 7.000s") {
		t.Fatalf("package elapsed total missing:\n%s", first)
	}
	if !strings.Contains(first, "concurrent") {
		t.Fatalf("concurrency explanation missing:\n%s", first)
	}
	if !strings.Contains(first, "- Package count: 2\n") {
		t.Fatalf("package count missing:\n%s", first)
	}
	if !strings.Contains(first, "- Discovered functional packages: 2\n") ||
		!strings.Contains(first, "- Top-level tests with observed outcomes: 3\n") ||
		!strings.Contains(first, "- Top-level test outcomes: pass=1, fail=1, skip=1\n") {
		t.Fatalf("top-level inventory counts missing:\n%s", first)
	}
	if !strings.Contains(first, "### Failed top-level tests") ||
		!strings.Contains(first, "| `github.com/portpowered/infinite-you/tests/functional/alpha` | `TestBroken` | 3.000 | assertion failed |") {
		t.Fatalf("failed top-level test diagnostics missing:\n%s", first)
	}

	alphaIdx := strings.Index(first, "| `github.com/portpowered/infinite-you/tests/functional/alpha` |")
	betaIdx := strings.Index(first, "| `github.com/portpowered/infinite-you/tests/functional/beta` |")
	if alphaIdx < 0 || betaIdx < 0 || alphaIdx >= betaIdx {
		t.Fatalf("packages must render in stable path order regardless of input order:\n%s", first)
	}
	if !strings.Contains(first, "| `github.com/portpowered/infinite-you/tests/functional/alpha` | 3.000 | fail |") {
		t.Fatalf("alpha row missing expected values:\n%s", first)
	}
	if !strings.Contains(first, "| `github.com/portpowered/infinite-you/tests/functional/beta` | 4.000 | pass |") {
		t.Fatalf("beta row missing expected values:\n%s", first)
	}
}

func TestRenderFunctionalTimingMarkdownIncompleteSummary(t *testing.T) {
	t.Parallel()

	summary := functionaltestviz.FunctionalTimingSummary{
		Version:                  functionaltestviz.FunctionalTimingSummaryVersion,
		Complete:                 false,
		WallSeconds:              1.0,
		PackageElapsedSecondsSum: 1.0,
		PackageCount:             1,
		Packages: []functionaltestviz.FunctionalPackageTiming{
			{Package: "github.com/portpowered/infinite-you/tests/functional/alpha", Seconds: 1.0, Outcome: "pass"},
		},
	}

	got := functionaltestviz.RenderFunctionalTimingMarkdown(summary)
	if !strings.Contains(got, "incomplete") {
		t.Fatalf("incomplete capture must be explicitly labeled, not rendered as a successful report:\n%s", got)
	}
}

func TestRenderFunctionalTimingMarkdownEmptyPackages(t *testing.T) {
	t.Parallel()

	got := functionaltestviz.RenderFunctionalTimingMarkdown(functionaltestviz.FunctionalTimingSummary{
		Version:      functionaltestviz.FunctionalTimingSummaryVersion,
		Complete:     true,
		WallSeconds:  0,
		PackageCount: 0,
		Packages:     []functionaltestviz.FunctionalPackageTiming{},
	})
	if !strings.Contains(got, "- _No functional-test packages in timing summary._\n") {
		t.Fatalf("empty packages presentation missing:\n%s", got)
	}
	if strings.Contains(got, "| Package |") {
		t.Fatalf("empty packages must not render a table:\n%s", got)
	}
}

func TestRenderFunctionalTimingMarkdownZeroDurationPackagesNotHiddenAsMissing(t *testing.T) {
	t.Parallel()

	summary := functionaltestviz.FunctionalTimingSummary{
		Version:      functionaltestviz.FunctionalTimingSummaryVersion,
		Complete:     true,
		WallSeconds:  0.5,
		PackageCount: 1,
		Packages: []functionaltestviz.FunctionalPackageTiming{
			{Package: "github.com/portpowered/infinite-you/tests/functional/empty", Seconds: 0, Outcome: "skip"},
		},
	}
	got := functionaltestviz.RenderFunctionalTimingMarkdown(summary)
	if !strings.Contains(got, "| `github.com/portpowered/infinite-you/tests/functional/empty` | 0.000 | skip |") {
		t.Fatalf("zero-duration package must still render, not be treated as missing:\n%s", got)
	}
}
