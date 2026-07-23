package registry

import (
	"context"
	"encoding/json"
	"fmt"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

const nativeRuntimePrerequisite = "provider-native-runtime"

// BuiltInRegistrations returns detached registrations for every selectable
// bundled manifest. The compatibility integrations describe the accepted
// built-ins without moving provider-native execution into the neutral
// Integration conductor.
func BuiltInRegistrations() ([]Registration, error) {
	var catalog catalogDocument
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		return nil, fmt.Errorf("parse embedded provider catalog for built-in registrations: %w", err)
	}
	registrations := make([]Registration, 0, len(catalog.Providers))
	for _, manifest := range catalog.Providers {
		if !requiresBundledImplementation(manifestCandidate{manifest: manifest, bundled: true}) {
			continue
		}
		integration := nativeRuntimeIntegration{
			identity: inference.Identity(normalize(manifest.ID)),
			maximum:  manifestCapabilities(manifest),
		}
		registrations = append(registrations, CatalogRegistration(integration.identity, integration))
	}
	return registrations, nil
}

// nativeRuntimeIntegration preserves the accepted Integration shape while the
// built-in invocation conductor remains on the legacy Provider runtime.
type nativeRuntimeIntegration struct {
	identity inference.Identity
	maximum  inference.CapabilitySet
}

func (i nativeRuntimeIntegration) Identity() inference.Identity {
	return i.identity
}

func (i nativeRuntimeIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(i.maximum.Values()...)
}

func (i nativeRuntimeIntegration) Discover(context.Context) (inference.Discovery, error) {
	return inference.NewDiscovery(
		inference.ReadinessUnavailable,
		inference.NewPrerequisite(
			inference.PrerequisiteDependency,
			nativeRuntimePrerequisite,
			inference.PrerequisiteMissing,
			"Use the provider-native runtime until neutral invocation routing is enabled.",
		),
	), nil
}

func (i nativeRuntimeIntegration) Capabilities(
	context.Context,
	inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}

func (i nativeRuntimeIntegration) Invoke(
	ctx context.Context,
	_ inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	failure := inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureDependency,
		Message: "Neutral invocation routing is not enabled for this built-in provider.",
	})
	return writer.Close(ctx, inference.FailedCompletion(failure))
}
