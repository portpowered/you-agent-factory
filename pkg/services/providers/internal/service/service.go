// Package service implements the published Providers root Service by
// delegating catalog list/get to the parent-private catalog subservice.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

// Service fulfills the published Providers root contract.
type Service struct {
	catalog     catalog.Service
	execution   execution.Service
	acp         acp.Service
	packagedACP []providers.ACPIntegration
	lifecycles  []providers.Lifecycle
	logger      logging.Logger
	attempts    *liveAttemptRegistry
}

var _ providers.Service = (*Service)(nil)

// New constructs an inert Providers root facade over its two private sibling
// capabilities. logger is the direct, required operation-logging
// abstraction; callers with no operation logging pass logging.NoopLogger{}.
func New(catalogService catalog.Service, executionService execution.Service, logger logging.Logger) (providers.Service, error) {
	return newService(catalogService, executionService, nil, nil, logger, nil)
}

// NewWithACP constructs the production Providers root with its persistent ACP
// subservice and exact lifecycle roles. logger is the direct, required
// operation-logging abstraction; callers with no operation logging pass
// logging.NoopLogger{}.
func NewWithACP(
	catalogService catalog.Service,
	executionService execution.Service,
	acpService acp.Service,
	packagedACP []providers.ACPIntegration,
	logger logging.Logger,
	lifecycles ...providers.Lifecycle,
) (providers.Service, error) {
	return newService(catalogService, executionService, acpService, packagedACP, logger, lifecycles)
}

func newService(
	catalogService catalog.Service,
	executionService execution.Service,
	acpService acp.Service,
	packagedACP []providers.ACPIntegration,
	logger logging.Logger,
	lifecycles []providers.Lifecycle,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	if executionService == nil {
		return nil, fmt.Errorf("construct Providers: execution is required")
	}
	for index, lifecycle := range lifecycles {
		if lifecycle == nil {
			return nil, fmt.Errorf("construct Providers: lifecycle %d is required", index)
		}
	}
	return &Service{
		catalog:     catalogService,
		execution:   executionService,
		acp:         acpService,
		packagedACP: cloneACPIntegrations(packagedACP),
		lifecycles:  append([]providers.Lifecycle(nil), lifecycles...),
		logger:      logging.EnsureLogger(logger),
		attempts:    newLiveAttemptRegistry(),
	}, nil
}

func (s *Service) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	listed, err := s.catalog.ListProviders(ctx, request)
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	byID := make(map[providers.ID]int, len(listed.Providers))
	for index, descriptor := range listed.Providers {
		byID[descriptor.ID] = index
	}
	if s.acp == nil {
		return listed, nil
	}
	for _, integration := range s.acp.Integrations() {
		descriptor := acpDescriptor(integration)
		if index, exists := byID[descriptor.ID]; exists {
			listed.Providers[index] = descriptor
		} else {
			listed.Providers = append(listed.Providers, descriptor)
		}
	}
	sort.Slice(listed.Providers, func(i, j int) bool { return listed.Providers[i].ID < listed.Providers[j].ID })
	return listed, nil
}

func (s *Service) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if s.acp != nil {
		if canonical, ok := s.acp.Resolve(request.ID); ok {
			for _, integration := range s.acp.Integrations() {
				if integration.Name == canonical {
					return providers.GetProviderResult{Provider: acpDescriptor(integration)}, nil
				}
			}
		}
	}
	return s.catalog.GetProvider(ctx, request)
}

func (s *Service) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: err.Error(),
		}
	}
	request.ReasoningEffort, _ = providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if s.acp != nil {
		if canonical, ok := s.acp.Resolve(request.Provider); ok {
			if request.ReasoningEffort != "" {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind: providers.ExecuteFailureKindInvalidRequest,
					Message: fmt.Sprintf(
						"ACP provider %q selects reasoning effort through its exact advertised model id; omit reasoningEffort and choose the intended model",
						canonical,
					),
				}
			}
			request.Provider = canonical
			control := &acpAttemptControl{acp: s.acp, canonical: canonical, attemptID: request.AttemptID}
			release, bindErr := s.bindLiveAttempt(canonical, request.AttemptID, control)
			if bindErr != nil {
				return providers.ExecuteResult{}, bindErr
			}
			defer release()
			return s.acp.Execute(ctx, canonical, request)
		}
	}
	canonicalProvider, err := s.catalog.ResolveProviderID(request.Provider)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	request.Provider = canonicalProvider
	if request.Provider == providers.IDClaude && request.ReasoningEffort == "minimal" {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: `Claude does not support reasoning effort "minimal"`,
		}
	}
	if request.Provider == providers.IDAntigravity && request.ReasoningEffort != "" {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: "Agy does not support a separate reasoning effort",
		}
	}
	attemptCtx, cancelAttempt := context.WithCancel(ctx)
	control := &nativeAttemptControl{cancel: cancelAttempt, done: make(chan struct{})}
	release, bindErr := s.bindLiveAttempt(canonicalProvider, request.AttemptID, control)
	if bindErr != nil {
		cancelAttempt()
		return providers.ExecuteResult{}, bindErr
	}
	defer release()
	defer cancelAttempt()
	return s.executeNativeAttempt(attemptCtx, control, request)
}

// executeNativeAttempt runs the bound native execution and records its real
// outcome on control before returning - via defer, so an unexpected unwind
// (including a panic) still closes control.done instead of leaving a
// concurrent claimed signal() call blocked until its own ctx ends.
// control.finish closes done itself, synchronously, right after Execute
// returns and before this defer's caller's own deferred release/
// cancelAttempt run. A concurrent ControlAttempt claim that lands in the
// registry-release race window therefore still only ever observes the true
// recorded outcome once it waits on done (see nativeAttemptControl.finish) -
// a natural success can never be reported as ControlOutcomeCompleted merely
// because a claim happened to land after Execute already returned.
func (s *Service) executeNativeAttempt(
	attemptCtx context.Context,
	control *nativeAttemptControl,
	request providers.ExecuteRequest,
) (result providers.ExecuteResult, err error) {
	defer func() {
		control.finish(errors.Is(err, providers.ErrExecuteCancelled))
	}()
	result, err = s.execution.Execute(attemptCtx, request)
	return result, err
}

// bindLiveAttempt registers canonical/attemptID as the one live execution for
// that exact identity before its controllable provider operation begins. A
// collision with an already-live identity is reported through the same typed
// ExecuteFailure model as any other Execute rejection, before any provider
// side effect for the second request begins. control is the exact-attempt
// signal handle a later ControlAttempt call can claim.
func (s *Service) bindLiveAttempt(
	canonical providers.ID,
	attemptID string,
	control liveAttemptControl,
) (func(), error) {
	release, err := s.attempts.bind(liveAttemptKey{provider: canonical, attemptID: attemptID}, control)
	if err != nil {
		return nil, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: err.Error(),
		}
	}
	return release, nil
}

// ControlAttempt routes a valid cancel or terminate request to the exact
// live native provider attempt it names, or a valid cancel request to the
// exact live ACP attempt it names, when one is bound and truthfully
// supports the requested action right now, and otherwise answers with the
// closed deterministic unsupported outcome. It never signals a different
// live attempt and invokes no Worker cancellation method or continuation
// behavior.
func (s *Service) ControlAttempt(
	ctx context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		s.logger.Info(
			"provider control attempt rejected",
			"provider", string(request.Provider),
			"attemptID", request.AttemptID,
			"action", string(request.Action),
			"outcome", "invalid",
		)
		return providers.ControlAttemptResult{}, err
	}
	s.logger.Info(
		"provider control attempt accepted",
		"provider", string(request.Provider),
		"attemptID", request.AttemptID,
		"action", string(request.Action),
	)
	outcome := providers.ControlOutcomeUnsupported
	key := liveAttemptKey{provider: request.Provider, attemptID: request.AttemptID}
	if control, claimed := s.attempts.claim(key, request.Action); claimed {
		accepted, err := control.signal(ctx)
		if err != nil {
			s.logger.Info(
				"provider control attempt outcome",
				"provider", string(request.Provider),
				"attemptID", request.AttemptID,
				"action", string(request.Action),
				"outcome", "failed",
			)
			return providers.ControlAttemptResult{}, err
		}
		// accepted is false only when a claimed control genuinely lost the
		// race to the attempt's own natural completion (see
		// acpAttemptControl.signal); outcome correctly stays Unsupported
		// rather than a false Completed.
		if accepted {
			outcome = providers.ControlOutcomeCompleted
		}
	}
	result := providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   outcome,
	}
	s.logger.Info(
		"provider control attempt outcome",
		"provider", string(request.Provider),
		"attemptID", request.AttemptID,
		"action", string(request.Action),
		"outcome", string(result.Outcome),
	)
	return result, nil
}

func (s *Service) ConfigureACPIntegrations(ctx context.Context, configured []providers.ACPIntegration) error {
	if s.acp == nil {
		return fmt.Errorf("configure ACP integrations: ACP service is unavailable")
	}
	return s.acp.Configure(ctx, effectiveACPIntegrations(s.packagedACP, configured))
}

func (s *Service) Close(ctx context.Context) error {
	var first error
	for _, lifecycle := range s.lifecycles {
		if err := lifecycle.Close(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func cloneACPIntegrations(values []providers.ACPIntegration) []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func effectiveACPIntegrations(packaged, configured []providers.ACPIntegration) []providers.ACPIntegration {
	values := cloneACPIntegrations(packaged)
	for _, integration := range configured {
		replaced := false
		for index := range values {
			if values[index].Name == integration.Name {
				values[index] = integration.Clone()
				replaced = true
				break
			}
		}
		if !replaced {
			values = append(values, integration.Clone())
		}
	}
	return values
}

func acpDescriptor(integration providers.ACPIntegration) providers.Descriptor {
	return providers.Descriptor{
		ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(),
		Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityReasoningSummaries, providers.CapabilityToolLifecycle, providers.CapabilityFileChanges, providers.CapabilityPlans, providers.CapabilityUsage},
	}
}

var _ providers.ACPConfiguration = (*Service)(nil)
