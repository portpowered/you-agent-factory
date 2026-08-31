package claude

import (
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const claudeFunctionalModel = "claude-sonnet-5"

// TestClaudeStreamJSONCommandThroughRootBuildProcess proves the customer
// process dispatches the complete Claude streaming CLI contract and consumes
// its native terminal result successfully.
func TestClaudeStreamJSONCommandThroughRootBuildProcess(t *testing.T) {
	fixture := claudeSharedProcess(t)
	route := fixture.route(t, "standalone")
	callStart := fixture.runner.CallsFor(route.factoryDir)
	session := fixture.openSession(t, "standalone")
	fixture.submitWork(t, session, "claude stream-json command")
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, session.id, 20*time.Second)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t, sessionWorkURL(fixture.baseURL, session.id))

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if got := fixture.runner.CallsFor(route.factoryDir) - callStart; got != 1 {
		t.Fatalf("Claude command calls = %d, want 1", got)
	}

	request, ok := fixture.runner.RequestFor(route.factoryDir)
	if !ok {
		t.Fatal("shared Claude standalone route recorded no command request")
	}
	if request.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("command = %q, want %q", request.Command, modelprovider.ProviderClaude)
	}
	support.AssertArgsContainSequence(t, request.Args, []string{
		"--model", claudeFunctionalModel,
		"--verbose",
		"--output-format", "stream-json",
		"--include-partial-messages",
	})
	fixture.closeSession(t, session)
}
