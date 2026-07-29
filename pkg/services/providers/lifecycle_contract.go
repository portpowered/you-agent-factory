package providers

import "context"

// Lifecycle is the exact Providers shutdown role consumed by the application
// initializer. Product callers continue to depend on Service only.
type Lifecycle interface {
	Close(context.Context) error
}
