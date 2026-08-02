package invocation

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type sessionOwnerFixture struct {
	FactoryConfig     func(string) (*interfaces.FactoryConfig, error)
	SubmitWork        func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe           func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	WaitNext          func(context.Context) error
	Telemetry         SessionInvocationTelemetry
	SpecialCase       SessionInvocationSpecialCase
	ResolveDefinition DefinitionResolver
	InputFiles        fileeffects.InvocationInputReader
	Work              work.Service
}

func newTestSessionOwner(fixture sessionOwnerFixture) *SessionOwner {
	resolveDefinition := fixture.ResolveDefinition
	if resolveDefinition == nil {
		resolveDefinition = func(
			_ context.Context,
			_ string,
			cfg *interfaces.FactoryConfig,
			_ *work.InvocationArguments,
			_ map[string][]byte,
		) (interfaces.ResolveInvocationDefinitionResult, error) {
			if cfg == nil {
				return interfaces.ResolveInvocationDefinitionResult{}, fmt.Errorf("Factory Definitions returned no Factory")
			}
			return interfaces.ResolveInvocationDefinitionResult{Factory: *cfg, DefaultWorkType: "task"}, nil
		}
	}
	inputFiles := fixture.InputFiles
	if inputFiles == nil {
		inputFiles = func(string) ([]byte, error) { return nil, nil }
	}
	workService := fixture.Work
	if workService == nil {
		workService = testInvocationWorkService()
	}
	return NewSessionOwner(
		fixture.FactoryConfig,
		fixture.SubmitWork,
		fixture.Observe,
		fixture.WaitNext,
		fixture.Telemetry,
		fixture.SpecialCase,
		resolveDefinition,
		inputFiles,
		workService,
	)
}

type rejectingInvocationWorkType struct{ err error }

func (workType rejectingInvocationWorkType) ResolveDefinition(
	context.Context,
	string,
	*interfaces.FactoryConfig,
	*work.InvocationArguments,
	map[string][]byte,
) (interfaces.ResolveInvocationDefinitionResult, error) {
	return interfaces.ResolveInvocationDefinitionResult{}, fmt.Errorf("resolve default Work type: %w", workType.err)
}

func rejectingInvocationInterpolation(parameter string) DefinitionResolver {
	return func(
		context.Context,
		string,
		*interfaces.FactoryConfig,
		*work.InvocationArguments,
		map[string][]byte,
	) (interfaces.ResolveInvocationDefinitionResult, error) {
		return interfaces.ResolveInvocationDefinitionResult{}, &work.ArgumentError{
			Code:      work.ArgumentErrorCodeInvalidInterpolation,
			Message:   "scripted invalid invocation interpolation",
			Parameter: parameter,
		}
	}
}

func testInvocationWorkService() work.Service {
	return work.NewInvocationPolicyService()
}
