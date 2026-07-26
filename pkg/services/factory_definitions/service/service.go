// Package service constructs the Factory Definitions service implementation.
// Concrete persistence and snapshot packages remain private to composition;
// callers depend on the factory_definitions root contract.
package service

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
	catalog "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/wire"
)

// New constructs the public Factory Definitions service.
func New(
	sessionHost factoryroot.SessionHost,
	clock factoryroot.Clock,
	versionFileSystem factoryroot.VersionFileSystem,
	validator factorydefinitions.Validator,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	readCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerReader,
	prepareFactoryLayoutPayload factorydefinitions.FactoryLayoutPayloadPreparer,
	persistNamedFactory factorydefinitions.NamedFactoryPersister,
	writeCurrentFactoryPointer factorydefinitions.CurrentFactoryPointerWriter,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
	replaceFactoryLayout factorydefinitions.FactoryLayoutReplacer,
	namedPaths factoryroot.NamedPathResolver,
	namedFactoryCatalogFileSystem factoryroot.NamedFactoryCatalogFileSystem,
) factoryroot.Service {
	if sessionHost == nil || clock == nil || versionFileSystem == nil ||
		namedPaths == nil || namedFactoryCatalogFileSystem == nil {
		return nil
	}
	host, err := factorydefinition.NewHost(
		sessionHost.PersistRootDir, sessionHost.WorkstationLoader,
		loadFactory,
		readCurrentFactoryPointer,
		func(
			segment string,
			payload []byte,
		) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return prepareFactoryLayoutPayload(
				context.Background(),
				segment,
				payload,
				validator,
			)
		},
		persistNamedFactory,
		writeCurrentFactoryPointer,
		preparePortableFactoryConfig,
		captureFactorySnapshot,
		sessionHost.CurrentRuntimeConfig, sessionHost.WorkflowID,
		namedPaths.ResolveExistingDir,
		sessionHost.RequireSession, sessionHost.SessionRuntimeConfig,
		sessionHost.SessionFactoryPersistRoot, sessionHost.ValidateEditableFactorySnapshot,
		sessionHost.GetCurrentFactorySnapshotForSession, sessionHost.WithActivationLock,
		sessionHost.RequireIdleRuntimeForSession, sessionHost.ActivateSessionEditableFactory,
		replaceFactoryLayout,
		clock.Now,
		sessionHost.RunSessionID, sessionHost.SessionForActivation,
		sessionHost.NamedFactoryActivationPaths, sessionHost.RequireIdleBeforeNamedFactoryActivation,
		sessionHost.SwapPersistedNamedFactoryRuntime,
	)
	if err != nil {
		return nil
	}
	// The exact ports were rejected above, which exhausts the catalog
	// constructor's failure cases.
	catalogService, _ := catalogwire.NewService(catalog.Dependencies{
		Paths:      namedPaths,
		FileSystem: namedFactoryCatalogFileSystem,
	})
	definitions := factorydefinition.NewWithCatalog(host, catalogService, versionFileSystem)
	sessionHost.AttachFactoryDefinitions(definitions)
	return definitions
}
