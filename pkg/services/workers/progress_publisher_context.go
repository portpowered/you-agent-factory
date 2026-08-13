package workers

import "context"

type progressPublisherContextKey struct{}

// WithProgressPublisher attaches a request-scoped observation sink to one
// detached execution. Workers keeps the sink in the execution context only;
// the process-scoped service does not retain Factory Session state.
func WithProgressPublisher(ctx context.Context, publisher ProgressPublisher) context.Context {
	if ctx == nil || publisher == nil {
		return ctx
	}
	return context.WithValue(ctx, progressPublisherContextKey{}, publisher)
}

// ProgressPublisherFromContext resolves the request-scoped sink, falling back
// to the construction-time sink used by direct Runner callers.
func ProgressPublisherFromContext(ctx context.Context, fallback ProgressPublisher) ProgressPublisher {
	if ctx != nil {
		if publisher, ok := ctx.Value(progressPublisherContextKey{}).(ProgressPublisher); ok && publisher != nil {
			return publisher
		}
	}
	return fallback
}
