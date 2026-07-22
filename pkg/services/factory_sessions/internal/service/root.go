package service

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	identity "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/identity"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/service"
)

// Root retains process-scoped Factory Sessions dependencies. It is inert until
// runtime opening binds a clock selected from the invocation's external edges.
type Root struct {
	factorysessions.Service
	newJavaScriptCheckpointStore factoryruntime.JavaScriptCheckpointStoreFactory
	sessionResultProjection      factoryruntime.SessionResultProjectionOperation
	interpolation                factorydefinitions.InvocationInterpolationService
	invocationWorkTypes          factorydefinitions.InvocationWorkTypeService
	ttsObservability             factorydefinitions.TTSObservabilityService
	eventIDs                     factorysessions.ResponseEventIDGenerator
	sessionIDs                   factorysessions.SessionIDGenerator
	resolveHome                  factorysessions.HomeDirectoryResolver
	directoryInspection          factorysessions.DirectoryInspection
	namedPaths                   factorydefinitions.NamedPathResolver
	invocationInputFiles         fileeffects.InvocationInputReader
	initialWorkFiles             fileeffects.InitialWorkReader
	identity                     identity.Service
	responseStreams              responsestreamservice.Service
}

var _ factorysessions.Service = (*Root)(nil)

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
	directoryInspection factorysessions.DirectoryInspection,
	namedPaths factorydefinitions.NamedPathResolver,
	invocationInputFiles fileeffects.InvocationInputReader,
	initialWorkFiles fileeffects.InitialWorkReader,
	identityService identity.Service,
	responseStreams responsestreamservice.Service,
) (*Root, error) {
	if sessionResultProjection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session result projection is required")
	}
	if eventIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: response event ID generator is required")
	}
	if sessionIDs == nil {
		return nil, fmt.Errorf("construct Factory Sessions: session ID generator is required")
	}
	if resolveHome == nil {
		return nil, fmt.Errorf("construct Factory Sessions: home directory resolver is required")
	}
	if directoryInspection == nil {
		return nil, fmt.Errorf("construct Factory Sessions: directory inspection is required")
	}
	if namedPaths == nil {
		return nil, fmt.Errorf("construct Factory Sessions: named path resolver is required")
	}
	if invocationInputFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: invocation input reader is required")
	}
	if initialWorkFiles == nil {
		return nil, fmt.Errorf("construct Factory Sessions: initial Work reader is required")
	}
	if identityService == nil {
		return nil, fmt.Errorf("construct Factory Sessions: identity service is required")
	}
	if responseStreams == nil {
		return nil, fmt.Errorf("construct Factory Sessions: response-stream service is required")
	}
	return &Root{
		newJavaScriptCheckpointStore: newJavaScriptCheckpointStore,
		sessionResultProjection:      sessionResultProjection,
		interpolation:                interpolation,
		invocationWorkTypes:          invocationWorkTypes,
		ttsObservability:             ttsObservability,
		eventIDs:                     eventIDs,
		sessionIDs:                   sessionIDs,
		resolveHome:                  resolveHome,
		directoryInspection:          directoryInspection,
		namedPaths:                   namedPaths,
		invocationInputFiles:         invocationInputFiles,
		initialWorkFiles:             initialWorkFiles,
		identity:                     identityService,
		responseStreams:              responseStreams,
	}, nil
}

// ForRuntime binds invocation-local runtime data to the already-constructed
// service and returns an isolated live-session assembly.
func (r *Root) ForRuntime(binding factorysessions.RuntimeBinding) (factorysessions.RuntimeAssembly, error) {
	if r == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime: service is required")
	}
	if binding.Clock == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime: clock is required")
	}
	assembly := legacyservice.NewAssembly(
		r.newJavaScriptCheckpointStore,
		r.sessionResultProjection,
		r.interpolation,
		r.invocationWorkTypes,
		r.ttsObservability,
		binding.Clock,
		r.eventIDs,
		r.sessionIDs,
		r.resolveHome,
		r.directoryInspection,
		r.namedPaths,
		r.invocationInputFiles,
		r.initialWorkFiles,
		r.identity,
		r.responseStreams,
	)
	if assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime: implementation rejected its dependencies")
	}
	return assembly, nil
}
