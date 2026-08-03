package service

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Option configures Providers root construction.
type Option func(*rootConfig)

type rootConfig struct {
	logger     logging.Logger
	lifecycles []providers.Lifecycle
}

// WithLogger injects the safe structured logger used for accepted-intent and
// terminal-outcome control records. A nil or omitted logger falls back to
// logging.NoopLogger.
func WithLogger(logger logging.Logger) Option {
	return func(config *rootConfig) { config.logger = logger }
}

// WithLifecycle contributes one additional owned lifecycle role closed by
// Service.Close.
func WithLifecycle(lifecycle providers.Lifecycle) Option {
	return func(config *rootConfig) { config.lifecycles = append(config.lifecycles, lifecycle) }
}
