package acp_test

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Isolation: isolated-with-reason - pinned wire peer; each failure branch
// requires a fresh golden subprocess and exact session/config RPC diagnostics.
func TestYouRunMapsGoldenSessionAndConfigRPCFailuresToTerminalWork(t *testing.T) {
	for _, test := range []struct {
		mode       string
		diagnostic string
	}{
		{mode: "new-fail", diagnostic: "golden session/new failure"},
		{mode: "config-fail", diagnostic: "golden model config failure"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
			testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP failure"}`))
			writeACPWorker(t, dir, "cursor-acp")
			fixture := goldenACPFixture(test.mode)
			t.Setenv("YOU_ACP_GOLDEN_SENTINEL", "preserved")

			var starts atomic.Int32
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
				PlatformProcessCommandFactory: goldenACPCommandFactory(&starts, fixture),
				ProvidersExecutableLocator:    availableExecutableLocator{},
			}, 20*time.Second)
			if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
				t.Fatalf("failed work = %d, want 1", got)
			}
			if starts.Load() != 1 {
				t.Fatalf("ACP process starts = %d, want 1", starts.Load())
			}
			encoded, err := json.Marshal(events)
			if err != nil {
				t.Fatalf("marshal Factory events: %v", err)
			}
			if !strings.Contains(string(encoded), test.diagnostic) {
				t.Fatalf("Factory events omitted RPC diagnostic %q: %s", test.diagnostic, encoded)
			}
		})
	}
}
