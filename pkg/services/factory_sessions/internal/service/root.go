package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// Root is the one process-scoped Factory Sessions root. Its live-session
// assembly is constructed once by Wire and retains process-scoped registries;
// opening a session only adds private session/runtime state to that assembly.
type Root struct {
	*legacyservice.Assembly
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator
	detachedOperations    factorysessions.DetachedService
}

var _ factorysessions.Service = (*Root)(nil)
var _ roles.RuntimeAssembly = (*Root)(nil)

// NewRoot constructs the process-scoped Factory Sessions service without
// starting runtimes, listeners, or background work.
func NewRoot(
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory,
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	interpolation factorydefinitions.InvocationInterpolationService,
	invocationWorkTypes factorydefinitions.InvocationWorkTypeService,
	ttsObservability factorydefinitions.TTSObservabilityService,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreams responsestreamservice.Service,
	clock factoryruntime.Clock,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordedSessionInventory recordings.RecordedSessionInventory,
) (*Root, error) {
	if err := validateRootDependencies(
		sessionResultProjection,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		invocationInputFiles,
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
		newJavaScriptCheckpointStore,
		sessionResultProjection,
		interpolation,
		invocationWorkTypes,
		ttsObservability,
		clock,
		eventIDs,
		sessionIDs,
		resolveHome,
		directoryInspection,
		namedPaths,
		invocationInputFiles,
		initialWorkFiles,
		identityService,
		responseStreams,
		liveChangeCoordinator,
		recordedSessionInventory,
	)
	assembly, ok := assemblyRole.(*legacyservice.Assembly)
	if !ok || assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	root := &Root{
		Assembly:              assembly,
		liveChangeCoordinator: liveChangeCoordinator,
	}
	detachedOperations, err := (&factorysessions.DetachedOperations{}).Bind(assembly)
	if err != nil {
		return nil, fmt.Errorf("construct Factory Sessions: bind detached operations: %w", err)
	}
	root.detachedOperations = detachedOperations
	return root, nil
}

// DetachedOperations returns the one process-scoped operation view bound to
// the root assembly. It is intentionally a value-operation capability; the
// runtime gateway routing remains private to the assembly.
func (r *Root) DetachedOperations() factorysessions.DetachedService {
	if r == nil {
		return nil
	}
	return r.detachedOperations
}

func validateRootDependencies(
	sessionResultProjection factoryruntime.SessionResultProjectionOperation,
	eventIDs factorysessions.ResponseEventIDGenerator,
	sessionIDs factorysessions.SessionIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	directoryInspection roles.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
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
	if invocationInputFiles == nil {
		return fmt.Errorf("construct Factory Sessions: invocation input reader is required")
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
