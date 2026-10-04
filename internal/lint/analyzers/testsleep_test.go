package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestTestsleepClassifiesCompilerExpressions(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Testsleep,
		"m/pkg/timing", "m/pkg/timingdot", "m/tests/timinghelper", "m/internal/testutil/timinghelper")
}

func TestTestsleepPreservesExactGroupsAndRejectsStaleDebt(t *testing.T) {
	useFixtures(t,
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/debt_test.go::Moved::sleep::1",
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/debt_test.go::Moved::sleep::2",
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/debt_test.go::Reduced::sleep::1",
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/debt_test.go::Reduced::sleep::2",
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/deleted_test.go::Deleted::sleep::1",
		"testsleep-sleep-test|pkg/timingdebt|pkg/timingdebt/inactive_test.go::Inactive::sleep::1",
	)
	analysistest.Run(t, analysistest.TestData(), Testsleep, "m/pkg/timingdebt")
}

func TestTestsleepScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"pkg/service/a_test.go", true}, {"cmd/tool/a_test.go", true},
		{"tests/functional/helper.go", true}, {"internal/testutil/helper.go", true},
		{"pkg/service/helper.go", false}, {"internal/lint/testdata/sample_test.go", false},
		{"ui/sample_test.go", false},
	} {
		if got := timingScope(tc.path); got != tc.want {
			t.Errorf("timingScope(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestTimingFileRetainsRealTestSuffixDirectories(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ unit, file, want string }{
		{"pkg/a", "/repo/pkg/a/a_test.go", "pkg/a/a_test.go"},
		{"pkg/a_test", "/repo/pkg/a/a_test.go", "pkg/a/a_test.go"},
		{"tests/functional_test", "/repo/tests/functional_test/a_test.go", "tests/functional_test/a_test.go"},
		{"tests/functional_test_test", "/repo/tests/functional_test/a_test.go", "tests/functional_test/a_test.go"},
	} {
		if got := timingFile(tc.unit, tc.file); got != tc.want {
			t.Errorf("timingFile(%q, %q) = %q, want %q", tc.unit, tc.file, got, tc.want)
		}
	}
}
