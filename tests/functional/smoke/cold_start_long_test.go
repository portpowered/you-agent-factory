//go:build functionallong

package smoke

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestColdStart_SingleTokenReachesTerminal(t *testing.T) {
	support.SkipLongFunctional(t, "slow cold-start single-token workflow smoke")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"task": "fix-bug"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
		"swe":      {{Content: "Done. COMPLETE"}},
		"reviewer": {{Content: "Done. COMPLETE"}},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("code-change:complete").
		HasNoTokenInPlace("code-change:init").
		TokenCount(1)

	if provider.CallCount("swe") != 1 {
		t.Errorf("expected swe called once, got %d", provider.CallCount("swe"))
	}
	if provider.CallCount("reviewer") != 1 {
		t.Errorf("expected reviewer called once, got %d", provider.CallCount("reviewer"))
	}
}
