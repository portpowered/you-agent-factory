//go:build functionallong

package current

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCurrentFactoryActivationSwitchesPersistedFactories proves that activating
// a second persisted Factory through the public session API becomes the Current
// Factory and resolves the correct customer-visible name and directory. The
// server fixture composes the runtime through root.BuildProcess and public CLI
// argv/stdio exactly as a customer invocation would.
func TestCurrentFactoryActivationSwitchesPersistedFactories(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory activation persistence smoke")

	rootDir := t.TempDir()
	alphaDir := seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	betaDir := createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	server := startCurrentFactoryServer(t, rootDir)
	defer server.Stop(t)
	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)

	assertCurrentFactoryNameAndDirectory(t, server.URL(), "alpha", alphaDir)

	activateNamedPersistedFactoryOverHTTP(
		t,
		server.URL(),
		functionalNamedFactoryPayloadWithWorkType(t, "beta", "beta-task"),
	)

	assertCurrentFactoryNameAndDirectory(t, server.URL(), "beta", betaDir)
}

// TestCurrentFactoryLiveAPIReadsFollowActivatedFactory proves that live Work
// submit and read outcomes after Current Factory activation follow the activated
// Factory through root.BuildProcess and the public session API, with provider
// execution captured through ProviderCommandRunner.
func TestCurrentFactoryLiveAPIReadsFollowActivatedFactory(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory live API activation smoke")

	rootDir := t.TempDir()
	seedNamedFactoryRootWithTerminalState(t, rootDir, "alpha", "alpha-complete")
	betaDir := createNamedFactoryFixture(
		t,
		rootDir,
		"beta",
		functionalNamedFactoryPayloadWithTerminalState(t, "beta", "beta-complete"),
	)

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.ClaudeSuccessStdout("Done. COMPLETE"),
	}, platformprocess.CommandResult{
		Stdout: support.ClaudeSuccessStdout("Done. COMPLETE"),
	})
	server := startCurrentFactoryServerWithProviderRunner(t, rootDir, runner)
	defer server.Stop(t)

	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)
	activateNamedPersistedFactoryOverHTTP(
		t,
		server.URL(),
		functionalNamedFactoryPayloadWithTerminalState(t, "beta", "beta-complete"),
	)
	assertCurrentFactoryNameAndDirectory(t, server.URL(), "beta", betaDir)
	waitForActivatedFactoryRuntime(t, server.URL(), "task:beta-complete", 5*time.Second)

	workName := "current-factory-live-api-work"
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      json.RawMessage(`{"title":"beta api work"}`),
	})
	if submitted.TraceId == "" {
		t.Fatal("POST /work returned an empty trace ID after activation")
	}

	listed := waitForSubmittedWorkAtPlace(
		t,
		server.URL(),
		submitted.TraceId,
		"task:beta-complete",
		5*time.Second,
	)
	if len(listed.Results) != 1 {
		status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
		t.Fatalf(
			"GET /work result count after activation = %d, want 1; provider_calls=%d factory_state=%q runtime_status=%q total_tokens=%d",
			len(listed.Results),
			runner.CallCount(),
			status.FactoryState,
			status.RuntimeStatus,
			status.TotalTokens,
		)
	}
	if workCustomerPlaceID(listed.Results[0]) != "task:beta-complete" {
		t.Fatalf(
			"GET /work place_id after activation = %q, want task:beta-complete",
			workCustomerPlaceID(listed.Results[0]),
		)
	}

	status := support.GetJSON[factoryapi.StatusResponse](t, server.URL()+"/status")
	if status.RuntimeStatus != string(interfaces.RuntimeStatusIdle) {
		t.Fatalf("GET /status runtime_status after activation = %q, want %q", status.RuntimeStatus, interfaces.RuntimeStatusIdle)
	}
	if status.TotalTokens != 1 {
		t.Fatalf("GET /status total_tokens after activation = %d, want 1", status.TotalTokens)
	}
	if status.Categories.Terminal != 1 {
		t.Fatalf("GET /status terminal count after activation = %d, want 1", status.Categories.Terminal)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
}

// TestCurrentFactoryWatchedFileExecutionFollowsActivatedFactory proves that
// watched-file execution follows the activated Current Factory and does not
// continue dispatching on the previously active Factory after handoff.
func TestCurrentFactoryWatchedFileExecutionFollowsActivatedFactory(t *testing.T) {
	support.SkipLongFunctional(t, "slow current-factory watcher activation smoke")

	rootDir := t.TempDir()
	_ = seedFilewatcherNamedFactoryRoot(t, rootDir, "alpha", true)
	betaDir := seedFilewatcherNamedFactoryRoot(t, rootDir, "beta", false)

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
		platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)
	server := startCurrentFactoryServerWithProviderRunner(t, rootDir, runner)
	defer server.Stop(t)

	support.WaitForRuntimeIdle(t, server.URL(), 5*time.Second)
	activateNamedPersistedFactoryOverHTTP(t, server.URL(), namedFilewatcherFactoryPayload(t, "beta"))
	assertCurrentFactoryNameAndDirectory(t, server.URL(), "beta", betaDir)

	testutil.WriteSeedFile(t, betaDir, "task", []byte(`{"title":"beta watched work"}`))
	waitForProviderCommandWorkSettlement(t, server.URL(), runner, 1, 10*time.Second)

	alphaDir := filepath.Join(rootDir, "alpha")
	testutil.WriteSeedFile(t, alphaDir, "task", []byte(`{"title":"alpha watched work"}`))
	assertNoAdditionalProviderCommandWork(t, server.URL(), runner, 10*time.Second)
}
