// Package wire constructs the published Providers root Service from the
// parent-private catalog subservice.
package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// CursorPlatformDependencies are platform facts required for oversized Windows
// Cursor prompt materialization in the built-in Providers Execution adapter.
type CursorPlatformDependencies struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
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

// NewService constructs one inert Providers root over sibling Catalog and
// Execution capabilities sharing the same private catalog identity authority.
func NewService(options ...Option) (providers.Service, error) {
	var config wireOptions
	for _, option := range options {
		if option != nil {
			option.apply(&config)
		}
	}
	catalogService, err := catalogwire.NewService(config.catalog...)
	if err != nil {
		return nil, err
	}
	return newRoot(catalogService, config.commandRunner, config.workersCommandRunner, config.cursorPlatform)
}

func newRoot(
	catalogService catalog.Service,
	commandRunner platformprocess.CommandRunner,
	workersCommandRunner workers.CommandRunner,
	cursorPlatform CursorPlatformDependencies,
) (providers.Service, error) {
	if workersCommandRunner == nil && commandRunner != nil {
		workersCommandRunner = workerprocess.AdaptCommandRunner(commandRunner)
	}
	executionService, err := executionwire.NewBuiltInService(
		catalogService,
		executionwire.BuiltInDependenciesFromWorkersRunner(
			workersCommandRunner,
			executionwire.CursorPlatformDependencies{
				OperatingSystem: cursorPlatform.OperatingSystem,
				TemporaryDir:    cursorPlatform.TemporaryDir,
				TemporaryFiles:  cursorPlatform.TemporaryFiles,
			},
		),
	)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService, executionService)
}
