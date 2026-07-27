package registry

import (
	"context"
	"encoding/json"
	"fmt"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/gemini"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/kiro"
)

const nativeRuntimePrerequisite = "provider-native-runtime"

// BuiltInDependencies optionally supplies shared execution collaborators for
// migrated catalog Integrations. Process composition injects the same
// ProviderCommandRunner edge used by native executors so conductor-routed
// built-ins and functional overrides share one command boundary.
type BuiltInDependencies struct {
	CommandRunner   workerprocess.CommandRunner
	OperatingSystem string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// BuiltInRegistrations returns detached registrations for every selectable
// bundled manifest. Unmigrated built-ins keep the native-runtime compatibility
// stub; migrated providers bind their package-owned Integration so the neutral
// conductor can invoke them without a concrete-provider switch in shared
// orchestration.
func BuiltInRegistrations(deps ...BuiltInDependencies) ([]Registration, error) {
	var dependencies BuiltInDependencies
	if len(deps) > 0 {
		dependencies = deps[0]
	}
	var catalog catalogDocument
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		return nil, fmt.Errorf("parse embedded provider catalog for built-in registrations: %w", err)
	}
	registrations := make([]Registration, 0, len(catalog.Providers))
	for _, manifest := range catalog.Providers {
		if !requiresBundledImplementation(manifestCandidate{manifest: manifest, bundled: true}) {
			continue
		}
		identity := inference.Identity(normalize(manifest.ID))
		integration := migratedBuiltInIntegration(identity, dependencies)
		if integration == nil {
			integration = nativeRuntimeIntegration{
				identity: identity,
				maximum:  manifestCapabilities(manifest),
			}
		}
		registrations = append(registrations, CatalogRegistration(identity, integration))
	}
	return registrations, nil
}

// ReplaceCatalogIntegration rebinds one catalog identity to a replacement
// Integration while preserving every other registration. Migrated provider
// tests use this to inject execution collaborators without inventing a second
// registry.
func ReplaceCatalogIntegration(
	registrations []Registration,
	identity inference.Identity,
	integration inference.Integration,
) ([]Registration, error) {
	canonical := normalize(string(identity))
	if canonical == "" {
		return nil, fmt.Errorf("catalog identity is required")
	}
	if isNilIntegration(integration) {
		return nil, fmt.Errorf("catalog integration is required")
	}
	if normalize(string(integration.Identity())) != canonical {
		return nil, fmt.Errorf(
			"integration identity %q differs from catalog identity %q",
			integration.Identity(),
			canonical,
		)
	}
	replaced := make([]Registration, 0, len(registrations))
	found := false
	for _, registration := range registrations {
		registrationIdentity := registrationIdentity(registration)
		if registration.kind == catalogRegistration && registrationIdentity == canonical {
			replaced = append(replaced, CatalogRegistration(identity, integration))
			found = true
			continue
		}
		replaced = append(replaced, registration)
	}
	if !found {
		return nil, fmt.Errorf("catalog identity %q is not present in registrations", canonical)
	}
	return replaced, nil
}

func migratedBuiltInIntegration(
	identity inference.Identity,
	dependencies BuiltInDependencies,
) inference.Integration {
	switch normalize(string(identity)) {
	case "cursor":
		return cursor.NewIntegration(cursor.IntegrationDependencies{
			CommandRunner:   dependencies.CommandRunner,
			OperatingSystem: dependencies.OperatingSystem,
			TemporaryFiles:  dependencies.TemporaryFiles,
		})
	case "gemini":
		if dependencies.CommandRunner == nil {
			return gemini.NewIntegration()
		}
		return gemini.NewIntegration(gemini.IntegrationDependencies{
			CommandRunner: dependencies.CommandRunner,
		})
	case "kiro":
		if dependencies.CommandRunner == nil {
			return kiro.NewIntegration()
		}
		return kiro.NewIntegration(kiro.IntegrationDependencies{
			CommandRunner: dependencies.CommandRunner,
		})
	default:
		return nil
	}
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

// UsesProviderNativeRuntime marks the compatibility stub so registry routing
// keeps unmigrated bundled providers on the retained native runner path.
func (nativeRuntimeIntegration) UsesProviderNativeRuntime() bool { return true }
