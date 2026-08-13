package workers

import "context"

type mockWorkersContextKey struct{}
type mockWorkerOutputPolicyContextKey struct{}

// WithMockWorkersConfig carries one detached mock-worker override through a
// single execution's command and provider effects. The context is an
// invocation transport only; the Workers and Providers roots retain no
// session-specific configuration.
func WithMockWorkersConfig(ctx context.Context, config *MockWorkersConfig) context.Context {
	if ctx == nil || config == nil {
		return ctx
	}
	return context.WithValue(ctx, mockWorkersContextKey{}, config.Clone())
}

// MockWorkersConfigFromContext returns the detached mock-worker override, if
// one was attached to an execution context.
func MockWorkersConfigFromContext(ctx context.Context) *MockWorkersConfig {
	if ctx == nil {
		return nil
	}
	config, _ := ctx.Value(mockWorkersContextKey{}).(*MockWorkersConfig)
	return config.Clone()
}

// WithMockWorkerOutputPolicy carries the detached output contract selected for
// the same execution as a mock-worker command. The process-scoped command
// runner uses it to emit provider-native mock output that still satisfies the
// invocation's authored decision-envelope and stop-token policy.
func WithMockWorkerOutputPolicy(ctx context.Context, policy OutputPolicy) context.Context {
	if ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, mockWorkerOutputPolicyContextKey{}, policy)
}

// MockWorkerOutputPolicyFromContext returns the output contract for the
// current mock-worker execution, if one was attached.
func MockWorkerOutputPolicyFromContext(ctx context.Context) OutputPolicy {
	if ctx == nil {
		return OutputPolicy{}
	}
	policy, _ := ctx.Value(mockWorkerOutputPolicyContextKey{}).(OutputPolicy)
	return policy
}
