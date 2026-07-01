package service

import (
	"net/http"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/hostedworkers"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// Config carries explicit runtime collaborators for worker-side scheduling.
type Config struct {
	Logger        *zap.Logger
	Clock         clockwork.Clock
	CommandRunner workers.CommandRunner
	// WorkflowID overrides factory-dir workflow identity for cron ticks when non-empty.
	WorkflowID string
	// DefaultFactoryDir is the coordinator factory directory used when runtime factoryDir is empty.
	DefaultFactoryDir string
	// HostedHTTPClient overrides the default HTTP client for repository-owned hosted pollers.
	HostedHTTPClient *http.Client
	// HostedSecretResolver resolves hosted-worker auth.secretRef values at runtime.
	HostedSecretResolver hostedworkers.SecretResolver
	// HostedLinearEndpoint overrides the default Linear API endpoint for hosted pollers.
	HostedLinearEndpoint string
}
