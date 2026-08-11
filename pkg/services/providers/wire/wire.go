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

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acpwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp/wire"
	builtinswire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins/wire"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation behind
// the Workers root allocation port.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source) (workers.PTYAllocator, error) {
	return executionwire.NewAgyPTYAllocator(host, clock)
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator workers.PTYAllocator
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
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
	catalog              []catalogwire.Option
	commandRunner        platformprocess.CommandRunner
	workersCommandRunner workers.CommandRunner
	agyCommandRunner     workers.CommandRunner
	agyCommandClock      workers.Clock
	agyPTYPlatform       AgyPTYPlatformDependencies
	acpIntegrations      []providers.ACPIntegration
	commandFactory       platformprocess.CommandFactory
	executableLocator    platformprocess.ExecutableLocator
	registrations        ProviderRegistrations
	logger               logging.Logger
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

type catalogOption struct {
	value catalogwire.Option
}

func (o catalogOption) apply(opts *wireOptions) {
	opts.catalog = append(opts.catalog, o.value)
}

// CatalogOption adapts a catalog subservice option for root construction.
func CatalogOption(option catalogwire.Option) Option {
	return catalogOption{value: option}
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
	config.catalog = append(config.catalog, catalogwire.WithCapabilityOverrides(overrides...))
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
	opts.commandRunner = o.runner
}

// WithCommandRunner injects the shared streaming subprocess runner used by
// built-in Codex and Claude command effects.
func WithCommandRunner(runner platformprocess.CommandRunner) Option {
	return commandRunnerOption{runner: runner}
}

type workersCommandRunnerOption struct {
	runner workers.CommandRunner
}

func (o workersCommandRunnerOption) apply(opts *wireOptions) {
	opts.workersCommandRunner = o.runner
}

// WithWorkersCommandRunner injects the shared Workers subprocess runner used
// by built-in Codex and Claude command effects without losing dispatch
// correlation required by mock-worker interception.
func WithWorkersCommandRunner(runner workers.CommandRunner) Option {
	return workersCommandRunnerOption{runner: runner}
}

type agyCommandRunnerOption struct {
	runner workers.CommandRunner
}

func (o agyCommandRunnerOption) apply(opts *wireOptions) { opts.agyCommandRunner = o.runner }

// WithAgyCommandRunner injects the Providers command-runner effect used by
// canonical AGY print-mode execution. The PTY option remains available for
// direct compatibility tests and hosts that intentionally select that seam.
func WithAgyCommandRunner(runner workers.CommandRunner) Option {
	return agyCommandRunnerOption{runner: runner}
}

type agyCommandClockOption struct {
	clock workers.Clock
}

func (o agyCommandClockOption) apply(opts *wireOptions) { opts.agyCommandClock = o.clock }

// WithAgyCommandClock injects the timing source used by AGY command
// diagnostics and duration facts.
func WithAgyCommandClock(clock workers.Clock) Option {
	return agyCommandClockOption{clock: clock}
}

type agyPTYPlatformOption struct {
	platform AgyPTYPlatformDependencies
}

func (o agyPTYPlatformOption) apply(opts *wireOptions) {
	opts.agyPTYPlatform = o.platform
}

// WithAgyPTY injects the platform facts required for the built-in Agy PTY
// execution adapter.
func WithAgyPTY(platform AgyPTYPlatformDependencies) Option {
	return agyPTYPlatformOption{platform: platform}
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
// Missing required composition inputs fail with a deterministic construction
// error and a nil service.
func NewService(options ...Option) (providers.Service, error) {
	var config wireOptions
	for _, option := range options {
		if option != nil {
			option.apply(&config)
		}
	}
	packaged, err := builtinswire.NewService()
	if err != nil {
		return nil, err
	}
	acp := effectiveACPIntegrations(packaged.ACPIntegrations(), config.acpIntegrations)
	descriptors := make([]providers.Descriptor, 0, len(acp))
	for _, integration := range acp {
		descriptors = append(descriptors, acpDescriptor(integration))
	}
	for _, registration := range config.registrations {
		descriptors = append(descriptors, registrationDescriptor(registration.Manifest))
	}
	config.catalog = append(config.catalog, catalogwire.WithDescriptors(descriptors...))
	catalogService, err := catalogwire.NewService(config.catalog...)
	if err != nil {
		return nil, err
	}
	if config.agyCommandClock == nil {
		config.agyCommandClock = platformclock.Real{}
	}
	return newRoot(
		catalogService,
		config.commandRunner,
		config.workersCommandRunner,
		config.agyCommandRunner,
		config.agyCommandClock,
		config.agyPTYPlatform,
		acp,
		config.commandFactory,
		config.executableLocator,
		config.logger,
		config.registrations...,
	)
}

// PackagedACPIntegrations returns the detached data-backed ACP defaults used by
// Providers. Composition uses this exact source when materializing a new
// operator configuration so init and runtime discovery cannot drift.
func PackagedACPIntegrations() ([]providers.ACPIntegration, error) {
	packaged, err := builtinswire.NewService()
	if err != nil {
		return nil, err
	}
	return packaged.ACPIntegrations(), nil
}

func newRoot(
	catalogService catalog.Service,
	commandRunner platformprocess.CommandRunner,
	workersCommandRunner workers.CommandRunner,
	agyCommandRunner workers.CommandRunner,
	agyCommandClock workers.Clock,
	agyPTYPlatform AgyPTYPlatformDependencies,
	acpIntegrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	executableLocator platformprocess.ExecutableLocator,
	logger logging.Logger,
	externalRegistrations ...Registration,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	if workersCommandRunner == nil && commandRunner != nil {
		workersCommandRunner = workers.AdaptCommandRunner(commandRunner)
	}
	registrations := executionserviceRegistrations(workersCommandRunner, agyCommandRunner, agyCommandClock, agyPTYPlatform)
	acpService, err := acpwire.NewService(acpIntegrations, commandFactory, executableLocator)
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
			err := registration.Integration.Invoke(ctx, InvocationRequest{
				ID: request.AttemptID, ModelID: request.Model,
				ReasoningEffort: request.ReasoningEffort,
				SkipPermissions: request.SkipPermissions, Prompt: request.UserMessage,
			}, writer)
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
					workers.ProviderResponseMetadataCompletionEvidence: "provider_response",
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
	workersCommandRunner workers.CommandRunner,
	agyCommandRunner workers.CommandRunner,
	agyCommandClock workers.Clock,
	agyPTYPlatform AgyPTYPlatformDependencies,
) []execution.Registration {
	return executionwire.BuiltInRegistrations(executionwire.BuiltInDependenciesFromWorkersRunner(
		workersCommandRunner,
		executionwire.BuiltInRunnerPlatformDependencies{
			AgyCommandRunner: agyCommandRunner,
			AgyCommandClock:  agyCommandClock,
			AgyPTY: executionwire.AgyPTYPlatformDependencies{
				Allocator: agyPTYPlatform.Allocator,
				Locator:   agyPTYPlatform.Locator,
				Inspector: agyPTYPlatform.Inspector,
			},
		},
	))
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
				values[i] = value.Clone()
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
	return providers.Descriptor{ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(), Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityPermissionBypass, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityReasoningSummaries, providers.CapabilityToolLifecycle, providers.CapabilityFileChanges, providers.CapabilityPlans, providers.CapabilityUsage}}
}

// NewFactory returns an inert constructor used for operator-configured ACP catalogs.
func NewFactory(commandFactory platformprocess.CommandFactory, options ...Option) providers.Factory {
	return func(integrations []providers.ACPIntegration) (providers.Service, error) {
		return NewService(append(options, WithCommandFactory(commandFactory), WithACPIntegrations(integrations...))...)
	}
}
