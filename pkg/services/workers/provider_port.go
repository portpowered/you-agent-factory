package workers

import (
	"context"
	"fmt"
	"strings"
)

// Provider is a transitional request-scoped compatibility port retained for
// callers that still inject an Infer-shaped fake. Production Workers
// execution uses providers.Service; this port is not used for provider
// selection, execution, continuation, or failure authority.
// TODO(WSE-09): remove after the retained compatibility consumers migrate.
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
