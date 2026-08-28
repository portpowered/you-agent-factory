package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction proves
// root.BuildProcess stays inert while its public provider identity capability is
// available for composition.
func TestAgentRunnerDeliveryRemainsInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	if process == nil || process.ProviderRegistry() == nil {
		t.Fatal("root-built process or provider registry = nil, want inert composition")
	}
	for _, providerID := range []string{"claude", "codex"} {
		if got, err := process.ProviderRegistry().CanonicalIdentity(providerID); err != nil || got != providerID {
			t.Fatalf("CanonicalIdentity(%q) = (%q, %v), want (%q, nil)", providerID, got, err, providerID)
		}
	}
	if _, err := process.ProviderRegistry().CanonicalIdentity("missing.provider"); err == nil {
		t.Fatal("CanonicalIdentity(missing.provider) error = nil, want unknown-provider failure")
	}
}

// TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService proves
// the customer process reaches the shared Workers execution path through the
// public Process.Execute boundary. The provider command runner edge replaces
// only the external inference effect; terminal Work state remains observable
// through the public API projection.
func TestBuildProcessExecutesModelWorkerThroughConvergedWorkersService(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"converged-agent-model",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte("converged agent payload"))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("converged agent response COMPLETE"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one", runner.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed customer Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed customer Work = %d, want zero; listed=%#v", got, listed)
	}
	assertAcceptedAgentDispatch(t, server.GetFactoryEvents(t), "converged agent response COMPLETE")
}

// TestBuildProcessResolvesRegisteredAgentThroughProviders proves the normal
// production composition selects the registered Agent runner and reaches the
// Providers command edge through the public Process.Execute path. This keeps
// the test distinct from the provider-override seam above: registry selection
// and provider resolution are part of the behavior under test.
func TestBuildProcessResolvesRegisteredAgentThroughProviders(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"converged-agent-model",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte("registered agent payload"))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("registered agent response COMPLETE"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one", runner.CallCount())
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed customer Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed customer Work = %d, want zero; listed=%#v", got, listed)
	}
	assertAcceptedAgentDispatch(t, server.GetFactoryEvents(t), "registered agent response COMPLETE")
}

func assertAcceptedAgentDispatch(t *testing.T, events []factoryapi.FactoryEvent, wantOutput string) {
	t.Helper()

	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
			continue
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("agent dispatch outcome = %s, want ACCEPTED", dispatch.Response.Outcome)
		}
		if got := support.StringPointerValue(dispatch.Response.Output); !strings.Contains(got, wantOutput) {
			t.Fatalf("agent dispatch output = %q, want content %q", got, wantOutput)
		}
		return
	}
	t.Fatalf("accepted agent dispatch for process with output %q not found", wantOutput)
}
