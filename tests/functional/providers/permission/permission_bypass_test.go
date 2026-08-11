package permission

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProviderPermissionBypassFunctionalContract(t *testing.T) {
	t.Run("capable route executes with bypass", func(t *testing.T) {
		const provider = "capable-route"
		integration := newPermissionBypassIntegration(provider)
		listed := runPermissionBypassFactory(t, provider, true, integration)

		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
			t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
		}
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
			t.Fatalf("failed work = %d, want 0", got)
		}
		invocations, skipPermissions := integration.Stats()
		if invocations != 1 || !skipPermissions {
			t.Fatalf("capable integration stats = (invocations=%d, skip=%v), want one bypass-enabled invocation", invocations, skipPermissions)
		}
	})

	t.Run("incapable route fails before integration", func(t *testing.T) {
		const provider = "incapable-route"
		integration := newPermissionBypassIntegration(provider)
		listed := runPermissionBypassFactory(t, provider, false, integration)

		if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
			t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
		}
		if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
			t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
		}
		invocations, skipPermissions := integration.Stats()
		if invocations != 0 || skipPermissions {
			t.Fatalf("incapable integration stats = (invocations=%d, skip=%v), want no execution edge invocation", invocations, skipPermissions)
		}
	})
}

func runPermissionBypassFactory(
	t *testing.T,
	provider string,
	bypass bool,
	integration *permissionBypassIntegration,
) factoryapi.ListWorkResponse {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", permissionBypassWorkerConfig(provider))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"permission bypass contract"}`))

	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{
			ProviderRegistrations: []providerswire.Registration{{
				Manifest:    permissionBypassManifest(t, provider, bypass),
				Integration: integration,
			}},
		},
		20*time.Second,
	)
	return listed
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

func permissionBypassManifest(t *testing.T, identity string, bypass bool) providerswire.Manifest {
	t.Helper()
	var catalog struct {
		Providers []providerswire.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{identity + "-alias"}
	manifest.ImplementationAvailability = providerswire.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = providerswire.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities.PermissionBypass = bypass
	return manifest
}

type permissionBypassIntegration struct {
	identity providerswire.Identity
	mu       sync.Mutex
	invokes  int
	skip     bool
}

func newPermissionBypassIntegration(identity string) *permissionBypassIntegration {
	return &permissionBypassIntegration{identity: providerswire.Identity(identity)}
}

func (integration *permissionBypassIntegration) Identity() providerswire.Identity {
	return integration.identity
}

func (*permissionBypassIntegration) MaximumCapabilities() providerswire.CapabilitySet {
	return providerswire.NewCapabilitySet(providerswire.CapabilityPromptSubmission)
}

func (*permissionBypassIntegration) Discover(context.Context) (providerswire.Discovery, error) {
	return providerswire.Discovery{}, nil
}

func (integration *permissionBypassIntegration) Capabilities(
	context.Context,
	providerswire.InvocationRequest,
) (providerswire.CapabilitySet, error) {
	return integration.MaximumCapabilities(), nil
}

func (integration *permissionBypassIntegration) Invoke(
	ctx context.Context,
	request providerswire.InvocationRequest,
	writer providerswire.ResponseWriter,
) error {
	integration.mu.Lock()
	integration.invokes++
	integration.skip = request.SkipPermissions
	integration.mu.Unlock()
	return writer.Close(ctx, providerswire.SuccessfulCompletion(providerswire.Response{
		Content: "permission bypass completed\nCOMPLETE",
	}))
}

func (integration *permissionBypassIntegration) Stats() (invocations int, skipPermissions bool) {
	integration.mu.Lock()
	defer integration.mu.Unlock()
	return integration.invokes, integration.skip
}
