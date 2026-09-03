package acp

import "context"

// InvocationProfile is the immutable customer profile selected when one ACP
// connection enters the application. It prevents later protocol operations
// from consulting process-global environment state while other connections
// are active.
type InvocationProfile struct {
	HomeDir             string
	WorkerModelProvider string
	WorkerModel         string
}

type invocationProfileContextKey struct{}

// WithInvocationProfile binds one connection-owned profile to ctx.
func WithInvocationProfile(ctx context.Context, profile InvocationProfile) context.Context {
	return context.WithValue(ctx, invocationProfileContextKey{}, profile)
}

// InvocationProfileFromContext returns the profile captured at admission.
func InvocationProfileFromContext(ctx context.Context) (InvocationProfile, bool) {
	if ctx == nil {
		return InvocationProfile{}, false
	}
	profile, ok := ctx.Value(invocationProfileContextKey{}).(InvocationProfile)
	return profile, ok
}
