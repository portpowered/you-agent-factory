// Package workertaxonomy owns worker and worker-execution behavior identifiers
// for authored Factory definition validation and compatibility paths.
package workertaxonomy

import "strings"

const (
	WorkerTypeInference = "INFERENCE_WORKER"
	WorkerTypeAgent     = "AGENT_WORKER"
	WorkerTypeScript    = "SCRIPT_WORKER"
	WorkerTypePoller    = "POLLER_WORKER"
	WorkerTypeModel     = "MODEL_WORKER"
	WorkerTypeHosted    = "HOSTED_WORKER"

	WorkstationTypeInference = "INFERENCE_RUN"
	WorkstationTypeAgent     = "AGENT_RUN"
	WorkstationTypeScript    = "SCRIPT_RUN"
	WorkstationTypePoller    = "POLLER_RUN"
	WorkstationTypeModel     = "MODEL_WORKSTATION"
	WorkstationTypeInvoke    = "MODEL_INVOKE"
	WorkstationTypeLogical   = "LOGICAL_MOVE"
	WorkstationTypeClassify  = "CLASSIFIER_WORKSTATION"

	HostedWorkerProviderLinear = "LINEAR"
	ModelProviderDefault       = "DEFAULT"
)

type WorkstationKind string

const (
	WorkstationKindStandard = "standard"
	WorkstationKindRepeater = "repeater"
	WorkstationKindCron     = "cron"
	WorkstationKindPoller   = "poller"
)

var workerAliases = map[string]string{
	WorkerTypeInference: WorkerTypeInference, WorkerTypeAgent: WorkerTypeAgent,
	WorkerTypeScript: WorkerTypeScript, WorkerTypePoller: WorkerTypePoller,
	WorkerTypeModel: WorkerTypeInference, WorkerTypeHosted: WorkerTypePoller,
}

var workstationAliases = map[string]string{
	WorkstationTypeInference: WorkstationTypeInference, WorkstationTypeAgent: WorkstationTypeAgent,
	WorkstationTypeScript: WorkstationTypeScript, WorkstationTypePoller: WorkstationTypePoller,
	WorkstationTypeInvoke: WorkstationTypeInference, WorkstationTypeModel: WorkstationTypeAgent,
	WorkstationTypeClassify: WorkstationTypeClassify, WorkstationTypeLogical: WorkstationTypeLogical,
}

func normalize(value string, aliases map[string]string, preserveUnknown bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	upper := strings.ToUpper(trimmed)
	if canonical, ok := aliases[upper]; ok {
		return canonical
	}
	if preserveUnknown {
		return trimmed
	}
	return ""
}

func PermissiveWorkerType(value string) string { return normalize(value, workerAliases, true) }
func StrictWorkerType(value string) string     { return normalize(value, workerAliases, false) }
func PermissiveWorkstationType(value string) string {
	return normalize(value, workstationAliases, true)
}
func StrictWorkstationType(value string) string { return normalize(value, workstationAliases, false) }

func PublicWorkerTypeFromInternal(value string) string {
	switch strings.TrimSpace(value) {
	case WorkerTypeModel:
		return WorkerTypeInference
	case WorkerTypeHosted:
		return WorkerTypePoller
	case WorkerTypeInference, WorkerTypeAgent, WorkerTypeScript, WorkerTypePoller:
		return strings.TrimSpace(value)
	default:
		return PermissiveWorkerType(value)
	}
}

func PublicWorkstationTypeFromInternalRuntime(workstationType, workerType string, kind WorkstationKind) string {
	switch strings.TrimSpace(workstationType) {
	case WorkstationTypeInvoke:
		return WorkstationTypeInference
	case WorkstationTypeModel:
		if PermissiveWorkerType(workerType) == WorkerTypeScript {
			return WorkstationTypeScript
		}
		return WorkstationTypeAgent
	case WorkstationTypeLogical, WorkstationTypeClassify:
		return strings.TrimSpace(workstationType)
	case "":
		if kind == WorkstationKindPoller {
			return WorkstationTypePoller
		}
		return ""
	case WorkstationTypeInference, WorkstationTypeAgent, WorkstationTypeScript, WorkstationTypePoller:
		return strings.TrimSpace(workstationType)
	default:
		return PermissiveWorkstationType(workstationType)
	}
}

func IsPollerRunPublicWorkstationType(value string, kind WorkstationKind) bool {
	return PermissiveWorkstationType(value) == WorkstationTypePoller || (strings.TrimSpace(value) == "" && kind == WorkstationKindPoller)
}

func IsInferenceWorkerType(value string) bool { return StrictWorkerType(value) == WorkerTypeInference }
func IsAgentWorkerType(value string) bool     { return StrictWorkerType(value) == WorkerTypeAgent }
func IsProviderBackedWorkerType(value string) bool {
	return IsInferenceWorkerType(value) || IsAgentWorkerType(value)
}
func UsesModelhostLease(workerType, locality string) bool {
	return IsProviderBackedWorkerType(workerType) && locality == "LOCAL"
}
func IsScriptWorkerType(value string) bool           { return StrictWorkerType(value) == WorkerTypeScript }
func IsPollerWorkerType(value string) bool           { return StrictWorkerType(value) == WorkerTypePoller }
func ProjectWorkerBehaviorClass(value string) string { return StrictWorkerType(value) }

func IsInferenceRunWorkstationType(value string) bool {
	return StrictWorkstationType(value) == WorkstationTypeInference
}
func IsAgentRunWorkstationType(value string) bool {
	return StrictWorkstationType(value) == WorkstationTypeAgent
}
func IsScriptRunWorkstationType(value string) bool {
	return StrictWorkstationType(value) == WorkstationTypeScript
}
func IsPollerRunWorkstationType[K ~string](value string, kind K) bool {
	return StrictWorkstationType(value) == WorkstationTypePoller || (strings.TrimSpace(value) == "" && string(kind) == string(WorkstationKindPoller))
}
func ProjectWorkstationBehaviorClass[K ~string](value string, kind K) string {
	if projected := StrictWorkstationType(value); projected != "" {
		return projected
	}
	if strings.TrimSpace(value) == "" && string(kind) == string(WorkstationKindPoller) {
		return WorkstationTypePoller
	}
	return ""
}
