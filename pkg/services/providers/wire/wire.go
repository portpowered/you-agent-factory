// Package wire is the Providers service composition boundary.
//
// Wire performs construction only, returns the singular providers.Service root
// interface, and starts no lifecycle components. Parent-private Catalog and
// Execution owner wiring stays inside the owner service assembly path; peers
// depend on Service rather than owner internals or construction ports. The
// process-edge registration contract in this package is for root composition,
// not a second peer-facing Providers service. Missing required construction
// ports fail with a deterministic construction error and a nil service.
package wire

import (
	"context"
	"fmt"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/catalogdata"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acpwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// CommandRunner is the Providers-owned subprocess effect accepted at the
// composition boundary. Workers-specific runners are projected into this
// contract by pkg/wire.
type CommandRunner = providerservice.CommandRunner
type CommandRequest = providerservice.CommandRequest
type CommandResult = providerservice.CommandResult
type OutputChunkObserver = providerservice.OutputChunkObserver
type PTYAllocator = providerservice.PTYAllocator
type PTYSession = providerservice.PTYSession
type PTYSessionConfig = providerservice.PTYSessionConfig
type PTYProcessLaunch = providerservice.PTYProcessLaunch
type PTYSessionResult = providerservice.PTYSessionResult

const (
	DefaultPTYMaxCaptureBytes = providerservice.DefaultPTYMaxCaptureBytes
	MaxPTYMaxCaptureBytes     = providerservice.MaxPTYMaxCaptureBytes
	DefaultPTYIdleTimeout     = providerservice.DefaultPTYIdleTimeout
	DefaultPTYHardTimeout     = providerservice.DefaultPTYHardTimeout
)

// DefaultPTYSessionConfig returns the bounded native-session defaults used by
// the Providers Agy adapter.
func DefaultPTYSessionConfig() PTYSessionConfig {
	return providerservice.DefaultPTYSessionConfig()
}

const (
	OutputStreamStdout = providerservice.OutputStreamStdout
	OutputStreamStderr = providerservice.OutputStreamStderr
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source, scheduler platformclock.TimerSource) (PTYAllocator, error) {
	return executionwire.NewAgyPTYAllocator(host, clock, scheduler)
}

// AgyEffect exposes the completed AGY execution collaborator to composition.
type AgyEffect = executionwire.AgyEffect

// AgyPTYPolicy contains detached native-session policy, without host effects.
type AgyPTYPolicy = executionwire.AgyPTYPolicy

// NewAgyPTYEffect constructs one native effect over individually supplied ports.
func NewAgyPTYEffect(allocator PTYAllocator, locator platformprocess.ExecutableLocator, inspector platformfilesystem.PathInspector, clock platformclock.Source, policy AgyPTYPolicy) AgyEffect {
	return executionwire.NewAgyPTYEffect(allocator, locator, inspector, clock, policy)
}

// CatalogCapabilityOverride supplies an authoritative capability view for one
// already-registered provider route during process construction. It is used by
// hosts and functional tests whose selected route has narrower capabilities
// than its static publication; it cannot register a new provider identity.
type CatalogCapabilityOverride struct {
	Provider     providers.ID
	Capabilities []providers.Capability
}

// Clone returns detached override values for the construction boundary.
func (override CatalogCapabilityOverride) Clone() CatalogCapabilityOverride {
	return CatalogCapabilityOverride{
		Provider:     override.Provider,
		Capabilities: append([]providers.Capability(nil), override.Capabilities...),
	}
}

// Option configures Providers root construction.
type Option interface {
	apply(*wireOptions)
}

type wireOptions struct {
	catalogDescriptors []providers.Descriptor
	catalogOverrides   []catalog.CapabilityOverride
	commandRunner      providerservice.CommandRunner
	agyCommandRunner   providerservice.CommandRunner
	agyCommandClock    platformclock.Source
	agyPTYEffect       AgyEffect
	acpIntegrations    []providers.ACPIntegration
	commandFactory     platformprocess.CommandFactory
	executableLocator  platformprocess.ExecutableLocator
	stdioPipes         platformprocess.StdioPipeFactory
	registrations      ProviderRegistrations
	logger             logging.Logger
}

type registrationsOption struct {
	registrations ProviderRegistrations
}

func (option registrationsOption) apply(config *wireOptions) {
	config.registrations = append(ProviderRegistrations(nil), option.registrations...)
}

// WithRegistrations contributes process-edge compatibility integrations.
// Provider execution still crosses the singular providers.Service boundary.
func WithRegistrations(registrations ...Registration) Option {
	return registrationsOption{registrations: registrations}
}

type executableLocatorOption struct {
	locator platformprocess.ExecutableLocator
}

func (o executableLocatorOption) apply(opts *wireOptions) { opts.executableLocator = o.locator }

// WithExecutableLocator injects ACP executable preflight discovery.
func WithExecutableLocator(locator platformprocess.ExecutableLocator) Option {
	return executableLocatorOption{locator: locator}
}

type acpIntegrationsOption struct{ integrations []providers.ACPIntegration }

func (o acpIntegrationsOption) apply(opts *wireOptions) {
	opts.acpIntegrations = append([]providers.ACPIntegration(nil), o.integrations...)
}

// WithACPIntegrations contributes configured ACP identities and commands.
func WithACPIntegrations(integrations ...providers.ACPIntegration) Option {
	return acpIntegrationsOption{integrations: integrations}
}

type commandFactoryOption struct {
	factory platformprocess.CommandFactory
}

func (o commandFactoryOption) apply(opts *wireOptions) { opts.commandFactory = o.factory }

// WithCommandFactory injects the only process-creation edge used by ACP.
func WithCommandFactory(factory platformprocess.CommandFactory) Option {
	return commandFactoryOption{factory: factory}
}

type stdioPipesOption struct {
	factory platformprocess.StdioPipeFactory
}

func (o stdioPipesOption) apply(opts *wireOptions) { opts.stdioPipes = o.factory }

// WithStdioPipeFactory injects the parent-owned ACP standard-stream channel
// factory. Canonical composition selects it here; this package never defaults
// it, so an ACP execution that reached Providers without one reports a missing
// channel dependency instead of opening a host pipe from inside the service.
func WithStdioPipeFactory(factory platformprocess.StdioPipeFactory) Option {
	return stdioPipesOption{factory: factory}
}

// CatalogProbeOperation is the completed catalog readiness projection.
type CatalogProbeOperation = catalog.ProbeOperation

// IdentityCatalogProbe preserves detached catalog facts without readiness I/O.
func IdentityCatalogProbe(ctx context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
	return catalogwire.IdentityProbe(ctx, descriptor)
}

type catalogDescriptorsOption struct{ descriptors []providers.Descriptor }

func (o catalogDescriptorsOption) apply(opts *wireOptions) {
	opts.catalogDescriptors = append(opts.catalogDescriptors, o.descriptors...)
}

// WithCatalogDescriptors contributes detached catalog facts.
func WithCatalogDescriptors(descriptors ...providers.Descriptor) Option {
	cloned := make([]providers.Descriptor, len(descriptors))
	for index, descriptor := range descriptors {
		cloned[index] = descriptor.Clone()
	}
	return catalogDescriptorsOption{descriptors: cloned}
}

type catalogCapabilityOverridesOption struct {
	overrides []CatalogCapabilityOverride
}

func (option catalogCapabilityOverridesOption) apply(config *wireOptions) {
	overrides := make([]catalog.CapabilityOverride, 0, len(option.overrides))
	for _, override := range option.overrides {
		overrides = append(overrides, catalog.CapabilityOverride{
			Provider:     override.Provider,
			Capabilities: append([]providers.Capability(nil), override.Capabilities...),
		})
	}
	config.catalogOverrides = append(config.catalogOverrides, overrides...)
}

// WithCatalogCapabilityOverrides supplies route-specific static capability
// facts without adding or replacing a provider registration.
func WithCatalogCapabilityOverrides(overrides ...CatalogCapabilityOverride) Option {
	cloned := make([]CatalogCapabilityOverride, len(overrides))
	for index, override := range overrides {
		cloned[index] = override.Clone()
	}
	return catalogCapabilityOverridesOption{overrides: cloned}
}

type commandRunnerOption struct {
	runner platformprocess.CommandRunner
}

func (o commandRunnerOption) apply(opts *wireOptions) {
	opts.commandRunner = executionwire.AdaptPlatformCommandRunner(o.runner)
}

// WithCommandRunner injects the shared streaming subprocess runner used by
// built-in Codex and Claude command effects.
func WithCommandRunner(runner platformprocess.CommandRunner) Option {
	return commandRunnerOption{runner: runner}
}

type commandEffectRunnerOption struct {
	runner providerservice.CommandRunner
}

func (o commandEffectRunnerOption) apply(opts *wireOptions) {
	opts.commandRunner = o.runner
}

type agyCommandRunnerOption struct {
	runner any
}

func (o agyCommandRunnerOption) apply(opts *wireOptions) {
	opts.agyCommandRunner = providerservice.AdaptCommandRunner(o.runner)
}

// WithAgyCommandRunner injects the Providers command-runner effect used by
// canonical AGY print-mode execution. The PTY option remains available for
// direct compatibility tests and hosts that intentionally select that seam.
func WithAgyCommandRunner(runner any) Option {
	return agyCommandRunnerOption{runner: runner}
}

type agyCommandClockOption struct {
	clock platformclock.Source
}

func (o agyCommandClockOption) apply(opts *wireOptions) { opts.agyCommandClock = o.clock }

// WithAgyCommandClock injects the timing source used by AGY command
// diagnostics and duration facts.
func WithAgyCommandClock(clock platformclock.Source) Option {
	return agyCommandClockOption{clock: clock}
}

type agyPTYEffectOption struct {
	effect AgyEffect
}

func (o agyPTYEffectOption) apply(opts *wireOptions) {
	opts.agyPTYEffect = o.effect
}

// WithAgyPTYEffect supplies the completed legacy PTY effect. An explicit
// command runner retains priority over this effect.
func WithAgyPTYEffect(effect AgyEffect) Option {
	return agyPTYEffectOption{effect: effect}
}

// WithWorkersCommandRunner is retained as a source-compatible migration
// option. Its value is projected immediately into the Providers command
// effect and is never stored as a Workers contract.
func WithWorkersCommandRunner(runner any) Option {
	return commandEffectRunnerOption{runner: providerservice.AdaptCommandRunner(runner)}
}

type loggerOption struct {
	logger logging.Logger
}

func (o loggerOption) apply(opts *wireOptions) { opts.logger = o.logger }

// WithLogger injects the safe structured logger the constructed root uses for
// accepted-intent and terminal-outcome operation records, including
// ControlAttempt. A nil or omitted logger falls back to logging.NoopLogger.
func WithLogger(logger logging.Logger) Option {
	return loggerOption{logger: logger}
}

// NewService constructs one inert Providers root over sibling Catalog and
// Execution capabilities sharing the same private catalog identity authority.
// The caller supplies the completed readiness projection; this boundary never
// substitutes an identity projection for a missing effect.
func NewService(probe CatalogProbeOperation, options ...Option) (providers.Service, error) {
	var config wireOptions
	for _, option := range options {
		if option != nil {
			option.apply(&config)
		}
	}
	packaged, err := PackagedACPIntegrations()
	if err != nil {
		return nil, err
	}
	acp := effectiveACPIntegrations(packaged, config.acpIntegrations)
	descriptors, err := packagedACPDescriptors(acp)
	if err != nil {
		return nil, err
	}
	for _, registration := range config.registrations {
		descriptors = append(descriptors, registrationDescriptor(registration.Manifest))
	}
	catalogService, err := catalogwire.NewService(probe, append(config.catalogDescriptors, descriptors...), config.catalogOverrides)
	if err != nil {
		return nil, err
	}
	return newRootWithOptions(
		catalogService,
		config.commandRunner,
		config.agyCommandRunner,
		config.agyCommandClock,
		config.agyPTYEffect,
		acp,
		config.commandFactory,
		config.executableLocator,
		config.stdioPipes,
		config.logger,
		config.registrations...,
	)
}

func packagedACPDescriptors(integrations []providers.ACPIntegration) ([]providers.Descriptor, error) {
	catalog, err := modelproviders.Catalog()
	if err != nil {
		return nil, fmt.Errorf("load packaged provider catalog for ACP descriptors: %w", err)
	}
	published := make(map[string]struct{}, len(catalog.Providers))
	for _, manifest := range catalog.Providers {
		published[manifest.Id] = struct{}{}
	}
	descriptors := make([]providers.Descriptor, 0, len(integrations))
	for _, integration := range integrations {
		if _, exists := published[integration.Name.String()]; exists {
			continue
		}
		descriptors = append(descriptors, acpDescriptor(integration))
	}
	return descriptors, nil
}

// PackagedACPIntegrations returns the detached data-backed ACP defaults used by
// Providers. Composition uses this exact source when materializing a new
// operator configuration so init and runtime discovery cannot drift.
func PackagedACPIntegrations() ([]providers.ACPIntegration, error) {
	return ACPIntegrationsFromRuntimeCatalog(modelproviders.RuntimeACPJSON())
}

// ACPIntegrationsFromRuntimeCatalog projects a generated package-owned
// runtime catalog into detached Providers integrations. The production
// catalog uses RuntimeACPJSON; the parameter keeps the composition boundary
// able to validate and diagnose alternate generated documents without starting
// any provider process.
func ACPIntegrationsFromRuntimeCatalog(document []byte) ([]providers.ACPIntegration, error) {
	return catalogdata.DecodeACPIntegrations(document)
}

func newRootWithOptions(
	catalogService catalog.Service,
	commandRunner providerservice.CommandRunner,
	agyCommandRunner providerservice.CommandRunner,
	agyCommandClock platformclock.Source,
	agyPTYEffect AgyEffect,
	acpIntegrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	executableLocator platformprocess.ExecutableLocator,
	stdioPipes platformprocess.StdioPipeFactory,
	logger logging.Logger,
	externalRegistrations ...Registration,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	registrations := executionserviceRegistrations(commandRunner, agyCommandRunner, agyCommandClock, agyPTYEffect)
	scheduler, ok := agyCommandClock.(platformclock.TimerSource)
	if !ok {
		scheduler = platformclock.Real{}
	}
	logger = logging.EnsureLogger(logger)
	acpService, err := acpwire.NewService(acpIntegrations, commandFactory, executableLocator, stdioPipes, scheduler, logger)
	if err != nil {
		return nil, err
	}
	for _, integration := range acpIntegrations {
		registrations = append(registrations, executionwire.NewACPRegistration(integration.Name, acpService))
	}
	for _, registration := range externalRegistrations {
		for _, existing := range registrations {
			if existing.Provider == providers.ID(registration.Manifest.ID) {
				return nil, fmt.Errorf("provider registry validation failed for %q: identity collision", registration.Manifest.ID)
			}
		}
		if err := validateExternalRegistrationCapabilities(registration); err != nil {
			return nil, err
		}
		attempt, err := externalRegistrationAttempt(registration)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, attempt)
	}
	executionService, err := executionwire.NewService(
		catalogService,
		registrations...,
	)
	if err != nil {
		return nil, err
	}
	return providerservice.NewWithACP(
		catalogService,
		executionService,
		acpService,
		acpIntegrations,
		logger,
		acpService,
	)
}

func validateExternalRegistrationCapabilities(registration Registration) error {
	if registration.Integration == nil {
		return nil
	}
	manifestSupportsBypass := registration.Manifest.MaximumExecutionCapabilities.PermissionBypass
	integrationSupportsBypass := registration.Integration.MaximumCapabilities().Has(CapabilityPermissionBypass)
	if manifestSupportsBypass == integrationSupportsBypass {
		return nil
	}
	return fmt.Errorf(
		"provider registry validation failed for %q: integration maximum capability %q contradicts manifest maximum execution capability permissionBypass",
		registration.Manifest.ID,
		CapabilityPermissionBypass,
	)
}

func registrationDescriptor(manifest Manifest) providers.Descriptor {
	capabilities := []providers.Capability{}
	if manifest.MaximumExecutionCapabilities.PromptSubmission {
		capabilities = append(capabilities, providers.CapabilityPromptSubmission)
	}
	if manifest.MaximumExecutionCapabilities.ImageInput {
		capabilities = append(capabilities, providers.CapabilityImageInput)
	}
	if manifest.MaximumExecutionCapabilities.SessionResume {
		capabilities = append(capabilities, providers.CapabilitySessionResume)
	}
	if manifest.MaximumExecutionCapabilities.StructuredOutput {
		capabilities = append(capabilities, providers.CapabilityStructuredOutput)
	}
	if manifest.MaximumExecutionCapabilities.PermissionBypass {
		capabilities = append(capabilities, providers.CapabilityPermissionBypass)
	}
	return providers.Descriptor{
		ID: providers.ID(manifest.ID), Aliases: append([]string(nil), manifest.Aliases...),
		DisplayName: manifest.DisplayName.Value, Availability: providers.AvailabilitySelectable,
		Readiness: providers.ReadinessReady, Capabilities: capabilities,
	}
}

func externalRegistrationAttempt(registration Registration) (execution.Registration, error) {
	if registration.Integration == nil {
		return execution.Registration{}, fmt.Errorf("provider registry validation failed for %q: integration is required", registration.Manifest.ID)
	}
	if got := string(registration.Integration.Identity()); got != registration.Manifest.ID {
		return execution.Registration{}, fmt.Errorf("provider registry validation failed for %q: integration identity %q does not match manifest", registration.Manifest.ID, got)
	}
	return execution.Registration{
		Provider: providers.ID(registration.Manifest.ID),
		Attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			writer := &externalResponseWriter{}
			invocation := InvocationRequest{
				ID: request.AttemptID, ModelID: request.Model,
				ReasoningEffort: request.ReasoningEffort,
				SkipPermissions: request.SkipPermissions, Prompt: request.UserMessage,
			}
			if err := validateExternalInvocationCapabilities(ctx, registration.Manifest.ID, registration.Integration, invocation); err != nil {
				return providers.ExecuteResult{}, err
			}
			err := registration.Integration.Invoke(ctx, invocation, writer)
			if err != nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: err.Error()}
			}
			if writer.completion == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: "external provider completed without a terminal result"}
			}
			if writer.completion.Err != nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: writer.completion.Err.Error()}
			}
			if writer.completion.Response == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown, Message: "external provider completed without a response"}
			}
			result := providers.ExecuteResult{
				Content: writer.completion.Response.Content,
				Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
					"completion_evidence": "provider_response",
				}},
			}
			if writer.progress > 0 {
				result.Diagnostics.Progress = []providers.ExecuteProgress{{Phase: "updated", Detail: "external provider progress"}}
			}
			return result, nil
		},
	}, nil
}

type externalResponseWriter struct {
	completion *Completion
	progress   int
}

func (writer *externalResponseWriter) WriteEvent(context.Context, EventDraft) error {
	writer.progress++
	return nil
}

func (writer *externalResponseWriter) Close(_ context.Context, completion Completion) error {
	clone := completion
	writer.completion = &clone
	return nil
}

func executionserviceRegistrations(
	commandRunner providerservice.CommandRunner,
	agyCommandRunner providerservice.CommandRunner,
	agyCommandClock platformclock.Source,
	agyPTYEffect AgyEffect,
) []execution.Registration {
	if agyCommandClock == nil {
		agyCommandClock = platformclock.Real{}
	}
	scheduler, ok := agyCommandClock.(platformclock.TimerSource)
	if !ok {
		scheduler = platformclock.Real{}
	}
	antigravity := agyPTYEffect
	if agyCommandRunner != nil {
		antigravity = executionwire.NewAgyCommandEffect(agyCommandRunner, agyCommandClock, scheduler)
	}
	return executionwire.BuiltInRegistrations(
		antigravity,
		executionwire.NewCodexEffect(commandRunner, platformclock.Real{}),
		executionwire.NewClaudeEffect(commandRunner, platformclock.Real{}),
	)
}

func effectiveACPIntegrations(packaged, configured []providers.ACPIntegration) []providers.ACPIntegration {
	values := make([]providers.ACPIntegration, len(packaged))
	for index, value := range packaged {
		values[index] = value.Clone()
	}
	for _, value := range configured {
		found := false
		for i := range values {
			if values[i].Name == value.Name {
				replacement := value.Clone()
				if replacement.Command == values[i].Command {
					// Persisted operator settings predate the generated runtime
					// projection and only carry the legacy command shape. Preserve
					// package-owned runtime facts when the saved command is still
					// the reviewed package command.
					if replacement.Aliases == nil {
						replacement.Aliases = append([]string(nil), values[i].Aliases...)
					}
					if replacement.Arguments == nil {
						replacement.Arguments = append([]string(nil), values[i].Arguments...)
					}
					if replacement.RuntimePosture == "" {
						replacement.RuntimePosture = values[i].RuntimePosture
					}
					if replacement.ImplementationProfile == "" {
						replacement.ImplementationProfile = values[i].ImplementationProfile
					}
				}
				values[i] = replacement
				found = true
				break
			}
		}
		if !found {
			values = append(values, value.Clone())
		}
	}
	return values
}

func acpDescriptor(integration providers.ACPIntegration) providers.Descriptor {
	return providers.Descriptor{ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(), Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessUnverified, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilitySessionResume}}
}
