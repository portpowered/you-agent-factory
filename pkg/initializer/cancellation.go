package initializer

// InvocationCancellation is the explicit cancellation authority for one
// Process.Execute invocation. The application process creates one authority
// per invocation and passes it through operation contracts to any hosted
// administrative control. Implementations must make repeated calls safe.
type InvocationCancellation interface {
	Cancel()
}
