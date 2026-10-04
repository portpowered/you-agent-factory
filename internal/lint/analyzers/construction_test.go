package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// Fixtures override analyzer globals, so these tests must remain serialized.
func TestConstructionOwnerAndQualifiedUses(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Construction,
		"m/pkg/services/b", "m/pkg/wire", "m/pkg/transports/http", "m/pkg/services/workers",
		"m/pkg/consumer", "m/pkg/dot", "m/pkg/external")
}
func TestConstructionExactDebtAndStale(t *testing.T) {
	useFixtures(t, "service-construction|pkg/listed|pkg/services/b.NewThing", "service-construction|pkg/ctorstale|pkg/services/b.NewThing")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/listed", "m/pkg/ctorstale")
}
func TestConstructionDefersTaggedStale(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing")
	old := Construction.Flags.Lookup("check-stale").Value.String()
	t.Cleanup(func() { _ = Construction.Flags.Set("check-stale", old) })
	_ = Construction.Flags.Set("check-stale", "false")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/tagged")
}

func TestConstructionTaggedDebtStrict(t *testing.T) {
	useFixtures(t, "service-construction|pkg/tagged|pkg/services/b.NewThing")
	t.Setenv("GOFLAGS", "-tags=integration")
	analysistest.Run(t, analysistest.TestData(), Construction, "m/pkg/tagged")
}
func TestConstructionNameBoundary(t *testing.T) {
	for _, name := range []string{"New", "NewThing", "EnsureThing", "BuildThing", "CreateThing", "OpenThing", "ProvideThing"} {
		if !serviceConstructorName(name) {
			t.Errorf("missed %s", name)
		}
	}
	for _, name := range []string{"Newthing", "Renew", "InjectThing"} {
		if serviceConstructorName(name) {
			t.Errorf("unexpected %s", name)
		}
	}
}
