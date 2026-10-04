package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// useFixtures points the analyzers at the m/ fixture module and a fixture
// baseline for the duration of one test.
func useFixtures(t *testing.T, entries ...string) {
	t.Helper()
	oldPrefix, oldBaseline := modulePrefix, baseline
	modulePrefix = "m/"
	listed := ""
	for _, entry := range entries {
		listed += entry + "\n"
	}
	baseline = func() map[string]struct{} { return parseBaseline(listed) }
	t.Cleanup(func() { modulePrefix, baseline = oldPrefix, oldBaseline })
}

func TestLayeringReportsEachRule(t *testing.T) {
	useFixtures(t,
		"service-subpackage|pkg/services/listed|pkg/services/b/sub",
		"service-subpackage|pkg/services/stale|pkg/services/b/sub",
	)
	analysistest.Run(t, analysistest.TestData(), Layering,
		"m/pkg/services/a", "m/pkg/services/c", "m/pkg/services/d", "m/pkg/services/listed", "m/pkg/services/stale",
		"m/pkg/platform/p", "m/pkg/initializer/i",
	)
}

func TestBehaviorReportsEachRule(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Behavior,
		"m/pkg/initializer/ctor", "m/pkg/transports/mapping/mp",
	)
}

func TestEmbeddedBaselineParses(t *testing.T) {
	if entries := parseBaseline(baselineText); entries == nil {
		t.Fatal("baseline must parse")
	}
}
