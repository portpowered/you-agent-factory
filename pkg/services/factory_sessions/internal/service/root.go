package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionregistry"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
)

var _ factorysessions.Service = (*Root)(nil)
var _ roles.RuntimeAssembly = (*Root)(nil)

// NewAssembly constructs the single process-scoped Factory Sessions assembly
// used as the runtime resolver while the remaining peer roots are composed.
// It returns the existing assembly capability rather than a second product
// service. The canonical public root is wrapped around this same assembly only
// after its process-scoped opening capability has been built.
func NewAssembly(
	registry sessionregistry.Service,
	state *sessionruntime.Service,
	streams legacyservice.StreamManager,
	invoker roles.InvocationService,
	control legacyservice.SessionScopeControl,
	activation legacyservice.SessionScopeActivation,
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreams responsestreamservice.Service,
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedHistory legacyservice.RecordedHistory,
) (roles.RuntimeAssembly, error) {
	if activation == nil {
		return nil, fmt.Errorf("construct Factory Sessions: scope activation is required")
	}
	if control == nil {
		return nil, fmt.Errorf("construct Factory Sessions: scoped control is required")
	}
	if invoker == nil {
		return nil, fmt.Errorf("construct Factory Sessions: invocation owner is required")
	}
	if err := validateRootDependencies(
		sessionResultProjection,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		initialWorkFiles,
		identityService,
		responseStreams,
	); err != nil {
		return nil, err
	}
	if err := validateRootRuntimeDependencies(clock, liveChangeCoordinator); err != nil {
		return nil, err
	}
	assemblyRole := legacyservice.NewAssembly(
		registry, state, streams, invoker, control, activation,
		newJavaScriptCheckpointStore,
		sessionResultProjection,
		clock,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		initialWorkFiles,
		identityService,
		responseStreams,
		liveChangeCoordinator,
		recordedHistory,
	)
	assembly, ok := assemblyRole.(*legacyservice.Assembly)
	if !ok || assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	return assembly, nil
}

// NewRootFromAssembly binds the already-composed assembly to the one
// process-scoped Factory Sessions root.
func NewRootFromAssembly(
	assembly roles.RuntimeAssembly,
	processRoot *Root,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) (*Root, error) {
	if assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: runtime assembly is required")
	}
	if liveChangeCoordinator == nil {
		return nil, fmt.Errorf("construct Factory Sessions: live-change coordinator is required")
	}
	concrete, ok := assembly.(*legacyservice.Assembly)
	if !ok || concrete == nil {
		return nil, fmt.Errorf("construct Factory Sessions: runtime assembly implementation rejected")
	}
	root := processRoot
	if root == nil {
		return nil, fmt.Errorf("construct Factory Sessions: process root is required")
	}
	if root.Assembly != nil && root.Assembly != concrete {
		return nil, fmt.Errorf("construct Factory Sessions: process root is already bound to another assembly")
	}
	if root.factorySessionExecutionFactory != nil {
		processDurable, err := root.buildProcessDurableExecution()
		if err != nil {
			return nil, err
		}
		if err := concrete.BindProcessDurable(processDurable); err != nil {
			return nil, err
		}
	}
	root.Assembly = concrete
	root.liveChangeCoordinator = liveChangeCoordinator
	return root, nil
}

func validateRootDependencies(
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreams responsestreamservice.Service,
) error {
	if sessionResultProjection == nil {
		return fmt.Errorf("construct Factory Sessions: session result projection is required")
	}
	if eventIDs == nil {
		return fmt.Errorf("construct Factory Sessions: response event ID generator is required")
	}
	if sessionIDs == nil {
		return fmt.Errorf("construct Factory Sessions: session ID generator is required")
	}
	if resolveHome == nil {
		return fmt.Errorf("construct Factory Sessions: home directory resolver is required")
	}
	if directoryInspection == nil {
		return fmt.Errorf("construct Factory Sessions: directory inspection is required")
	}
	if namedPaths == nil {
		return fmt.Errorf("construct Factory Sessions: named path resolver is required")
	}
	if initialWorkFiles == nil {
		return fmt.Errorf("construct Factory Sessions: initial Work reader is required")
	}
	if identityService == nil {
		return fmt.Errorf("construct Factory Sessions: identity service is required")
	}
	if responseStreams == nil {
		return fmt.Errorf("construct Factory Sessions: response-stream service is required")
	}
	return nil
}

func validateRootRuntimeDependencies(
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
) error {
	if clock == nil {
		return fmt.Errorf("construct Factory Sessions: clock is required")
	}
	if liveChangeCoordinator == nil {
		return fmt.Errorf("construct Factory Sessions: live-change coordinator is required")
	}
	return nil
}
