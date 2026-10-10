package execution_test

import (
	"encoding/json"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSelectedProviderFailureDiagnosticsExcludePrivateContent(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zap.DebugLevel)
	runner := &selectedCommandRunner{calls: make(chan selectedCommandCall, 3)}
	process := support.BuildProcess(t, serviceedges.Edges{ProcessLogger: zap.New(core).With(zap.String("backend", "selected-diagnostics")), ProviderCommandRunner: runner})
	var runs []selectedInvocation
	for _, flags := range [][]string{{"--json"}, {"--json", "--verbose"}, {"--json", "--debug"}} {
		runs = append(runs, startSelectedOutputInvocation(t, process, t.Context(), support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"), flags, "private-prompt-canary"))
	}
	// All sessions enter before release; verbosity and payload stay local.
	var calls []selectedCommandCall
	for range runs {
		calls = append(calls, selectedAwait(t, runner.calls))
	}
	for _, call := range calls {
		call.reply <- platformprocess.CommandResult{ExitCode: 1, Stdout: []byte("{\"type\":\"turn.failed\",\"error\":{\"message\":\"authentication failed: sk-fi-private-provider-canary\"}}\n"), Stderr: []byte("private-stderr-canary")}
	}
	for _, run := range runs {
		if err := selectedAwait(t, run.done); err == nil {
			t.Fatal("failed provider returned success")
		}
		response := support.DecodeInvocationResponseJSON(t, run.stdout())
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("provider failure status = %#v", response)
		}
		assertSelectedSessionLog(t, logs, run)
	}
	finished := logs.FilterMessage("workers execute finished").All()
	if len(finished) == 0 {
		t.Fatal("missing actual terminal worker diagnostic")
	}
	for _, entry := range logs.All() {
		encoded, err := json.Marshal(entry.ContextMap())
		if err != nil {
			t.Fatal(err)
		}
		for _, private := range []string{"private-prompt-canary", "sk-fi-private-provider-canary", "private-stderr-canary", "turn.failed"} {
			if strings.Contains(entry.Message+string(encoded), private) {
				t.Errorf("private content %s in diagnostic %q", private, entry.Message)
			}
		}
	}
	t.Log("L02: actual provider failure reaches public FAILED and safe terminal diagnostics without prompt/raw-provider canaries")
}
