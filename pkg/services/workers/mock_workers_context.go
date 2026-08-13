package workers

import "context"

type mockWorkersContextKey struct{}

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
