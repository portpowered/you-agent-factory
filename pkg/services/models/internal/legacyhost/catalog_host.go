package modelhost

import (
	"fmt"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"strings"
)

const supervisedHealthEndpointFlag = "--health-endpoint"

func canonicalModelKey(modelName string) string {
	return strings.ToUpper(strings.TrimSpace(modelName))
}
func modelScopedResource(runtimeCfg *models.RuntimeConfig, modelName string) *models.RuntimeResource {
	if runtimeCfg == nil {
		return nil
	}
	factoryCfg := runtimeCfg
	key := canonicalModelKey(modelName)
	for _, resource := range factoryCfg.Resources {
		if canonicalModelKey(resource.Model) != key {
			continue
		}
		if strings.TrimSpace(resource.Type) != models.RuntimeResourceTypeModel {
			continue
		}
		copied := resource
		return &copied
	}
	return nil
}

// LocalInvocationEndpoint preserves the worker's existing choice between a
// supervised endpoint and command execution without retaining a scoped host.
func LocalInvocationEndpoint(runtimeCfg *models.RuntimeConfig, modelName string, diagnostics map[string]string) (string, error) {
	resource := modelScopedResource(runtimeCfg, modelName)
	if resource == nil || !models.IsManagedRuntimeBackend(resource.Backend) {
		return "", nil
	}
	worker, err := localWorkerForModel(runtimeCfg, modelName)
	if err != nil {
		return "", err
	}
	if !workerDeclaresSupervisedHealthEndpoint(worker) {
		return "", nil
	}
	endpoint := strings.TrimSpace(diagnostics["endpoint"])
	if endpoint == "" {
		return "", ErrRuntimeNotReady
	}
	return endpoint, nil
}

func localWorkerForModel(
	runtimeCfg *models.RuntimeConfig,
	modelName string,
) (*models.RuntimeWorker, error) {
	if runtimeCfg == nil {
		return nil, fmt.Errorf("runtime config is not available")
	}
	target := canonicalModelKey(modelName)
	for _, worker := range runtimeCfg.Workers {
		if canonicalModelKey(worker.Model) != target {
			continue
		}
		if strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
			continue
		}
		copied := worker
		return &copied, nil
	}
	return nil, fmt.Errorf("local model worker not found for %q", modelName)
}
func supervisedHealthEndpointAndArgs(workerArgs []string) (string, []string, error) {
	args := append([]string(nil), workerArgs...)
	for i := 0; i < len(args); i++ {
		if args[i] != supervisedHealthEndpointFlag {
			continue
		}
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("flag %q requires a value", supervisedHealthEndpointFlag)
		}
		endpoint := strings.TrimSpace(args[i+1])
		remaining := append(append([]string(nil), args[:i]...), args[i+2:]...)
		if endpoint == "" {
			return "", nil, fmt.Errorf("flag %q requires a non-empty value", supervisedHealthEndpointFlag)
		}
		return endpoint, remaining, nil
	}
	return "", args, fmt.Errorf("supervised llama.cpp runtime requires worker arg %q", supervisedHealthEndpointFlag)
}

func workerDeclaresSupervisedHealthEndpoint(worker *models.RuntimeWorker) bool {
	if worker == nil {
		return false
	}
	_, _, err := supervisedHealthEndpointAndArgs(worker.Args)
	return err == nil
}
