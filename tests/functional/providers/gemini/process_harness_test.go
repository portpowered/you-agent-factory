package gemini

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootBuiltProcessExecutesThroughSharedSupport(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Run and manage") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want root command help", inputs.Stdout())
	}
}

func TestGeminiConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini functional answer COMPLETE"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini (conductor-selected built-in)", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "gemini-2.5-flash") {
		t.Fatalf("args = %#v, want --model gemini-2.5-flash", request.Args)
	}
}

func TestGeminiNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini native failure"}`))

	// Use a non-retryable auth failure so the factory reaches a single terminal
	// failure without provider/workstation retry amplification.
	const leaked = "/tmp/secret-key"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED","message":"token path ` + leaked + ` leaked"}}`),
	})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 after native failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.CallCount())
	}
	if request := runner.LastRequest(); request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", request.Command)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) ||
		strings.Contains(payload, "secret-key") ||
		strings.Contains(payload, "/tmp/") {
		t.Fatalf("factory events leaked unsafe Gemini failure detail: %s", payload)
	}
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
