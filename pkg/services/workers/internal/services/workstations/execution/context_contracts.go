package workerexecution

import (
	"context"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	ProjectTagKey    = workers.ProjectTagKey
	DefaultProjectID = workers.DefaultProjectID
	DefaultSessionID = workers.DefaultSessionID
)

type Context = workers.Context

var ResolveProjectID = workers.ResolveProjectID

type mockWorkersConfigKey struct{}
type mockWorkerOutputPolicyKey struct{}
type progressPublisherKey struct{}

// WithMockWorkersConfig attaches a cloned, request-scoped mock override to a
// detached Workers execution. The process-scoped Workers service retains no
// session-specific configuration.
func WithMockWorkersConfig(ctx context.Context, config *workers.MockWorkersConfig) context.Context {
	if ctx == nil || config == nil {
		return ctx
	}
	return context.WithValue(ctx, mockWorkersConfigKey{}, config.Clone())
}

// MockWorkersConfigFromContext resolves the detached mock override, if any.
func MockWorkersConfigFromContext(ctx context.Context) *workers.MockWorkersConfig {
	if ctx == nil {
		return nil
	}
	config, _ := ctx.Value(mockWorkersConfigKey{}).(*workers.MockWorkersConfig)
	return config.Clone()
}

// WithMockWorkerOutputPolicy attaches the output contract selected for one
// detached mock-worker execution.
func WithMockWorkerOutputPolicy(ctx context.Context, policy workers.OutputPolicy) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, mockWorkerOutputPolicyKey{}, policy)
}

// MockWorkerOutputPolicyFromContext resolves the detached output contract.
func MockWorkerOutputPolicyFromContext(ctx context.Context) workers.OutputPolicy {
	if ctx == nil {
		return workers.OutputPolicy{}
	}
	policy, _ := ctx.Value(mockWorkerOutputPolicyKey{}).(workers.OutputPolicy)
	return policy
}

// WithProgressPublisher attaches a request-scoped observation sink to one
// detached execution.
func WithProgressPublisher(ctx context.Context, publisher workers.ProgressPublisher) context.Context {
	if ctx == nil || publisher == nil {
		return ctx
	}
	return context.WithValue(ctx, progressPublisherKey{}, publisher)
}

// ProgressPublisherFromContext resolves a request-scoped sink, falling back
// to the construction-time sink used by direct runner callers.
func ProgressPublisherFromContext(ctx context.Context, fallback workers.ProgressPublisher) workers.ProgressPublisher {
	if ctx != nil {
		if publisher, ok := ctx.Value(progressPublisherKey{}).(workers.ProgressPublisher); ok && publisher != nil {
			return publisher
		}
	}
	return fallback
}
