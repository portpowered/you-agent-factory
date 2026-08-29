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
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProviderPermissionBypassFunctionalContract(t *testing.T) {
	// Keep the cases on separate root-built processes: capability overrides are
	// immutable construction-time provider wiring. Sharing one process would
	// require mutable capability state or post-start routing.
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

	// The capability override targets the real published Codex route while
	// leaving its built-in adapter and the command edge intact. It models a
	// route-specific authoritative capability view without registering an
	// in-process provider fake or selecting an unknown provider.
	t.Run("registered incapable Codex route fails before the command edge", func(t *testing.T) {
		dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
		support.WriteAgentConfig(t, dir, "worker", permissionBypassWorkerConfig("codex"))
		testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"permission bypass incapable route"}`))

		runner := support.NewShapedProviderCommandRunner()
		_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
			ProviderCommandRunner: runner,
			ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{
				Provider:     providers.IDCodex,
				Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
			}},
		}, 20*time.Second)
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
			t.Fatalf("failed work = %d, want one capability failure; listed=%#v", got, listed)
		}
		observations := support.ObserveDispatchEvents(t, events)
		if len(observations) != 1 || observations[0].Response == nil {
			t.Fatalf("dispatch observations = %#v, want one terminal response", observations)
		}
		response := observations[0].Response
		if response.FailureDetail == nil || !strings.Contains(response.FailureDetail.Message, `provider "codex" does not support capability "permission_bypass"`) {
			t.Fatalf("capability failure detail = %#v, want bounded provider capability diagnostic", response.FailureDetail)
		}
		if response.Error != nil && strings.Contains(*response.Error, "command") {
			t.Fatalf("capability failure error = %q, want no command detail", *response.Error)
		}
		if response.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
			t.Fatalf("capability failure reason = %q, want permanent bad request", response.FailureDetail.Reason)
		}
		if requests := runner.Requests(); len(requests) != 0 {
			t.Fatalf("provider command calls = %d, want zero for incapable route", len(requests))
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
