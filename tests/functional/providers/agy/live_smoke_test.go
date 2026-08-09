package agy

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agyLiveSmokeEnv   = "YOU_AGY_LIVE_SMOKE"
	agyLiveSmokeModel = "gemini-3.6-flash-low"
)

// TestAgyLiveSmoke is the one operator-gated live AGY invocation. It stays
// outside ordinary CI because AGY credentials and quota are external effects;
// the default path is the offline golden suite in this package. Run it with:
// $env:YOU_AGY_LIVE_SMOKE='1'; go test ./tests/functional/providers/agy/... -run '^TestAgyLiveSmoke$' -count=1
func TestAgyLiveSmoke(t *testing.T) {
	if strings.TrimSpace(os.Getenv(agyLiveSmokeEnv)) != "1" {
		t.Skip("AGY live smoke disabled; set $env:YOU_AGY_LIVE_SMOKE='1' to run it")
	}

	agyPath, err := exec.LookPath("agy")
	if err != nil {
		t.Skipf("AGY live smoke skipped: agy is not on PATH (%v); install AGY 1.1.11 or add it to PATH", err)
	}
	t.Logf("running one live AGY smoke through root.BuildProcess with %s", agyPath)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", agyLiveWorkerConfig())
	support.WriteWorkstationConfig(t, dir, "process", agyLiveWorkstationConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy live smoke"}`))

	_, listed, events, _ := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		// An empty edge set deliberately selects the production subprocess runner.
		// The gate above is the only protection against an external AGY call.
		serviceedges.Edges{},
		// AGY's native print timeout is five minutes; the extra observation budget
		// lets a slow operator call reach a terminal Work without a sleep loop.
		10*time.Minute,
	)

	done := support.CountWorkAtCustomerState(listed, "task:done")
	failed := support.CountWorkAtCustomerState(listed, "task:failed")
	if done != 1 || failed != 0 {
		t.Fatalf(
			"AGY live smoke did not complete Work (done=%d, failed=%d, events=%d); verify AGY authentication, model availability, and quota",
			done,
			failed,
			len(events),
		)
	}

	response := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if response.Response == nil || !strings.Contains(*response.Response, "TRACE_OK") {
		t.Fatalf(
			"AGY live smoke response = %#v, want a response containing TRACE_OK; verify AGY authentication, model availability, and quota",
			response.Response,
		)
	}
}

func agyLiveWorkerConfig() string {
	return "---\n" +
		"type: MODEL_WORKER\n" +
		"model: " + agyLiveSmokeModel + "\n" +
		"modelProvider: " + string(modelprovider.ProviderAntigravity) + "\n" +
		"skipPermissions: true\n" +
		"---\n" +
		"Return exactly TRACE_OK and nothing else.\n"
}

func agyLiveWorkstationConfig() string {
	return "---\n" +
		"type: MODEL_WORKSTATION\n" +
		"---\n" +
		"Reply with exactly TRACE_OK and nothing else.\n"
}
