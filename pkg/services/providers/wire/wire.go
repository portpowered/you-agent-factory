// Package wire is the Providers service composition boundary.
//
// Wire performs construction only, returns the singular providers.Service root
// interface, and starts no lifecycle components. Parent-private Catalog and
// Execution owner wiring stays inside the owner service assembly path; peers
// depend on Service rather than owner internals or construction ports. Missing
// required construction ports fail with a deterministic construction error and
// a nil service.
package wire

import (
	"fmt"
	"github.com/mattn/go-shellwords"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// CursorPlatformDependencies are platform facts required for oversized Windows
// Cursor prompt materialization in the built-in Providers Execution adapter.
type CursorPlatformDependencies struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator agypty.PTYAllocator
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// Option configures Providers root construction.
type Option interface {
	apply(*wireOptions)
}

type wireOptions struct {
	catalog              []catalogwire.Option
	commandRunner        platformprocess.CommandRunner
	workersCommandRunner workers.CommandRunner
	cursorPlatform       CursorPlatformDependencies
	agyPTYPlatform       AgyPTYPlatformDependencies
	acpIntegrations      []providers.ACPIntegration
	commandFactory       platformprocess.CommandFactory
	executableLocator    platformprocess.ExecutableLocator
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

type cursorPlatformOption struct {
	platform CursorPlatformDependencies
}

func (o cursorPlatformOption) apply(opts *wireOptions) {
	opts.cursorPlatform = o.platform
}

// WithCursorPlatform injects the platform facts required for oversized Windows
// Cursor prompt materialization in the built-in Providers Execution adapter.
func WithCursorPlatform(platform CursorPlatformDependencies) Option {
	return cursorPlatformOption{platform: platform}
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
	acp := effectiveACPIntegrations(config.acpIntegrations)
	descriptors := make([]providers.Descriptor, 0, len(acp))
	for _, integration := range acp {
		descriptors = append(descriptors, acpDescriptor(integration))
	}
	config.catalog = append(config.catalog, catalogwire.WithDescriptors(descriptors...))
	catalogService, err := catalogwire.NewService(config.catalog...)
	if err != nil {
		return nil, err
	}
	return newRoot(
		catalogService,
		config.commandRunner,
		config.workersCommandRunner,
		config.cursorPlatform,
		config.agyPTYPlatform,
		acp,
		config.commandFactory,
		config.executableLocator,
	)
}

func newRoot(
	catalogService catalog.Service,
	commandRunner platformprocess.CommandRunner,
	workersCommandRunner workers.CommandRunner,
	cursorPlatform CursorPlatformDependencies,
	agyPTYPlatform AgyPTYPlatformDependencies,
	acpIntegrations []providers.ACPIntegration,
	commandFactory platformprocess.CommandFactory,
	executableLocator platformprocess.ExecutableLocator,
) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	if workersCommandRunner == nil && commandRunner != nil {
		workersCommandRunner = workerprocess.AdaptCommandRunner(commandRunner)
	}
	registrations := executionserviceRegistrations(workersCommandRunner, cursorPlatform, agyPTYPlatform)
	for _, integration := range acpIntegrations {
		parts, err := shellwords.Parse(integration.Command)
		if err != nil || len(parts) == 0 {
			return nil, fmt.Errorf("construct ACP provider %q: invalid command", integration.Name)
		}
		registrations = append(registrations, executionwire.NewACPRegistration(integration.Name, parts[0], parts[1:], commandFactory, executableLocator))
	}
	executionService, err := executionwire.NewService(
		catalogService,
		registrations...,
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService, executionService)
}

func executionserviceRegistrations(workersCommandRunner workers.CommandRunner, cursorPlatform CursorPlatformDependencies, agyPTYPlatform AgyPTYPlatformDependencies) []execution.Registration {
	return executionwire.BuiltInRegistrations(executionwire.BuiltInDependenciesFromWorkersRunner(
		workersCommandRunner,
		executionwire.BuiltInRunnerPlatformDependencies{
			Cursor: executionwire.CursorPlatformDependencies{
				OperatingSystem: cursorPlatform.OperatingSystem,
				TemporaryDir:    cursorPlatform.TemporaryDir,
				TemporaryFiles:  cursorPlatform.TemporaryFiles,
			},
			AgyPTY: executionwire.AgyPTYPlatformDependencies{
				Allocator: agyPTYPlatform.Allocator,
				Locator:   agyPTYPlatform.Locator,
				Inspector: agyPTYPlatform.Inspector,
			},
		},
	))
}

func effectiveACPIntegrations(configured []providers.ACPIntegration) []providers.ACPIntegration {
	values := []providers.ACPIntegration{
		{ID: "cursor-acp", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp"},
		{ID: "kiro-acp", Name: "kiro-acp", Transport: "stdio", Command: "kiro-cli acp"},
		{ID: "opencode-acp", Name: "opencode-acp", Transport: "stdio", Command: "opencode acp"},
	}
	for _, value := range configured {
		found := false
		for i := range values {
			if values[i].Name == value.Name {
				values[i] = value
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

func acpDescriptor(integration providers.ACPIntegration) providers.Descriptor {
	return providers.Descriptor{ID: integration.Name, DisplayName: integration.Name.String(), Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission, providers.CapabilityImageInput, providers.CapabilitySessionResume, providers.CapabilityNativeStreaming, providers.CapabilityMessageDeltas, providers.CapabilityReasoningSummaries, providers.CapabilityToolLifecycle, providers.CapabilityFileChanges, providers.CapabilityPlans, providers.CapabilityUsage}}
}

// NewFactory returns an inert constructor used for operator-configured ACP catalogs.
func NewFactory(commandFactory platformprocess.CommandFactory, options ...Option) providers.Factory {
	return func(integrations []providers.ACPIntegration) (providers.Service, error) {
		return NewService(append(options, WithCommandFactory(commandFactory), WithACPIntegrations(integrations...))...)
	}
}
