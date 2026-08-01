// Worker taxonomy helpers normalize the Definition-owned worker and
// workstation identifiers used by validation and compatibility paths.
package factorycontracts

import "strings"

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

func IsPollerRunPublicWorkstationType(value string, kind WorkstationKind) bool {
	return PermissiveWorkstationType(value) == WorkstationTypePoller || (strings.TrimSpace(value) == "" && kind == WorkstationKindPoller)
}
