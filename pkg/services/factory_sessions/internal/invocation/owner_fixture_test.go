package invocation

import (
	"context"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type sessionOwnerFixture struct {
	FactoryConfig   func(string) (*interfaces.FactoryConfig, error)
	SubmitWork      func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe         func(context.Context, string, SessionInvocationWaitInput) (SessionInvocationObservation, error)
	WaitNext        func(context.Context) error
	WaitSession     func(context.Context, string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter)
	Telemetry       SessionInvocationTelemetry
	SpecialCase     SessionInvocationSpecialCase
	Interpolation   interfaces.InvocationInterpolationService
	WorkTypes       interfaces.InvocationWorkTypeService
	InputFiles      fileeffects.InvocationInputReader
	Work            work.Service
	CancelOnTimeout func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
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
		fixtureAuthority{fixture}, fixtureControls{fixture.CancelOnTimeout},
		fixture.Telemetry, fixture.SpecialCase, interpolation, workTypes, inputFiles, workService,
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

// Controlled authority keeps the owner isolated from Runtime and subscriptions.
type fixtureAuthority struct{ fixture sessionOwnerFixture }

func (a fixtureAuthority) FactoryConfig(id string) (*interfaces.FactoryConfig, error) {
	return a.fixture.FactoryConfig(id)
}
func (a fixtureAuthority) SubmitWork(ctx context.Context, id string, req work.SubmitRequest) (work.WorkRequestSubmitResult, error) {
	return a.fixture.SubmitWork(ctx, id, req)
}
func (a fixtureAuthority) Observe(ctx context.Context, id string, input SessionInvocationWaitInput) (SessionInvocationObservation, error) {
	return a.fixture.Observe(ctx, id, input)
}
func (a fixtureAuthority) WaitSession(ctx context.Context, id string) (SessionInvocationWaiter, ReleaseSessionInvocationWaiter) {
	if a.fixture.WaitSession != nil {
		if waiter, release := a.fixture.WaitSession(ctx, id); waiter != nil {
			if release == nil {
				release = func() {}
			}
			return waiter, release
		}
	}
	if a.fixture.WaitNext != nil {
		return a.fixture.WaitNext, func() {}
	}
	return func(ctx context.Context) error { return ctx.Err() }, func() {}
}

type fixtureControls struct {
	cancel func(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error)
}

func (c fixtureControls) CancelLiveFactorySession(ctx context.Context, id string, req factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	if c.cancel == nil {
		return factorysessions.LifecycleControlResult{}, nil
	}
	return c.cancel(ctx, id, req)
}
