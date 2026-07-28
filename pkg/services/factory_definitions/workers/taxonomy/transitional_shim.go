// Package workertaxonomy exposes transitional compile-time re-exports of the
// validation-owned authored schema helper under internal/services/validation.
// Production ownership lives in internal/services/validation/authoredmodel/taxonomy.
package workertaxonomy

import taxonomyimpl "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy"

type WorkstationKind = taxonomyimpl.WorkstationKind

const (
	WorkerTypeInference = taxonomyimpl.WorkerTypeInference
	WorkerTypeAgent     = taxonomyimpl.WorkerTypeAgent
	WorkerTypeScript    = taxonomyimpl.WorkerTypeScript
	WorkerTypePoller    = taxonomyimpl.WorkerTypePoller
	WorkerTypeModel     = taxonomyimpl.WorkerTypeModel
	WorkerTypeHosted    = taxonomyimpl.WorkerTypeHosted

	WorkstationTypeInference = taxonomyimpl.WorkstationTypeInference
	WorkstationTypeAgent     = taxonomyimpl.WorkstationTypeAgent
	WorkstationTypeScript    = taxonomyimpl.WorkstationTypeScript
	WorkstationTypePoller    = taxonomyimpl.WorkstationTypePoller
	WorkstationTypeModel     = taxonomyimpl.WorkstationTypeModel
	WorkstationTypeInvoke    = taxonomyimpl.WorkstationTypeInvoke
	WorkstationTypeLogical   = taxonomyimpl.WorkstationTypeLogical
	WorkstationTypeClassify  = taxonomyimpl.WorkstationTypeClassify

	HostedWorkerProviderLinear = taxonomyimpl.HostedWorkerProviderLinear
	ModelProviderDefault       = taxonomyimpl.ModelProviderDefault

	WorkstationKindStandard = taxonomyimpl.WorkstationKindStandard
	WorkstationKindRepeater = taxonomyimpl.WorkstationKindRepeater
	WorkstationKindCron     = taxonomyimpl.WorkstationKindCron
	WorkstationKindPoller   = taxonomyimpl.WorkstationKindPoller
)

var (
	PermissiveWorkerType                      = taxonomyimpl.PermissiveWorkerType
	StrictWorkerType                          = taxonomyimpl.StrictWorkerType
	PermissiveWorkstationType                 = taxonomyimpl.PermissiveWorkstationType
	StrictWorkstationType                     = taxonomyimpl.StrictWorkstationType
	PublicWorkerTypeFromInternal              = taxonomyimpl.PublicWorkerTypeFromInternal
	PublicWorkstationTypeFromInternalRuntime  = taxonomyimpl.PublicWorkstationTypeFromInternalRuntime
	IsPollerRunPublicWorkstationType          = taxonomyimpl.IsPollerRunPublicWorkstationType
	IsInferenceWorkerType                     = taxonomyimpl.IsInferenceWorkerType
	IsAgentWorkerType                         = taxonomyimpl.IsAgentWorkerType
	IsProviderBackedWorkerType                = taxonomyimpl.IsProviderBackedWorkerType
	UsesModelhostLease                        = taxonomyimpl.UsesModelhostLease
	IsScriptWorkerType                        = taxonomyimpl.IsScriptWorkerType
	IsPollerWorkerType                        = taxonomyimpl.IsPollerWorkerType
	ProjectWorkerBehaviorClass                = taxonomyimpl.ProjectWorkerBehaviorClass
	IsInferenceRunWorkstationType    = taxonomyimpl.IsInferenceRunWorkstationType
	IsAgentRunWorkstationType        = taxonomyimpl.IsAgentRunWorkstationType
	IsScriptRunWorkstationType       = taxonomyimpl.IsScriptRunWorkstationType
)

func IsPollerRunWorkstationType[K ~string](value string, kind K) bool {
	return taxonomyimpl.IsPollerRunWorkstationType(value, kind)
}

func ProjectWorkstationBehaviorClass[K ~string](value string, kind K) string {
	return taxonomyimpl.ProjectWorkstationBehaviorClass(value, kind)
}
