// Package wire is the Factory Definitions service composition boundary.
//
// Wire performs construction only, returns the singular factorydefinitions.Service
// root interface, and starts no lifecycle components. Parent-private catalog Wire
// and the accepted service assembly stay inside the owner boundary; peers depend on
// Service rather than Definition owner internals or construction ports.
package wire

import (
	"context"
	"fmt"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/lifecycle"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
)

// LifecycleHost is the completed session/persistence adapter consumed by Definitions.
type LifecycleHost = lifecycle.Host

// NewService constructs only the inert root from its completed owners.
func NewService(
	host LifecycleHost,
	activationGateway factorydefinitions.DefinitionActivationGateway,
	catalogService Catalog,
	validationService Validation,
	authoringLayout AuthoringLayout,
	distribution Distribution,
	runtimeSnapshot RuntimeSnapshot,
	compilation Compilation,
	versionFileSystem factorydefinitions.VersionFileSystem,
	listEffective factorydefinitions.EffectiveFactoryCatalogOperation,
	snapshotsPortability SnapshotsPortability,
) (factorydefinitions.Service, error) {
	if host == nil {
		return nil, fmt.Errorf("construct Factory Definitions: lifecycle host is required")
	}
	if activationGateway == nil {
		return nil, fmt.Errorf("construct Factory Definitions: activation gateway is required")
	}
	if versionFileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions: version filesystem is required")
	}
	if listEffective == nil {
		return nil, fmt.Errorf("construct Factory Definitions: effective Factory catalog is required")
	}
	return lifecycle.NewWithCatalogPackagesValidationDistributionAndAuthoring(
		host, activationGateway, catalogService, validationService, authoringLayout, distribution,
		runtimeSnapshot, compilation, versionFileSystem, listEffective, snapshotsPortability,
	), nil
}

// NewLifecycleHost binds completed session, loading and persistence operations.
func NewLifecycleHost(
	sessionHost factorydefinitions.SessionHost,
	persistence factorydefinitions.Persistence,
	loader *compilationloading.Loader,
	namedPaths factorydefinitions.NamedPathResolver,
	preparePortable factorydefinitions.PortableFactoryConfigPreparer,
	captureSnapshot factorydefinitions.FactorySnapshotCapturer,
) (LifecycleHost, error) {
	if sessionHost == nil {
		return nil, fmt.Errorf("construct Factory Definitions: session host is required")
	}
	if persistence == nil {
		return nil, fmt.Errorf("construct Factory Definitions: persistence is required")
	}
	if loader == nil {
		return nil, fmt.Errorf("construct Factory Definitions: loader is required")
	}
	if namedPaths == nil {
		return nil, fmt.Errorf("construct Factory Definitions: named path resolver is required")
	}
	return lifecycle.NewHost(
		sessionHost.PersistRootDir, sessionHost.WorkstationLoader, loader.LoadRuntimeSource,
		namedPaths.ReadCurrentPointer,
		func(segment string, payload []byte) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
			return persistence.PrepareFactoryLayout(context.Background(), segment, payload)
		},
		persistence.CreateNamedFactory, namedPaths.WriteCurrentPointer, preparePortable, captureSnapshot,
		sessionHost.CurrentRuntimeConfig, sessionHost.WorkflowID, namedPaths.ResolveExistingDir,
		sessionHost.RequireSession, sessionHost.SessionRuntimeConfig, sessionHost.SessionFactoryPersistRoot,
		sessionHost.ValidateEditableFactorySnapshot, sessionHost.GetCurrentFactorySnapshotForSession,
		persistence.ReplaceFactoryLayout,
	)
}
