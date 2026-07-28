package inference

import "errors"

// ErrInvocationInFlight reports that an invocation runtime accepted work without
// completing it. Inference retains lease capacity until explicit cancel or context
// cancellation converges the outcome.
var ErrInvocationInFlight = errors.New("model invocation in flight")
