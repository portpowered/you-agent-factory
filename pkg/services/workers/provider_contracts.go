package workers

import "context"

// Provider performs one inference request against an external model provider.
//
// Concrete provider adapters implement this contract, while Factory Runtime and
// other peer services depend on the Workers root port without importing nested
// provider implementation packages.
type Provider interface {
	Infer(context.Context, ProviderInferenceRequest) (InferenceResponse, error)
}
