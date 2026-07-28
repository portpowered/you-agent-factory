package workers

import "context"

// Provider performs one inference request against an external model provider.
// Cross-service consumers (for example Recordings replay binding) name this
// Workers root contract instead of importing provider/inferencecontract
// packages.
type Provider interface {
	Infer(context.Context, ProviderInferenceRequest) (InferenceResponse, error)
}
