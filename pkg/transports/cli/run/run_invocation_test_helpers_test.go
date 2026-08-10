package run

import (
	"context"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"testing"
)

const packagedGoalFactoryName = "@you/goal"
const packagedGoalExecuteWorkstationName = "execute-goal"

func packagedRunFixtureResolution(
	name string,
	factoryDir string,
	globalRoot string,
) *interfaces.NamedFactoryResolution {
	return &interfaces.NamedFactoryResolution{
		Name:       name,
		FactoryDir: factoryDir,
		Source:     interfaces.NamedFactoryResolutionSourceGlobal,
		GlobalRoot: globalRoot,
	}
}

var goal = struct {
	PackagedFactoryName            string
	PackagedExecuteWorkstationName string
}{
	PackagedFactoryName:            packagedGoalFactoryName,
	PackagedExecuteWorkstationName: packagedGoalExecuteWorkstationName,
}

type stubInvocationService struct {
	run    func(context.Context) error
	invoke func(context.Context, string, factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error)
	close  func(context.Context, string) error
	events []interfaces.FactoryEvent
}

func (s stubInvocationService) Run(ctx context.Context) error {
	return s.run(ctx)
}

func (s stubInvocationService) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{Name: "portable"}, nil
}

func (s stubInvocationService) GetFactoryEvents(context.Context) ([]interfaces.FactoryEvent, error) {
	events := make([]interfaces.FactoryEvent, len(s.events))
	for i := range s.events {
		events[i] = s.events[i].Clone()
	}
	return events, nil
}

func (s stubInvocationService) InvokeFactorySession(ctx context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
	return s.invoke(ctx, sessionID, request)
}

func (s stubInvocationService) CloseFactorySession(ctx context.Context, sessionID string) error {
	if s.close != nil {
		return s.close(ctx, sessionID)
	}
	return nil
}

func extractInvocationText(t *testing.T, request *factoryapi.InvocationRequest) string {
	t.Helper()

	if request == nil {
		t.Fatal("invocation request = nil")
	}
	if request.Content == nil {
		t.Fatal("content = nil, want one text part")
	}
	parts := *request.Content
	if len(parts) != 1 {
		t.Fatalf("content parts = %d, want 1", len(parts))
	}
	part, err := parts[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("AsWorkTextContentPart: %v", err)
	}
	return part.Text
}
