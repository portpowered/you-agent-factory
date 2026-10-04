package analyzers

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestTransportBehaviorOwnership(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Behavior,
		"m/pkg/transports/behaviorownership", "m/pkg/transports/behaviordot",
		"m/pkg/transports/behaviorpolicyexternal",
		"m/pkg/transports/mapping/behaviorpolicy", "m/pkg/transports/http/behaviorpolicy",
		"m/pkg/transports/mapping/workcontent/behaviorfixture",
		"m/pkg/transports/mcp/factorysession/behaviorfixture")
}

func TestTransportAllowedProtocolRoles(t *testing.T) {
	useFixtures(t)
	analysistest.Run(t, analysistest.TestData(), Behavior, "m/pkg/transports/behaviorallowed",
		"m/pkg/services/b/transports/http", "m/pkg/services/work/transports/behaviorfixture")
}

func TestTransportDebtAndStale(t *testing.T) {
	useFixtures(t,
		"transport-lifecycle|pkg/transports/behaviordebt|context.WithCancel",
		"transport-lifecycle|pkg/transports/behaviorstale|context.WithCancel")
	analysistest.Run(t, analysistest.TestData(), Behavior,
		"m/pkg/transports/behaviordebt", "m/pkg/transports/behaviorstale")
}

func TestTransportDefersTaggedStale(t *testing.T) {
	useFixtures(t, "transport-lifecycle|pkg/transports/behaviortagged|context.WithCancel")
	old := Behavior.Flags.Lookup("check-stale").Value.String()
	t.Cleanup(func() { _ = Behavior.Flags.Set("check-stale", old) })
	_ = Behavior.Flags.Set("check-stale", "false")
	analysistest.Run(t, analysistest.TestData(), Behavior, "m/pkg/transports/behaviortagged")
}

func TestTransportTaggedDebtStrict(t *testing.T) {
	useFixtures(t, "transport-lifecycle|pkg/transports/behaviortagged|context.WithCancel")
	t.Setenv("GOFLAGS", "-tags=integration")
	analysistest.Run(t, analysistest.TestData(), Behavior, "m/pkg/transports/behaviortagged")
}
