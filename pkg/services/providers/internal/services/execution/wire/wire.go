// Package wire constructs the parent-private Providers execution subservice.
package wire

import (
	"context"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation behind
// the Workers root allocation port.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source, scheduler platformclock.TimerSource) (providerservice.PTYAllocator, error) {
	return agypty.NewAllocator(host, clock, scheduler)
}

// NewService constructs an inert execution service over the supplied canonical
// catalog and private adapter registrations.
func NewService(
	catalogService catalog.Service,
	registrations ...execution.Registration,
) (execution.ContinuationService, error) {
	return executionservice.New(catalogService, registrations...)
}

// NewACPRegistration delegates one configured ACP identity to the already
// constructed parent-private ACP service.
func NewACPRegistration(id providers.ID, service acp.ContinuationService) execution.Registration {
	return execution.Registration{
		Provider: id,
		Attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return service.Execute(ctx, id, request)
		},
		Continue: func(ctx context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
			if request.ResumeSession == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:    providers.ExecuteFailureKindInvalidRequest,
					Message: "provider continuation is missing its session reference",
				}
			}
			return service.Continue(ctx, id, request.ExecuteRequest, *request.ResumeSession)
		},
	}
}

// NewUnsupportedContinuation completes the unavailable continuation operation
// for an adapter that supports ordinary attempts only.
func NewUnsupportedContinuation() execution.ContinuationAttempt {
	return executionservice.UnsupportedContinuation{}.Continue
}

// Effect aliases expose completed native collaborators to owner composition.
type CodexEffect = codexadapter.Effect
type ClaudeEffect = claudeadapter.Effect
type AgyEffect = agyadapter.Effect
type AgyPTYPolicy = agyadapter.PTYPolicy

// BuiltInRegistrations binds individually supplied native effects.
func BuiltInRegistrations(antigravity AgyEffect, codex CodexEffect, claude ClaudeEffect) []execution.Registration {
	return executionservice.BuiltInRegistrations(antigravity, codex, claude)
}

// NewCodexEffect constructs one native Codex command effect.
func NewCodexEffect(runner providerservice.CommandRunner, clock platformclock.Source) CodexEffect {
	return codexadapter.NewCommandEffect(runner, clock)
}

// NewClaudeEffect constructs one native Claude command effect.
func NewClaudeEffect(runner providerservice.CommandRunner, clock platformclock.Source) ClaudeEffect {
	return claudeadapter.NewCommandEffect(runner, clock)
}

// NewAgyCommandEffect constructs one native AGY command effect.
func NewAgyCommandEffect(runner providerservice.CommandRunner, clock platformclock.Source, scheduler platformclock.TimerSource) AgyEffect {
	return agyadapter.NewCommandEffect(runner, clock, scheduler)
}

// NewAgyPTYEffect constructs the intentionally selected legacy PTY effect.
func NewAgyPTYEffect(allocator providerservice.PTYAllocator, locator platformprocess.ExecutableLocator, inspector platformfilesystem.PathInspector, clock platformclock.Source, policy AgyPTYPolicy) AgyEffect {
	return agyadapter.NewPTYEffect(allocator, locator, inspector, clock, policy)
}
