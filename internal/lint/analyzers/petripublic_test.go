package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// These compiler fixtures share modulePrefix/baseline overrides and remain
// serialized, like the other package-owned analyzer fixtures.
func TestPetriPublicRecursiveTypes(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Petripublic,
		"m/pkg/services/factory_runtime/petrifixture", "m/pkg/petriconsumer")
}

func TestPetriPublicIdentityAndInternalScope(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Petripublic,
		"m/pkg/services/factory_runtime", "m/pkg/services/factory_runtime/internal/petriallowed",
		"m/pkg/petrilookalike")
}

func TestPetriPublicCyclesAndMethods(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Petripublic,
		"m/pkg/services/factory_runtime/petrimethods")
}

func TestPetriPublicDebtAndStale(t *testing.T) {
	useFixtures(t,
		"petri-public|pkg/petridebt|Listed|"+petriPackage+".Marking",
		"petri-public|pkg/petristale|Removed|"+petriPackage+".Marking")
	analysistest.Run(t, analysistest.TestData(), Petripublic, "m/pkg/petridebt", "m/pkg/petristale")
}

func TestPetriPublicDefersTaggedStale(t *testing.T) {
	useFixtures(t, "petri-public|pkg/petritagged|Tagged|"+petriPackage+".Marking")
	old := Petripublic.Flags.Lookup("check-stale").Value.String()
	t.Cleanup(func() { _ = Petripublic.Flags.Set("check-stale", old) })
	_ = Petripublic.Flags.Set("check-stale", "false")
	analysistest.Run(t, analysistest.TestData(), Petripublic, "m/pkg/petritagged")
}

func TestPetriPublicTaggedDebtStrict(t *testing.T) {
	useFixtures(t, "petri-public|pkg/petritagged|Tagged|"+petriPackage+".Marking")
	t.Setenv("GOFLAGS", "-tags=integration")
	analysistest.Run(t, analysistest.TestData(), Petripublic, "m/pkg/petritagged")
}

func TestPetriPublicEstablishedDebtCannotGrow(t *testing.T) {
	t.Parallel()
	key := "petri-public|pkg/petridebt|Listed|" + petriPackage + ".Marking"
	for _, extra := range []string{
		"petri-public|pkg/petridebt|Unlisted|" + petriPackage + ".Marking",
		"petri-public|pkg/petridebt|Listed|" + petriPackage + ".Token",
	} {
		if _, err := CompareBaselineGrowth(key, key+"\n"+extra); err == nil {
			t.Fatalf("admitted new exported-object/type pair %s", extra)
		}
	}
}
