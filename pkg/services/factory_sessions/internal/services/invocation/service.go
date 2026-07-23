// Package invocation defines the owner-private Factory Session invocation
// capability consumed by the outer Factory Sessions runtime.
package invocation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service owns request preparation, Work submission, result waiting, and
// invocation telemetry for one bound Factory Sessions runtime.
type Service interface {
	roles.SessionInvoker
	roles.InvocationInputResolver
}

// Dependencies are the exact runtime and effect ports needed by invocation.
// They contain no process-wide service bag and are safe to bind independently
// for each opened Factory Sessions runtime.
type Dependencies struct {
	FactoryConfig func(string) (*factorydefinitions.FactoryConfig, error)
	SubmitWork    func(context.Context, string, work.SubmitRequest) (work.WorkRequestSubmitResult, error)
	Observe       func(context.Context, string, legacyinvocation.SessionInvocationWaitInput) (legacyinvocation.SessionInvocationObservation, error)
	WaitNext      func(context.Context) error
	Telemetry     legacyinvocation.SessionInvocationTelemetry
	SpecialCase   legacyinvocation.SessionInvocationSpecialCase
	Interpolation factorydefinitions.InvocationInterpolationService
	WorkTypes     factorydefinitions.InvocationWorkTypeService
	InputFiles    fileeffects.InvocationInputReader
}
