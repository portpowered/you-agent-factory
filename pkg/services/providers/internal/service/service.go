// Package service implements the published Providers root Service by
// delegating catalog list/get to the parent-private catalog subservice.
package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
func New(catalogService catalog.Service, executionService execution.Service, logger logging.Logger) (*Service, error) {
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
) (*Service, error) {
	return newService(catalogService, executionService, acpService, packagedACP, logger, lifecycles)
}

func newService(
	catalogService catalog.Service,
	executionService execution.Service,
	acpService acp.Service,
	packagedACP []providers.ACPIntegration,
	logger logging.Logger,
	lifecycles []providers.Lifecycle,
) (*Service, error) {
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
		if _, err := s.catalog.RegistrationProvider(integration.Name); err == nil {
			// Packaged ACP metadata is authored in the provider package and was
			// already returned by Catalog. Do not replace it with a generic
			// runtime-only descriptor.
			continue
		}
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
			if descriptor, err := s.catalog.GetProvider(ctx, providers.GetProviderRequest{ID: canonical}); err == nil {
				return descriptor, nil
			}
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
	return s.dispatch(ctx, request)
}

// dispatch runs one normalized attempt for an already-validated request,
// resolving it to the exact ACP or native adapter its canonical provider
// names. Execute and Continue share this path so a continued attempt reaches
// its adapter through the identical routing an ordinary attempt uses.
func (s *Service) dispatch(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
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
			if err := s.validatePermissionBypass(canonical, request.SkipPermissions); err != nil {
				return providers.ExecuteResult{}, err
			}
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
	if err := s.validatePermissionBypass(request.Provider, request.SkipPermissions); err != nil {
		return providers.ExecuteResult{}, err
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

// dispatchContinuation routes one validated continuation through the same
// provider selection and live-attempt barriers as Execute while carrying its
// exact session reference only through private Providers services.
func (s *Service) dispatchContinuation(
	ctx context.Context,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (providers.ExecuteResult, error) {
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
			if err := s.validatePermissionBypass(canonical, request.SkipPermissions); err != nil {
				return providers.ExecuteResult{}, err
			}
			control := &acpAttemptControl{acp: s.acp, canonical: canonical, attemptID: request.AttemptID}
			release, bindErr := s.bindLiveAttempt(canonical, request.AttemptID, control)
			if bindErr != nil {
				return providers.ExecuteResult{}, bindErr
			}
			defer release()
			continuation, ok := s.acp.(acp.ContinuationService)
			if !ok {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:    providers.ExecuteFailureKindDependency,
					Message: "ACP provider continuation is unavailable",
				}
			}
			return continuation.Continue(ctx, canonical, request, reference)
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
	if err := s.validatePermissionBypass(request.Provider, request.SkipPermissions); err != nil {
		return providers.ExecuteResult{}, err
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
	return s.executeNativeContinuation(attemptCtx, control, request, reference)
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

func (s *Service) executeNativeContinuation(
	attemptCtx context.Context,
	control *nativeAttemptControl,
	request providers.ExecuteRequest,
	reference providers.SessionRef,
) (result providers.ExecuteResult, err error) {
	defer func() {
		control.finish(errors.Is(err, providers.ErrExecuteCancelled))
	}()
	continuation, ok := s.execution.(execution.ContinuationService)
	if !ok {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "provider continuation adapter is unavailable",
		}
	}
	return continuation.Continue(attemptCtx, execution.ContinuationRequest{
		ExecuteRequest: request,
		ResumeSession:  &reference,
	})
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

// Continue resolves the exact provider ContinueRequest.Reference names and
// resumes it for one continued attempt. A malformed or foreign reference
// fails validation before this method inspects any catalog or ACP state. An
// unknown provider fails with the same typed error GetProvider/Execute use.
// A resolved provider or session kind that cannot continue returns the
// closed unsupported outcome without invoking any provider adapter, binding
// a live attempt, or starting a fresh execution. A valid reference reaches
// its adapter through the identical dispatch path Execute uses, with
// Reference forwarded unchanged through the private continuation seam.
func (s *Service) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		s.logger.Info(
			"provider continuation rejected",
			"provider", string(request.Reference.Provider),
			"kind", request.Reference.Kind,
		)
		return providers.ContinueResult{}, err
	}
	reference := request.Reference.Clone()
	s.logger.Info(
		"provider continuation received",
		"provider", string(reference.Provider),
		"kind", reference.Kind,
	)

	canonical, supported, err := s.resolveContinuationProvider(reference)
	if err != nil {
		s.logger.Info(
			"provider continuation outcome",
			"provider", string(reference.Provider),
			"kind", reference.Kind,
			"outcome", "failed",
		)
		return providers.ContinueResult{}, err
	}
	reference.Provider = canonical
	if !supported {
		s.logger.Info(
			"provider continuation outcome",
			"provider", string(canonical),
			"kind", reference.Kind,
			"outcome", "unsupported",
		)
		return providers.ContinueResult{
			Reference: reference,
			Outcome:   providers.ContinuationOutcomeUnsupported,
		}, nil
	}

	attempt := request.Attempt.Clone()
	attempt.Provider = canonical
	attempt.ReasoningEffort, _ = providers.ReasoningEffort(attempt.ReasoningEffort).Canonical()

	result, err := s.dispatchContinuation(ctx, attempt, reference)
	if err != nil {
		if stale := staleContinuationFailure(err, reference); stale != nil {
			s.logger.Info(
				"provider continuation outcome",
				"provider", string(canonical),
				"kind", reference.Kind,
				"outcome", "stale",
			)
			return providers.ContinueResult{}, *stale
		}
		s.logger.Info(
			"provider continuation outcome",
			"provider", string(canonical),
			"kind", reference.Kind,
			"outcome", "failed",
		)
		return providers.ContinueResult{}, err
	}
	s.logger.Info(
		"provider continuation outcome",
		"provider", string(canonical),
		"kind", reference.Kind,
		"outcome", "resumed",
	)
	return providers.ContinueResult{
		Reference: reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    result,
	}, nil
}

// resolveContinuationProvider returns the canonical provider identity for
// reference.Provider and whether that resolved provider truthfully
// advertises CapabilitySessionResume, without invoking any provider adapter
// or catalog readiness probe. For an ACP integration whose daemon has
// already completed at least one real initialize handshake (necessarily true
// for any session identity a caller could legitimately hold, since a
// Provider Session can only exist after a prior successful attempt against
// that same daemon), it answers with the daemon's actual negotiated
// LoadSession capability instead of the static config-time claim, so a
// daemon that cannot really resume is reported as unsupported rather than
// left to fail loudly deeper in the adapter. err is the same typed error
// ListProviders/GetProvider/Execute use for an unknown provider.
func (s *Service) resolveContinuationProvider(
	reference providers.SessionRef,
) (canonical providers.ID, supported bool, err error) {
	if reference.Kind != providers.SessionIDKind {
		return "", false, providers.ContinuationFailure{
			Kind:      providers.ContinuationFailureKindInvalid,
			Message:   fmt.Sprintf("provider continuation session kind %q is not supported", reference.Kind),
			Reference: reference.Clone(),
		}
	}
	if s.acp != nil {
		if resolved, ok := s.acp.Resolve(reference.Provider); ok {
			for _, integration := range s.acp.Integrations() {
				if integration.Name != resolved {
					continue
				}
				if negotiated, known := s.acp.NegotiatedCapabilities(resolved); known {
					return resolved, negotiated.LoadSession, nil
				}
				return resolved, descriptorHasCapability(acpDescriptor(integration), providers.CapabilitySessionResume), nil
			}
			return resolved, false, nil
		}
	}
	resolved, err := s.catalog.ResolveProviderID(reference.Provider)
	if err != nil {
		return "", false, err
	}
	descriptor, err := s.catalog.RegistrationProvider(resolved)
	if err != nil {
		return "", false, err
	}
	return resolved, descriptorHasCapability(descriptor, providers.CapabilitySessionResume), nil
}

// staleContinuationFailure translates a dispatch failure whose resolved
// provider reported the exact requested reference id as not found (an
// ExecuteFailureKindSessionNotFound declared failure - never produced by
// ordinary Execute, since only a continuation dispatch has a prior session
// reference) into the typed stale continuation failure. It returns nil
// for any other dispatch failure, which Continue reports unchanged.
func staleContinuationFailure(err error, reference providers.SessionRef) *providers.ContinuationFailure {
	var declared providers.ExecuteFailure
	if !errors.As(err, &declared) || declared.Kind != providers.ExecuteFailureKindSessionNotFound {
		return nil
	}
	failure := providers.ContinuationFailure{
		Kind:      providers.ContinuationFailureKindStale,
		Message:   declared.Message,
		Reference: reference,
	}
	return &failure
}

func descriptorHasCapability(descriptor providers.Descriptor, capability providers.Capability) bool {
	return slices.Contains(descriptor.Capabilities, capability)
}

// validatePermissionBypass checks the static capability fact before binding a
// live attempt. This keeps an unsupported bypass request away from native
// process startup, ACP prompt traffic, and other provider side effects.
func (s *Service) validatePermissionBypass(provider providers.ID, requested bool) error {
	if !requested {
		return nil
	}
	descriptor, err := s.permissionDescriptor(provider)
	if err != nil {
		return err
	}
	if descriptorHasCapability(descriptor, providers.CapabilityPermissionBypass) {
		return nil
	}
	return providers.ExecuteFailure{
		Kind: providers.ExecuteFailureKindCapabilityMismatch,
		Message: fmt.Sprintf(
			"provider %q does not support capability %q",
			provider,
			providers.CapabilityPermissionBypass,
		),
	}
}

func (s *Service) permissionDescriptor(provider providers.ID) (providers.Descriptor, error) {
	if s.acp != nil {
		if canonical, ok := s.acp.Resolve(provider); ok {
			// A package-owned catalog descriptor is the authoritative static
			// capability fact for a packaged ACP integration. The conservative
			// runtime fallback below is only for operator-configured integrations
			// that have no published catalog entry.
			if descriptor, err := s.catalog.RegistrationProvider(canonical); err == nil {
				return descriptor, nil
			}
			for _, integration := range s.acp.Integrations() {
				if integration.Name == canonical {
					return acpDescriptor(integration), nil
				}
			}
			return providers.Descriptor{}, providers.ErrUnknownProvider
		}
	}
	return s.catalog.RegistrationProvider(provider)
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
				replacement := integration.Clone()
				if replacement.Command == values[index].Command {
					// Persisted settings carry the legacy command-only shape. Keep
					// the reviewed package runtime binding when that command still
					// identifies the package-owned launch.
					if replacement.Aliases == nil {
						replacement.Aliases = append([]string(nil), values[index].Aliases...)
					}
					if replacement.Arguments == nil {
						replacement.Arguments = append([]string(nil), values[index].Arguments...)
					}
					if replacement.RuntimePosture == "" {
						replacement.RuntimePosture = values[index].RuntimePosture
					}
					if replacement.ImplementationProfile == "" {
						replacement.ImplementationProfile = values[index].ImplementationProfile
					}
				}
				values[index] = replacement
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
		Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessUnverified,
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilitySessionResume},
	}
}

var _ providers.ACPConfiguration = (*Service)(nil)

// acpAttemptControl is the control handle bound for one live ACP provider
// attempt. Unlike nativeAttemptControl, whether Cancel is truthfully
// supported depends on live ACP session state that exists only for the span
// between the bound attempt's session/prompt call starting and returning
// (see acp.Service.Claim); before or after that span the attempt stays live
// and untouched, so a later Cancel can still succeed once the span opens.
// Pause and Terminate are never supported: the only ACP protocol seam
// available cancels one session/prompt turn, it does not pause a turn or
// shut down the daemon.
//
// generation is nil until supports(Cancel) successfully claims one; signal
// then delivers only to that captured generation, never re-deriving
// liveness from canonical/attemptID strings. This is what keeps a claimed
// control bound to the exact execution generation it was claimed from: even
// if that generation completes and a later execution reuses the identical
// canonical/attemptID identity before signal runs, signal still targets only
// the originally captured generation and cannot be redirected to the
// replacement. supports and signal are always called sequentially by the
// same caller (registry.claim, then ControlAttempt) with no intervening
// concurrent access, so generation needs no synchronization of its own.
type acpAttemptControl struct {
	acp        acp.Service
	canonical  providers.ID
	attemptID  string
	generation acp.Generation
}

var _ liveAttemptControl = (*acpAttemptControl)(nil)

// supports atomically claims the bound attempt's exact live generation, if
// its session/prompt turn is truthfully live right now, capturing it into
// generation for signal to use. It is used only to decide whether the
// registry should remove this identity's registration for a Cancel request
// at all (so a Cancel that arrives before or after the live window leaves the
// registration untouched for a later attempt); signal remains the atomic
// source of truth for whether the resulting outcome was genuinely accepted,
// grounded in the exact generation captured here rather than in a later
// re-derivation by identity.
func (control *acpAttemptControl) supports(action providers.ControlAction) bool {
	if action != providers.ControlActionCancel {
		return false
	}
	generation, ok := control.acp.Claim(control.canonical, control.attemptID)
	if !ok {
		return false
	}
	control.generation = generation
	return true
}

// signal delivers to the exact generation supports captured, via
// acp.Service.TryCancel, which grounds its accepted result in that
// generation's real recorded outcome - so a natural completion racing this
// call, or a generation that already ended before this call runs, reports
// accepted=false instead of a false ControlOutcomeCompleted, and can never
// observe a different (for example later, identity-reusing) generation's
// outcome. TryCancel already distinguishes its two non-nil-error cases by
// construction: a genuine delivery failure is wrapped in
// providers.ErrControlSignalFailed, while ctx ending before the outcome
// could be observed surfaces as the caller's own unwrapped ctx.Err(); signal
// only adds identifying context, preserving errors.Is for both underneath.
func (control *acpAttemptControl) signal(ctx context.Context) (bool, error) {
	accepted, err := control.acp.TryCancel(ctx, control.generation)
	if err != nil {
		return false, fmt.Errorf("deliver cancel to provider %q attempt %q: %w", control.canonical, control.attemptID, err)
	}
	return accepted, nil
}
