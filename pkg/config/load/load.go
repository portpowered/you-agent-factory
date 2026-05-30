// Package load is the canonical entrypoint for loading and validating factory
// definitions from on-disk directories or canonical JSON bytes.
package load

import (
	"errors"

	"github.com/portpowered/infinite-you/pkg/config"
)

// Re-export stable load error values for callers that import pkg/config/load only.
var (
	ErrInvalidNamedFactory   = config.ErrInvalidNamedFactory
	ErrFactoryLayoutNotFound = config.ErrFactoryLayoutNotFound
)

// LoadOptions configures canonical JSON load behavior.
type LoadOptions struct {
	WorkstationLoader config.WorkstationLoader
}

// LoadFromFactoryDir loads and validates a factory from a concrete on-disk
// factory directory without following current-factory pointer indirection.
func LoadFromFactoryDir(factoryDir string, workstationLoader config.WorkstationLoader) (*config.LoadedFactoryConfig, error) {
	return config.LoadFromFactoryDir(factoryDir, workstationLoader)
}

// LoadFromCanonicalJSON normalizes and validates canonical factory JSON without
// requiring an on-disk factory directory.
func LoadFromCanonicalJSON(payload []byte, opts LoadOptions) (*config.LoadedFactoryConfig, error) {
	return config.LoadFromCanonicalJSON(payload, opts.WorkstationLoader)
}

// LoadRuntimeConfig reads factory.json plus worker/workstation AGENTS.md files
// into a single runtime configuration object, following current-factory pointer
// indirection for workspace roots.
func LoadRuntimeConfig(factoryDir string, workstationLoader config.WorkstationLoader) (*config.LoadedFactoryConfig, error) {
	return config.LoadRuntimeConfig(factoryDir, workstationLoader)
}

// LoadRuntimeConfigFromFactoryDir loads one concrete factory directory without
// following current-factory pointer indirection.
func LoadRuntimeConfigFromFactoryDir(factoryDir string, workstationLoader config.WorkstationLoader) (*config.LoadedFactoryConfig, error) {
	return config.LoadRuntimeConfigFromFactoryDir(factoryDir, workstationLoader)
}

// IsInvalidNamedFactory reports whether err wraps ErrInvalidNamedFactory.
func IsInvalidNamedFactory(err error) bool {
	return errors.Is(err, ErrInvalidNamedFactory)
}

// IsFactoryLayoutNotFound reports whether err wraps ErrFactoryLayoutNotFound.
func IsFactoryLayoutNotFound(err error) bool {
	return errors.Is(err, ErrFactoryLayoutNotFound)
}
