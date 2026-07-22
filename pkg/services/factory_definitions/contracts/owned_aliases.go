package factorycontracts

import (
	resource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

type FactoryWorkerConfig = workerconfig.Config
type HostedLinearWorkerConfig = workerconfig.HostedLinearWorkerConfig
type HostedLinearWorkerClaimConfig = workerconfig.HostedLinearWorkerClaimConfig
type HostedLinearWorkerMappingConfig = workerconfig.HostedLinearWorkerMappingConfig
type HostedWorkerAuthConfig = workerconfig.HostedWorkerAuthConfig
type AgentToolsConfig = workerconfig.AgentToolsConfig
type ModelOperation = workerconfig.ModelOperation
type ModelOperationSlot = workerconfig.ModelOperationSlot
type ResourceConfig = resource.Config

const (
	ResourceTypeInvocationSlot = resource.TypeInvocationSlot
	ResourceTypeModel          = resource.TypeModel
	ResourceTypeProviderQuota  = resource.TypeProviderQuota
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
