// Package service constructs the Factory Definitions service implementation.
// Concrete persistence and snapshot packages remain private to composition;
// callers depend on the factory_definitions root contract.
package service

import (
	"context"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
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
	namedPaths interface {
		ResolveExistingDir(string, string) (string, error)
	},
) factoryroot.Service {
	if sessionHost == nil || clock == nil || versionFileSystem == nil {
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
	definitions := factorydefinition.New(host, versionFileSystem)
	sessionHost.AttachFactoryDefinitions(definitions)
	return definitions
}
