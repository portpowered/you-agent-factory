// Package invocation defines the owner-private Factory Session invocation
// capability consumed by the outer Factory Sessions runtime. This nested
// subservice is the FND-02 parent-private home for prepare, command-Work, and
// observe-completion behind the CTR-SES root invocation slice.
package invocation

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	legacyinvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/invocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the single named invocation subservice interface. It owns prepare
// (ResolveInvocationInput), command-Work, and observe-completion for one bound
// Factory Sessions runtime. InvokeFactorySession orchestrates those three
// phases through this private ownership rather than a second competing
// invocation authority. Implementation lives under invocation/internal/service
// and is constructed through invocation/wire.
type Service interface {
	roles.SessionInvoker
	roles.InvocationInputResolver
}

// Dependencies are the exact runtime and effect ports needed by invocation.
// They contain no process-wide service bag and are safe to bind independently
// for each opened Factory Sessions runtime. Work command ports are injected
// here; this package does not select Work implementation packages.
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
