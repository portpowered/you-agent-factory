package workertaxonomy

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

const (
	WorkerTypeInference        = factorycontracts.WorkerTypeInference
	WorkerTypeAgent            = factorycontracts.WorkerTypeAgent
	WorkerTypeScript           = factorycontracts.WorkerTypeScript
	WorkerTypePoller           = factorycontracts.WorkerTypePoller
	WorkerTypeModel            = factorycontracts.WorkerTypeModel
	WorkerTypeHosted           = factorycontracts.WorkerTypeHosted
	WorkstationTypeInference   = factorycontracts.WorkstationTypeInference
	WorkstationTypeAgent       = factorycontracts.WorkstationTypeAgent
	WorkstationTypeScript      = factorycontracts.WorkstationTypeScript
	WorkstationTypePoller      = factorycontracts.WorkstationTypePoller
	WorkstationTypeModel       = factorycontracts.WorkstationTypeModel
	WorkstationTypeInvoke      = factorycontracts.WorkstationTypeInvoke
	WorkstationTypeLogical     = factorycontracts.WorkstationTypeLogical
	WorkstationTypeClassify    = factorycontracts.WorkstationTypeClassify
	HostedWorkerProviderLinear = factorycontracts.HostedWorkerProviderLinear
	ModelProviderDefault       = factorycontracts.WorkerModelProviderDefault
	WorkstationKindStandard    = factorycontracts.WorkstationKindStandard
	WorkstationKindRepeater    = factorycontracts.WorkstationKindRepeater
	WorkstationKindCron        = factorycontracts.WorkstationKindCron
	WorkstationKindPoller      = factorycontracts.WorkstationKindPoller
)

type WorkstationKind = factorycontracts.WorkstationKind

func PermissiveWorkerType(value string) string { return factorycontracts.PermissiveWorkerType(value) }
func StrictWorkerType(value string) string     { return factorycontracts.StrictWorkerType(value) }
func PermissiveWorkstationType(value string) string {
	return factorycontracts.PermissiveWorkstationType(value)
}
func StrictWorkstationType(value string) string {
	return factorycontracts.StrictWorkstationType(value)
}
func PublicWorkerTypeFromInternal(value string) string {
	return factorycontracts.PublicWorkerTypeFromInternal(value)
}
func PublicWorkstationTypeFromInternalRuntime(value, worker string, kind WorkstationKind) string {
	return factorycontracts.PublicWorkstationTypeFromInternalRuntime(value, worker, kind)
}
func IsPollerRunPublicWorkstationType[K ~string](value string, kind K) bool {
	return factorycontracts.IsPollerRunPublicWorkstationType(value, factorycontracts.WorkstationKind(kind))
}
func IsInferenceWorkerType(value string) bool { return factorycontracts.IsInferenceWorkerType(value) }
func ProjectWorkerBehaviorClass(value string) string {
	return factorycontracts.ProjectWorkerBehaviorClass(value)
}
func IsInferenceRunWorkstationType(value string) bool {
	return factorycontracts.IsInferenceRunWorkstationType(value)
}
func IsAgentRunWorkstationType(value string) bool {
	return factorycontracts.IsAgentRunWorkstationType(value)
}
func IsScriptRunWorkstationType(value string) bool {
	return factorycontracts.IsScriptRunWorkstationType(value)
}
func IsPollerRunWorkstationType[K ~string](value string, kind K) bool {
	return factorycontracts.IsPollerRunWorkstationType(value, factorycontracts.WorkstationKind(kind))
}
func ProjectWorkstationBehaviorClass[K ~string](value string, kind K) string {
	return factorycontracts.ProjectWorkstationBehaviorClass(value, factorycontracts.WorkstationKind(kind))
}
