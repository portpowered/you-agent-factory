package invocation

import (
	"context"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type sessionOwnerFixture struct {
	FactoryConfig func(string) (*interfaces.FactoryConfig, error)
	SubmitWork    func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe       func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	WaitNext      func(context.Context) error
	WaitSession   func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter)
	Telemetry     SessionInvocationTelemetry
	SpecialCase   SessionInvocationSpecialCase
	Interpolation interfaces.InvocationInterpolationService
	WorkTypes     interfaces.InvocationWorkTypeService
	InputFiles    fileeffects.InvocationInputReader
	Work          work.Service
}

func newTestSessionOwner(fixture sessionOwnerFixture) *SessionOwner {
	interpolation := fixture.Interpolation
	if interpolation == nil {
		interpolation = factorydefinitionfixtures.InvocationInterpolation{}
	}
	workTypes := fixture.WorkTypes
	if workTypes == nil {
		workTypes = staticInvocationWorkType("task")
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
		fixture.WaitSession,
		fixture.Telemetry,
		fixture.SpecialCase,
		interpolation,
		workTypes,
		inputFiles,
		workService,
	)
}

type staticInvocationWorkType string

func (workType staticInvocationWorkType) DefaultWorkType(*interfaces.FactoryConfig) (string, error) {
	return string(workType), nil
}

type rejectingInvocationWorkType struct{ err error }

func (workType rejectingInvocationWorkType) DefaultWorkType(*interfaces.FactoryConfig) (string, error) {
	return "", workType.err
}

func rejectingInvocationInterpolation(parameter string) interfaces.InvocationInterpolationService {
	return factorydefinitionfixtures.InvocationInterpolation{
		Validate: func(*interfaces.FactoryConfig, *work.InvocationArguments, interfaces.FileReader) error {
			return &work.ArgumentError{
				Code:      work.ArgumentErrorCodeInvalidInterpolation,
				Message:   "scripted invalid invocation interpolation",
				Parameter: parameter,
			}
		},
	}
}

func testInvocationWorkService() work.Service {
	return work.NewInvocationPolicyService()
}
