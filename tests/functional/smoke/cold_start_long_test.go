//go:build functionallong

package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestColdStart_SingleTokenReachesTerminal(t *testing.T) {
	support.SkipLongFunctional(t, "slow cold-start single-token workflow smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "fix-bug"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}},
	})

	runFactoryThroughCustomerProcess(t, dir, provider)

	if provider.CallCount("swe") != 1 {
		t.Errorf("expected swe called once, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called once, got %d", provider.CallCount("reviewer"))
	}
}
