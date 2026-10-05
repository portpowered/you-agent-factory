package initializer

import "context"

// InvocationCancellation is the explicit cancellation authority for one
// Process.Execute invocation. The application process creates one authority
// per invocation and passes it through operation contracts to any hosted
// administrative control. Implementations must make repeated calls safe.
type InvocationCancellation interface {
	Cancel()
}

type cancellationOriginKey struct{}

// WithCancellationOrigin preserves the parent lifetime across derived scopes.
// The origin is request-scoped cancellation data, never a service dependency.
func WithCancellationOrigin(ctx, origin context.Context) context.Context {
	return context.WithValue(ctx, cancellationOriginKey{}, origin)
}

// CancellationOrigin reports the parent lifetime explicitly attached to ctx.
func CancellationOrigin(ctx context.Context) context.Context {
	origin, _ := ctx.Value(cancellationOriginKey{}).(context.Context)
	return origin
}
