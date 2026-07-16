package service

import (
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/workers"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	"go.uber.org/zap"
)

// Config carries explicit runtime collaborators for worker-side scheduling.
type Config struct {
	Logger        *zap.Logger
	Clock         factory.Clock
	CommandRunner workers.CommandRunner
	// WorkflowID overrides factory-dir workflow identity for cron ticks when non-empty.
	WorkflowID string
	// DefaultFactoryDir is the coordinator factory directory used when runtime factoryDir is empty.
	DefaultFactoryDir string
	// HostedWorkers is the already-constructed hosted-worker component selected
	// by composition. Scheduler lifecycle code passes it through unchanged.
	HostedWorkers hostedworkers.Config
}
