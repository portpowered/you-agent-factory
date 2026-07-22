//go:build functionallong

package smoke

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCustomerProcess_HappyPath(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness happy-path sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "service harness happy path"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Step one done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Step two done. COMPLETE"},
	)
	runCustomerFactoryProcess(t, dir, provider)

	if provider.CallCount() != 2 {
		t.Errorf("expected provider called 2 times, got %d", provider.CallCount())
	}

	calls := provider.Calls()
	if calls[0].Model != "test-model" {
		t.Errorf("expected model test-model for call 0, got %q", calls[0].Model)
	}
	if calls[0].SystemPrompt == "" {
		t.Error("expected non-empty system prompt for call 0")
	}
}

func TestCustomerProcess_NoopFallback(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness noop sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "noop_pipeline"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "noop fallback test"}`))

	runCustomerFactoryProcess(t, dir, nil)
}

func TestCustomerProcess_MultipleWorkItems(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness multi-item sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))

	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-1"}`))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title": "queued-2"}`))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
		workerexecution.InferenceResponse{Content: "Done. COMPLETE"},
	)
	runCustomerFactoryProcess(t, dir, provider)
	if provider.CallCount() != 4 {
		t.Fatalf("provider call count = %d, want 4", provider.CallCount())
	}
}

func runCustomerFactoryProcess(
	t *testing.T,
	dir string,
	provider providercontract.Provider,
) {
	t.Helper()
	support.SetWorkingDirectory(t, dir)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", dir, "--no-record",
	})
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute() error = %v; stdout=%q stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
}
