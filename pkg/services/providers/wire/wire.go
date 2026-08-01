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
)

// CommandRunner is the private Providers execution effect accepted by root
// composition. It is deliberately not a Workers command contract.
type CommandRunner = providerservice.CommandRunner
type CommandRequest = providerservice.CommandRequest
type CommandResult = providerservice.CommandResult
type OutputChunkObserver = providerservice.OutputChunkObserver

const (
	OutputStreamStdout = providerservice.OutputStreamStdout
	OutputStreamStderr = providerservice.OutputStreamStderr
)

// PTYAllocator is the private Providers Agy effect contract. Workers receives
// a separate adapter at the application composition boundary.
type PTYAllocator = providerservice.PTYAllocator
type PTYSession = providerservice.PTYSession
type PTYSessionConfig = providerservice.PTYSessionConfig
type PTYProcessLaunch = providerservice.PTYProcessLaunch
type PTYSessionResult = providerservice.PTYSessionResult

// PTY errors are exposed only at this construction boundary so the
// composition adapter can preserve Workers' established error identity while
// keeping the implementation-owned effect sentinels private to Providers.
var (
	ErrPTYUnsupportedPlatform = providerservice.ErrPTYUnsupportedPlatform
	ErrPTYAllocationFailed    = providerservice.ErrPTYAllocationFailed
	ErrPTYSessionTimedOut     = providerservice.ErrPTYSessionTimedOut
	ErrPTYNonzeroExit         = providerservice.ErrPTYNonzeroExit
	ErrPTYClockRequired       = providerservice.ErrPTYClockRequired
	ErrPTYHostRequired        = providerservice.ErrPTYHostRequired
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source) (PTYAllocator, error) {
	return executionwire.NewAgyPTYAllocator(host, clock)
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator PTYAllocator
	Clock     platformclock.Source
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// Option configures Providers root construction.
type Option interface {
	apply(*wireOptions)
}

type wireOptions struct {
	catalog           []catalogwire.Option
	commandRunner     CommandRunner
	agyPTYPlatform    AgyPTYPlatformDependencies
	acpIntegrations   []providers.ACPIntegration
	commandFactory    platformprocess.CommandFactory
	executableLocator platformprocess.ExecutableLocator
	registrations     ProviderRegistrations
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

type commandRunnerOption struct {
	runner CommandRunner
}

func (o commandRunnerOption) apply(opts *wireOptions) {
	opts.commandRunner = o.runner
}

// WithCommandRunner injects the platform subprocess runner used by built-in
// Codex and Claude command execution. The platform runner is adapted into the
// Providers-owned private effect contract inside this package's wire path.
func WithCommandRunner(runner platformprocess.CommandRunner) Option {
	return commandRunnerOption{runner: executionwire.AdaptPlatformCommandRunner(runner)}
}

type commandEffectRunnerOption struct {
	runner CommandRunner
}

func (o commandEffectRunnerOption) apply(opts *wireOptions) {
	opts.commandRunner = o.runner
}

// WithCommandEffectRunner injects a Providers-owned effect runner from the
// application composition boundary. Workers-specific projection belongs in
// pkg/wire, not in Providers.
func WithCommandEffectRunner(runner CommandRunner) Option {
	return commandEffectRunnerOption{runner: runner}
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
	return newRoot(
		catalogService,
		config.commandRunner,
		config.agyPTYPlatform,
		acp,
		config.commandFactory,
		config.executableLocator,
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
	commandRunner CommandRunner,
	agyPTYPlatform AgyPTYPlatformDependencies,
	acpIntegrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	executableLocator platformprocess.ExecutableLocator,
	externalRegistrations ...Registration,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	registrations := executionserviceRegistrations(commandRunner, agyPTYPlatform)
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
	return providerservice.NewWithACP(catalogService, executionService, acpService, acpIntegrations, acpService)
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
				ReasoningEffort: request.ReasoningEffort, Prompt: request.UserMessage,
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
			result := providers.ExecuteResult{Content: writer.completion.Response.Content}
			if writer.progress > 0 {
				result.Diagnostics = &providers.ExecuteDiagnostics{Progress: []providers.ExecuteProgress{{Phase: "updated", Detail: "external provider progress"}}}
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

func executionserviceRegistrations(commandRunner CommandRunner, agyPTYPlatform AgyPTYPlatformDependencies) []execution.Registration {
	return executionwire.BuiltInRegistrations(executionwire.BuiltInDependenciesFromRunner(
		commandRunner,
		executionwire.BuiltInRunnerPlatformDependencies{
			Clock: agyPTYPlatform.Clock,
			AgyPTY: executionwire.AgyPTYPlatformDependencies{
				Allocator: agyPTYPlatform.Allocator,
				Clock:     agyPTYPlatform.Clock,
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
	return providers.Descriptor{ID: integration.Name, Aliases: append([]string(nil), integration.Aliases...), DisplayName: integration.Name.String(), Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityReasoningSummaries, providers.CapabilityToolLifecycle, providers.CapabilityFileChanges, providers.CapabilityPlans, providers.CapabilityUsage}}
}

// NewFactory returns an inert constructor used for operator-configured ACP catalogs.
func NewFactory(commandFactory platformprocess.CommandFactory, options ...Option) providers.Factory {
	return func(integrations []providers.ACPIntegration) (providers.Service, error) {
		return NewService(append(options, WithCommandFactory(commandFactory), WithACPIntegrations(integrations...))...)
	}
}
