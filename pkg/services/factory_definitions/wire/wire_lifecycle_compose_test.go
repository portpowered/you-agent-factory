package wire_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

// Wire-constructed roots must exercise lifecycle-host behavior through the
// internal composed root rather than the transitional public definition shim.
func TestNewServiceConstructsLifecycleHostThroughInternalComposition(t *testing.T) {
	t.Parallel()

	ports := validConstructionPorts(t)
	service, err := factorydefinitionswire.NewService(
		ports.sessionHost,
		ports.activationGateway,
		ports.validator,
		ports.persistence,
		ports.loader,
		ports.applySupportedFiles,
		ports.applyStarterWork,
		ports.namedPaths,
		ports.namedFactoryCatalogFileSystem,
		ports.clock,
		ports.versionFileSystem,
		ports.listEffective,
		ports.packagedCatalog,
		ports.packagedInstaller,
		ports.requiredToolChecker,
		ports.orchestratorValidator,
		ports.portableFileSystem,
		ports.directoryReplacementStore,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	var root factorydefinitions.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to factorydefinitions.Service")
	}

	_, getCurrentErr := root.GetCurrentNamedFactory(context.Background())
	if !errors.Is(getCurrentErr, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentNamedFactory() error = %v, want %v",
			getCurrentErr,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
}
