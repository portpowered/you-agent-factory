package factoryconfig

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The authored AGENTS.md loader uses the same boundary enum normalization as
// Factory JSON. These helpers keep that translation owned by transport mapping
// while the legacy Config loader is being moved behind Factory Definitions.
func InternalFactoryHostedWorkerProviderFromPublic(value string) string {
	return internalFactoryHostedWorkerProviderFromPublic(value)
}

func InternalFactoryWorkerModelProviderFromPublic(
	value *factoryapi.WorkerModelProvider,
) string {
	return internalFactoryWorkerModelProviderFromPublic(value)
}

func InternalFactoryWorkerProviderFromPublic(
	value *factoryapi.WorkerProvider,
) string {
	return internalFactoryWorkerProviderFromPublic(value)
}

func InternalFactoryWorkstationKindFromPublic(
	value *factoryapi.WorkstationKind,
) factorydefinitions.WorkstationKind {
	return internalFactoryWorkstationKindFromPublic(value)
}

func InternalFactoryGuardTypeFromPublic(
	value factoryapi.GuardType,
) factorydefinitions.GuardType {
	return internalFactoryGuardTypeFromPublic(value)
}

func PublicFactoryHostedWorkerProviderFromInternal(value string) string {
	return publicFactoryHostedWorkerProviderFromInternal(value)
}

func PublicFactoryWorkerModelProviderFromInternal(
	value string,
) factoryapi.WorkerModelProvider {
	return publicFactoryWorkerModelProviderFromInternal(value)
}

func PublicFactoryWorkerProviderFromInternal(
	value string,
) factoryapi.WorkerProvider {
	return publicFactoryWorkerProviderFromInternal(value)
}

func PublicFactoryWorkstationKindFromInternal(
	value factorydefinitions.WorkstationKind,
) factoryapi.WorkstationKind {
	return publicFactoryWorkstationKindFromInternal(value)
}

func PublicFactoryGuardTypeStringFromInternal(
	value factorydefinitions.GuardType,
) string {
	return publicFactoryGuardTypeStringFromInternal(value)
}

func RuntimeResourceRequirementsFromBoundaryValue(value any) any {
	return runtimeResourceRequirementsFromBoundaryValue(value)
}

func ValidateOpenCodeAgentField(path, agent string) error {
	return validateOpenCodeAgentField(path, agent)
}

func MergeStopWords(base, extra []string) []string {
	return mergeStopWords(base, extra)
}

func CloneStringMap(values map[string]string) map[string]string {
	return cloneStringMap(values)
}

func NormalizeCanonicalWorkstationRuntime(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	normalizeCanonicalWorkstationRuntime(workstation)
}

func NormalizeWorkstationTaxonomyKind(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) {
	normalizeWorkstationTaxonomyKind(workstation)
}
