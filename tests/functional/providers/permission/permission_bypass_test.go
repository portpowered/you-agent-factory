package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// The immutable incapable route is reused across denial, ordinary success and
// denial again. Each public one-shot invocation owns its session and streams;
// only the external command effect and capability publication are controlled.
func TestDirectProviderCapabilityDenialDoesNotPoisonFreshAttempt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	home := t.TempDir()
	workflow := filepath.Join(dir, "direct.js")
	ordinary := filepath.Join(dir, "ordinary.js")
	const source = `return (async function () { return await agent.run({
  prompt: "direct capability proof", label: "direct-child",
  modelProvider: "codex", model: "test-model", permissions: "%s"
}); })();`
	for path, permission := range map[string]string{workflow: "SKIP_PERMISSIONS", ordinary: "DEFAULT"} {
		if err := os.WriteFile(path, []byte(fmt.Sprintf(source, permission)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("fresh direct attempt COMPLETE"),
	})
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
		ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{
			Provider:     providers.IDCodex,
			Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
		}},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Close(ctx); err != nil {
			t.Errorf("close direct provider process: %v", err)
		}
	})
	sessions := make(map[string]bool)
	for _, denied := range []bool{true, false, true} {
		selected := ordinary
		if denied {
			selected = workflow
		}
		args := []string{"you", "--json", "run", "--factory", selected}
		if denied {
			args = append(args, "--skip-permissions")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		inputs := support.FakeInputs(ctx, args)
		inputs.Input.Env = []string{"HOME=" + home, "USERPROFILE=" + home}
		inputs.Input.WorkingDirectory = dir
		before := runner.CallCount()
		err := process.Execute(inputs.Input)
		cancel()
		var outcome struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
		}
		if decodeErr := json.Unmarshal([]byte(inputs.Stdout()), &outcome); decodeErr != nil || outcome.SessionID == "" || sessions[outcome.SessionID] {
			t.Fatalf("direct outcome = %q, decode=%v; want a fresh session", inputs.Stdout(), decodeErr)
		}
		sessions[outcome.SessionID] = true
		if denied {
			if err != nil || outcome.Status != "FAILED" || runner.CallCount() != before || inputs.Stderr() != "" {
				t.Fatalf("denied outcome=%q error=%v calls=%d stderr=%q", outcome.Status, err, runner.CallCount()-before, inputs.Stderr())
			}
			continue
		}
		if err != nil || outcome.Status != "SUCCEEDED" || !strings.Contains(inputs.Stdout(), "fresh direct attempt COMPLETE") || inputs.Stderr() != "" {
			t.Fatalf("fresh attempt = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
		}
		requests := runner.Requests()
		if len(requests) != before+1 || requests[before].Command != "codex" ||
			slices.Contains(requests[before].Args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("fresh command requests = %#v, want one ordinary Codex attempt", requests)
		}
	}
}

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
