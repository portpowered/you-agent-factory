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
)

// Root is the one process-scoped Factory Sessions root. Its live-session
// assembly is constructed once by Wire and retains process-scoped registries;
// opening a session only adds private session/runtime state to that assembly.
type Root struct {
	factorysessions.Service
	*legacyservice.Assembly
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
	if clock == nil {
		return nil, fmt.Errorf("construct Factory Sessions: clock is required")
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
	)
	assembly, ok := assemblyRole.(*legacyservice.Assembly)
	if !ok || assembly == nil {
		return nil, fmt.Errorf("construct Factory Sessions: implementation rejected its dependencies")
	}
	root := &Root{Service: &legacyservice.Service{}, Assembly: assembly}
	if err := validateCompatibilityBinding(root, clock); err != nil {
		return nil, fmt.Errorf("construct Factory Sessions: compatibility binding rejected root: %w", err)
	}
	return root, nil
}

// validateCompatibilityBinding keeps the published compatibility contract
// checked against the exact process root. The binding is inert: it neither
// constructs a child service nor starts runtime work.
func validateCompatibilityBinding(root *Root, clock factoryruntime.Clock) error {
	_, err := root.ForRuntime(factorysessions.RuntimeBinding{Clock: clock})
	return err
}

// ForRuntime is retained as a compatibility binding for callers that have not
// yet moved to the direct runtime-root port. It never constructs or returns a
// child service; the process root already owns the shared assembly.
func (r *Root) ForRuntime(binding factorysessions.RuntimeBinding) (factorysessions.Service, error) {
	if r == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime: service is required")
	}
	if binding.Clock == nil {
		return nil, &factorysessions.OpeningBindingError{
			Field:   "clock",
			Message: "clock is required",
		}
	}
	return r, nil
}
