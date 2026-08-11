package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestProviderPermissionBypassFunctionalContract(t *testing.T) {
	t.Parallel()

	capable := newPermissionBypassIntegration("capable-route")
	capableRoot := newPermissionBypassRoot(t, capable, true)
	result, err := capableRoot.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.ID(capable.identity),
		AttemptID:       "capable-bypass",
		SkipPermissions: true,
	})
	if err != nil || result.Content != "capable-route" || capable.invocations != 1 || !capable.lastSkipPermissions {
		t.Fatalf(
			"capable Execute() = (%#v, %v, invocations=%d, skip=%v), want one bypass-enabled execution edge",
			result,
			err,
			capable.invocations,
			capable.lastSkipPermissions,
		)
	}

	incapable := newPermissionBypassIntegration("incapable-route")
	incapableRoot := newPermissionBypassRoot(t, incapable, false)
	_, err = incapableRoot.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.ID(incapable.identity),
		AttemptID:       "incapable-bypass",
		SkipPermissions: true,
	})
	var failure providers.ExecuteFailure
	if !errors.Is(err, providers.ErrCapabilityMismatch) ||
		!errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindCapabilityMismatch ||
		!strings.Contains(failure.Message, "incapable-route") ||
		!strings.Contains(failure.Message, "permission_bypass") ||
		incapable.invocations != 0 {
		t.Fatalf(
			"incapable Execute() = (%v, failure=%#v, invocations=%d), want safe pre-edge capability failure",
			err,
			failure,
			incapable.invocations,
		)
	}

	for _, request := range []providers.ExecuteRequest{
		{Provider: providers.ID(incapable.identity), AttemptID: "omitted-bypass"},
		{Provider: providers.ID(incapable.identity), AttemptID: "false-bypass", SkipPermissions: false},
	} {
		result, err := incapableRoot.Execute(context.Background(), request)
		if err != nil || result.Content != "incapable-route" {
			t.Fatalf("default Execute(%q) = (%#v, %v), want existing provider default", request.AttemptID, result, err)
		}
	}
	if incapable.invocations != 2 || incapable.lastSkipPermissions {
		t.Fatalf("default execution edge state = invocations=%d skip=%v, want two false requests", incapable.invocations, incapable.lastSkipPermissions)
	}
}

func newPermissionBypassRoot(
	t *testing.T,
	integration *permissionBypassIntegration,
	bypass bool,
) providers.Service {
	t.Helper()
	root, err := providerswire.NewService(providerswire.WithRegistrations(providerswire.Registration{
		Manifest: providerswire.Manifest{
			ID:          string(integration.identity),
			DisplayName: providerswire.LocalizedValue{Value: string(integration.identity)},
			MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{
				PromptSubmission: true,
				PermissionBypass: bypass,
			},
		},
		Integration: integration,
	}))
	if err != nil {
		t.Fatalf("NewService(%q) = %v", integration.identity, err)
	}
	return root
}

type permissionBypassIntegration struct {
	identity            providerswire.Identity
	invocations         int
	lastSkipPermissions bool
}

func newPermissionBypassIntegration(identity providerswire.Identity) *permissionBypassIntegration {
	return &permissionBypassIntegration{identity: identity}
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
	integration.invocations++
	integration.lastSkipPermissions = request.SkipPermissions
	return writer.Close(ctx, providerswire.SuccessfulCompletion(providerswire.Response{
		Content: string(integration.identity),
	}))
}
