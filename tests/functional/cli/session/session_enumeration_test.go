package session

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestSessionEnumeration(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))
	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "settings page"}`))

	// System
	// phase 0
	// Step - 0: initialize the system state using a wire injection of the world
	// step - 1: run the system using the wire injected world state
	// Step - 2: assert the world state based on the injected elements

	// phase 1
	// step - 1: initialize the injected worl dstate but with a mocked injected state
	// step -2 : run with the injected mocks
	// step 3: assert world state base don injected mocks

	// repeat for each phase of the system
	// missing reagants:
	// - MCP client
	// - REST/SSE CLIENT
	// - CLI/STDOUT/STDERR test instancess

	// provider := testutil.NewMockWorkerMapProvider(map[string][]interfaces.InferenceResponse{
	// 	"swe":      {{Content: "Done. COMPLETE"}},
	// 	"reviewer": {{Content: "Approved. COMPLETE"}},
	// })

	// h := testutil.NewServiceTestHarness(t, dir,
	// 	testutil.WithProvider(provider),
	// 	testutil.WithFullWorkerPoolAndScriptWrap(),
	// )

	// h.RunUntilComplete(t, 10*time.Second)

	// h.Assert().
	// 	HasTokenInPlace("code-change:complete").
	// 	HasNoTokenInPlace("code-change:init").
	// 	HasNoTokenInPlace("code-change:in-review").
	// 	HasNoTokenInPlace("code-change:failed")

	// if provider.CallCount("swe") != 1 {
	// 	t.Errorf("swe called unexpected number of times: expected 1, got %d", provider.CallCount("swe"))
	// }
	// if provider.CallCount("reviewer") != 1 {
	// 	t.Errorf("reviewer called unexpected number of times: expected 1, got %d", provider.CallCount("reviewer"))
	// }
}
