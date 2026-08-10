package workers

import (
	"context"
	"fmt"
	"strings"
)

// Provider is the request-scoped compatibility adapter retained at the Workers
// root for existing construction boundaries. Providers owns provider protocol,
// selection, session identity, native adapters, and the one normalized attempt;
// Workers owns when to execute this request and how to retry or schedule it.
// New durable provider-effect contracts belong under Providers Execution.
type Provider interface {
	Infer(context.Context, ProviderInferenceRequest) (InferenceResponse, error)
}

const ExecutorProviderACP = "ACP"

// RunnerIdentityForWorker maps the authored execution mechanism and provider
// identity into the Providers catalog identity selected at runtime.
func RunnerIdentityForWorker(executorProvider, modelProvider string) (string, error) {
	executorProvider = strings.TrimSpace(executorProvider)
	modelProvider = strings.TrimSpace(modelProvider)
	if strings.EqualFold(executorProvider, ExecutorProviderACP) {
		if modelProvider == "" {
			return "", fmt.Errorf("executorProvider ACP requires modelProvider to name an ACP integration")
		}
		return modelProvider, nil
	}
	if executorProvider != "" && !strings.EqualFold(executorProvider, "SCRIPT_WRAP") {
		return executorProvider, nil
	}
	return "", nil
}

func UsesNamedProvider(executorProvider, modelProvider string) bool {
	identity, err := RunnerIdentityForWorker(executorProvider, modelProvider)
	return err == nil && identity != ""
}
