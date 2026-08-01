package workerconfig

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

type Config = factorycontracts.Config
type HostedWorkerAuthConfig = factorycontracts.HostedWorkerAuthConfig
type HostedLinearWorkerConfig = factorycontracts.HostedLinearWorkerConfig
type HostedLinearWorkerMappingConfig = factorycontracts.HostedLinearWorkerMappingConfig
type HostedLinearWorkerClaimConfig = factorycontracts.HostedLinearWorkerClaimConfig
type ModelOperation = factorycontracts.ModelOperation
type ModelOperationSlot = factorycontracts.ModelOperationSlot
type AgentToolsConfig = factorycontracts.AgentToolsConfig

const (
	ModelLocalityLocal              = factorycontracts.ModelLocalityLocal
	ModelLocalityCloud              = factorycontracts.ModelLocalityCloud
	ModelOperationContentTypeText   = factorycontracts.ModelOperationContentTypeText
	ModelOperationContentTypeImage  = factorycontracts.ModelOperationContentTypeImage
	ModelOperationContentTypeAudio  = factorycontracts.ModelOperationContentTypeAudio
	ModelOperationContentTypeJSON   = factorycontracts.ModelOperationContentTypeJSON
	ModelOperationContentTypeBinary = factorycontracts.ModelOperationContentTypeBinary
	AgentToolPolicyDisabled         = factorycontracts.AgentToolPolicyDisabled
	AgentToolPolicyReadOnly         = factorycontracts.AgentToolPolicyReadOnly
	AgentToolPolicyEnabled          = factorycontracts.AgentToolPolicyEnabled
)

func EffectiveAgentToolPolicy(cfg *AgentToolsConfig) string {
	return factorycontracts.EffectiveAgentToolPolicy((*factorycontracts.AgentToolsConfig)(cfg))
}
func NormalizeAgentToolPolicy(policy string) string {
	return factorycontracts.NormalizeAgentToolPolicy(policy)
}
func IsKnownAgentToolPolicy(policy string) bool {
	return factorycontracts.IsKnownAgentToolPolicy(policy)
}
func AgentToolsAllowExecution(policy string) bool {
	return factorycontracts.AgentToolsAllowExecution(policy)
}
