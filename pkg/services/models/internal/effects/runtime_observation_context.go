package effects

import "context"

type runtimeObservationContextKey struct{}

type runtimeObservationContext struct {
	recorder      RuntimeEvidenceRecorder
	configuration ResolvedHostConfiguration
}

// WithRuntimeObservation carries invocation-scoped private evidence and its
// resolved host configuration to an owning runtime adapter. The value is
// transient and is never serialized or exposed through the Models contract.
func WithRuntimeObservation(
	ctx context.Context,
	recorder RuntimeEvidenceRecorder,
	configuration ResolvedHostConfiguration,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if isNilRuntimeEvidenceRecorder(recorder) {
		return ctx
	}
	return context.WithValue(ctx, runtimeObservationContextKey{}, runtimeObservationContext{
		recorder: recorder, configuration: configuration.Clone(),
	})
}

// RuntimeObservationFromContext retrieves the optional private invocation
// observer and a detached copy of the selected host configuration.
func RuntimeObservationFromContext(
	ctx context.Context,
) (RuntimeEvidenceRecorder, ResolvedHostConfiguration, bool) {
	if ctx == nil {
		return nil, ResolvedHostConfiguration{}, false
	}
	observation, ok := ctx.Value(runtimeObservationContextKey{}).(runtimeObservationContext)
	if !ok || isNilRuntimeEvidenceRecorder(observation.recorder) {
		return nil, ResolvedHostConfiguration{}, false
	}
	return observation.recorder, observation.configuration.Clone(), true
}
