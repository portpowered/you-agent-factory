package runtime_metrics_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic proves
// the customer CLI process keeps the metrics-owned code and safe message at
// the production central-diagnostics boundary.
func TestMetricsInvalidGroupThroughRootProcessPreservesCodedDiagnostic(t *testing.T) {
	t.Parallel()

	process := runtimeMetricsCLIProcess
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "metrics", "--group-by", "region",
	})
	inputs.Input.Env = []string{"HOME=" + t.TempDir(), "USERPROFILE=" + t.TempDir()}

	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(metrics invalid group) error = nil, want coded failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("metrics stdout = %q, want empty", inputs.Stdout())
	}
	assertMetricsDiagnostic(t, inputs.Stderr(), "METRICS_INVALID_GROUP_BY", `invalid --group-by "region": choose workstation, worker, or provider`)
}

// TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability proves
// both public presenters consume the query result returned by the canonical
// process rather than relying on a presenter-local cost constant.
func TestMetricsSuccessThroughRootProcessRendersQueryCostAvailability(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	factoryDirectory := support.ScaffoldSingleStepFactory(t, "metrics-cost-availability")
	workingDirectory := t.TempDir()
	environment := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDirectory,
		WorkingDirectory:          workingDirectory,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
	})

	process := runtimeMetricsCLIProcess

	human := support.FakeInputs(t.Context(), []string{"you", "--server", server.URL(), "metrics"})
	human.Input.Env = environment
	human.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(human.Input); err != nil {
		t.Fatalf("Process.Execute(metrics human) error = %v\nstdout:\n%s\nstderr:\n%s", err, human.Stdout(), human.Stderr())
	}
	if !strings.Contains(human.Stdout(), "Cost: unavailable\n") || human.Stderr() != "" {
		t.Fatalf("human metrics output = %q, stderr = %q", human.Stdout(), human.Stderr())
	}

	machine := support.FakeInputs(t.Context(), []string{"you", "--json", "--server", server.URL(), "metrics"})
	machine.Input.Env = environment
	machine.Input.WorkingDirectory = workingDirectory
	if err := process.Execute(machine.Input); err != nil {
		t.Fatalf("Process.Execute(metrics JSON) error = %v\nstdout:\n%s\nstderr:\n%s", err, machine.Stdout(), machine.Stderr())
	}
	var document struct {
		Cost struct {
			Availability string `json:"availability"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(machine.Stdout()), &document); err != nil {
		t.Fatalf("decode metrics JSON: %v\n%s", err, machine.Stdout())
	}
	if document.Cost.Availability != "unavailable" || machine.Stderr() != "" {
		t.Fatalf("JSON metrics cost = %#v, stderr = %q", document.Cost, machine.Stderr())
	}
}

func assertMetricsDiagnostic(t *testing.T, output, wantCode, wantMessage string) {
	t.Helper()
	trimmed := strings.TrimSpace(output)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		t.Fatalf("metrics diagnostic = %q, want one JSON line", output)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		t.Fatalf("decode metrics diagnostic: %v\n%s", err, output)
	}
	if response.Code != factoryapi.ErrorResponseCode(wantCode) || response.Message != wantMessage {
		t.Fatalf("metrics diagnostic = %#v, want code %q and message %q", response, wantCode, wantMessage)
	}
}
