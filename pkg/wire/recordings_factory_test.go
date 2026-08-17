package wire

import (
	"context"
	"strings"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestProvideRecordingsRootConstructsThroughRecordingsWire(t *testing.T) {
	t.Parallel()

	root, err := provideRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	if root == nil {
		t.Fatal("provideRecordingsRoot() returned nil root")
	}
	var published recordings.Service = root
	if _, err := published.LoadReplayRecording(recordings.LoadReplayRecordingRequest{
		RecordingID: "missing-wire-factory-root",
	}); err == nil {
		t.Fatal("LoadReplayRecording() error = nil, want missing recording failure")
	}
}

func TestWireUsesPrecomposedRecordingsRuntimeAndMCPRoles(t *testing.T) {
	t.Parallel()

	root, err := provideRecordingsRoot(
		serviceedges.Edges{},
		recordings.LiveRecordingTargetPlannerFunc(
			func(recordings.LiveRecordingTargetRequest) (recordings.LiveRecordingTarget, error) {
				return recordings.LiveRecordingTarget{}, nil
			},
		),
		platformreplay.Local{}, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("provideRecordingsRoot() error = %v", err)
	}
	opening, err := provideRecordingsRuntimeOpening(root)
	if err != nil || opening == nil {
		t.Fatalf("provideRecordingsRuntimeOpening(root) = %v, %v; want runtime opening", opening, err)
	}
	if _, err := provideRecordingsRuntimeOpening(nil); err == nil {
		t.Fatal("provideRecordingsRuntimeOpening(nil) error = nil, want capability validation")
	}

	buildServer := provideMCPServerBuilder()
	if buildServer == nil {
		t.Fatal("provideMCPServerBuilder() returned nil")
	}
	if server, err := buildServer(nil, nil, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(nil roles) = %v, %v; want inert protocol server", server, err)
	}
	if server, err := buildServer(nil, root, nil, nil); err != nil || server == nil {
		t.Fatalf("buildServer(recordings root) = %v, %v; want owner-backed protocol server", server, err)
	}

	if _, err := provideHTTPRuntimeBinding(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideHTTPRuntimeBinding(nil roles) error = nil, want required-owner validation")
	}
}

func TestHTTPRuntimeBindingRejectsMissingOpenedRoles(t *testing.T) {
	t.Parallel()

	_, err := newHTTPRuntimeHandler(factorysessionwire.OpenedApplicationRuntime{}, nil, nil, nil, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "opened Factory Session roles") {
		t.Fatalf("newHTTPRuntimeHandler() error = %v, want missing opened roles", err)
	}
}

func TestHTTPRuntimeBindingRejectsUnavailableModels(t *testing.T) {
	t.Parallel()

	opened := wireHTTPOpenedRuntime(&wireHTTPSessionsRole{})
	_, err := newHTTPRuntimeHandler(opened, nil, nil, &wireHTTPContentRole{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Models service") {
		t.Fatalf("newHTTPRuntimeHandler() error = %v, want missing Models service", err)
	}
}

func TestHTTPRuntimeBindingRejectsMissingSessionStatusCapability(t *testing.T) {
	t.Parallel()

	opened := wireHTTPOpenedRuntime(&wireHTTPSessionsRole{})
	opened.Models = &wireHTTPModelsRole{}
	opened.ModelInvoker = &wireHTTPModelInvokerRole{}
	_, err := newHTTPRuntimeHandler(opened, nil, nil, &wireHTTPContentRole{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "session-scoped status observation") {
		t.Fatalf("newHTTPRuntimeHandler() error = %v, want missing status capability", err)
	}
}

func TestHTTPRuntimeBindingRejectsMissingLiveGatewayCapability(t *testing.T) {
	t.Parallel()

	opened := wireHTTPOpenedRuntime(&wireHTTPStatusOnlySessionsRole{})
	opened.Models = &wireHTTPModelsRole{}
	opened.ModelInvoker = &wireHTTPModelInvokerRole{}
	_, err := newHTTPRuntimeHandler(opened, nil, nil, &wireHTTPContentRole{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "live result gateway") {
		t.Fatalf("newHTTPRuntimeHandler() error = %v, want missing live gateway", err)
	}
}

func TestDirectJavaScriptHTTPCompositionRejectsMissingRoles(t *testing.T) {
	t.Parallel()

	if _, err := provideDirectJavaScriptHostAdapter(nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("provideDirectJavaScriptHostAdapter(nil roles) error = nil, want required-role validation")
	}
	if _, err := newDurableExecutionHTTPHandler(nil, nil, nil, nil, nil); err == nil {
		t.Fatal("newDurableExecutionHTTPHandler(nil roles) error = nil, want required-role validation")
	}
}

type wireHTTPRuntimeRole struct {
	factoryruntime.Service
}

func (*wireHTTPRuntimeRole) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	return nil, nil
}

type wireHTTPDefinitionsRole struct {
	factorydefinitions.Service
}

type wireHTTPSessionsRole struct {
	factorysessions.Service
}

type wireHTTPStatusOnlySessionsRole struct {
	factorysessions.Service
}

func (*wireHTTPStatusOnlySessionsRole) ObserveForSession(
	context.Context,
	string,
	factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	return factoryruntime.ObserveResult{}, nil
}

type wireHTTPLiveControlRole struct {
	factorysessions.LiveControlService
}

type wireHTTPModelsRole struct {
	models.Service
}

type wireHTTPModelInvokerRole struct {
	workers.ModelInvoker
}

type wireHTTPContentRole struct {
	work.ContentPreparation
}

func wireHTTPOpenedRuntime(sessions factorysessions.Service) factorysessionwire.OpenedApplicationRuntime {
	return factorysessionwire.OpenedApplicationRuntime{
		FactoryRuntime:     &wireHTTPRuntimeRole{},
		FactoryDefinitions: &wireHTTPDefinitionsRole{},
		FactorySessions:    sessions,
		LiveControl:        &wireHTTPLiveControlRole{},
		Logger:             zap.NewNop(),
	}
}
