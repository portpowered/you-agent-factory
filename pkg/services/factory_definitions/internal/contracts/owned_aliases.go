package factorycontracts

import (
	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

type FactoryWorkerConfig = workerconfig.Config
type HostedLinearWorkerConfig = workerconfig.HostedLinearWorkerConfig
type HostedLinearWorkerClaimConfig = workerconfig.HostedLinearWorkerClaimConfig
type HostedLinearWorkerMappingConfig = workerconfig.HostedLinearWorkerMappingConfig
type HostedWorkerAuthConfig = workerconfig.HostedWorkerAuthConfig
type AgentToolsConfig = workerconfig.AgentToolsConfig
type ModelOperation = workerconfig.ModelOperation
type ModelOperationSlot = workerconfig.ModelOperationSlot
type ResourceConfig = catalogresource.Config

const (
	ResourceTypeInvocationSlot = catalogresource.TypeInvocationSlot
	ResourceTypeModel          = catalogresource.TypeModel
	ResourceTypeProviderQuota  = catalogresource.TypeProviderQuota
	ModelLocalityLocal         = workerconfig.ModelLocalityLocal
	ModelLocalityCloud         = workerconfig.ModelLocalityCloud
	AgentToolPolicyDisabled    = workerconfig.AgentToolPolicyDisabled
	AgentToolPolicyEnabled     = workerconfig.AgentToolPolicyEnabled
	AgentToolPolicyReadOnly    = workerconfig.AgentToolPolicyReadOnly
)

var CloneWorkerConfig = workerconfig.Clone

func NormalizeAgentToolPolicy(policy string) string {
	return workerconfig.NormalizeAgentToolPolicy(policy)
}

func IsKnownAgentToolPolicy(policy string) bool {
	return workerconfig.IsKnownAgentToolPolicy(policy)
}
