package permission

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProviderPermissionBypassFunctionalContract(t *testing.T) {
	t.Run("capable Codex route uses the command edge", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", permissionBypassWorkerConfig("codex"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"permission bypass contract"}`))

		runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
			Stdout: []byte("permission bypass completed\nCOMPLETE"),
		})
		_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
			t,
			dir,
			serviceedges.Edges{ProviderCommandRunner: runner},
			20*time.Second,
		)

		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
			t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
		}
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
			t.Fatalf("failed work = %d, want 0", got)
		}

		requests := runner.Requests()
		if len(requests) != 1 {
			t.Fatalf("provider command calls = %d, want one Codex execution", len(requests))
		}
		request := requests[0]
		if request.Command != string(modelprovider.ProviderCodex) {
			t.Fatalf("provider command = %q, want %q", request.Command, modelprovider.ProviderCodex)
		}
		if !slices.Contains(request.Args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("provider args = %#v, want Codex permission-bypass flag", request.Args)
		}
	})

	// Every currently published bundled provider advertises permission bypass;
	// the registered incapable-manifest path is covered at the provider root
	// and neutral registry boundaries. This root-built unavailable selection
	// keeps the functional assertion on the sanctioned command edge without
	// inventing an in-process provider implementation.
	t.Run("unavailable route fails before the command edge", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", permissionBypassWorkerConfig("cursor"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"permission bypass unavailable route"}`))

		runner := support.NewShapedProviderCommandRunner()
		process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "run", "--dir", dir, "--continuously", "--quiet", "--no-record",
		})
		inputs.Input.WorkingDirectory = dir
		executeErr := process.Execute(inputs.Input)
		if executeErr == nil ||
			(!strings.Contains(executeErr.Error(), `provider "cursor" is unknown`) &&
				!strings.Contains(executeErr.Error(), "validate Factory provider selections")) {
			t.Fatalf("Process.Execute(unavailable bypass route) error = %v, want provider-selection failure", executeErr)
		}
		if requests := runner.Requests(); len(requests) != 0 {
			t.Fatalf("provider command calls = %d, want zero for unavailable route", len(requests))
		}
	})
}

func permissionBypassWorkerConfig(provider string) string {
	return "---\n" +
		"type: MODEL_WORKER\n" +
		"model: test-model\n" +
		"modelProvider: " + provider + "\n" +
		"skipPermissions: true\n" +
		"stopToken: COMPLETE\n" +
		"---\n" +
		"Process the input task.\n"
}
