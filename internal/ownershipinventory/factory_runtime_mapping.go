package ownershipinventory

import "strings"

const (
	factoryRuntimeOwner         = "factory_runtime"
	factoryRuntimePackagePrefix = "pkg/services/factory_runtime"
)

type factoryRuntimeMoveRule struct {
	exact      string
	prefix     string
	subservice string // empty means owner/internal root
}

// factoryRuntimeMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules
// for factory_runtime. Ownership rows keep owner destination with concrete successor paths.
var factoryRuntimeMoveRules = []factoryRuntimeMoveRule{
	{exact: "internal", subservice: ""},
	{exact: "internal/host", prefix: "internal/host/", subservice: ""},
	{exact: "javascript", prefix: "javascript/", subservice: "orchestration"},
	{prefix: "internal/orchestrators/", subservice: "orchestration"},
	{exact: "tooling", prefix: "tooling/", subservice: "orchestration"},
	{exact: "checkpointstore", prefix: "checkpointstore/", subservice: "checkpoint_recovery"},
	{exact: "checkpointsummary", prefix: "checkpointsummary/", subservice: "checkpoint_recovery"},
	{exact: "build", prefix: "build/", subservice: "instance_host"},
	{exact: "internal/factorystatus", prefix: "internal/factorystatus/", subservice: ""},
	{exact: "internal/legacysnapshot", prefix: "internal/legacysnapshot/", subservice: ""},
	{exact: "internal/rootobservation", prefix: "internal/rootobservation/", subservice: ""},
	{exact: "internal/service", prefix: "internal/service/", subservice: ""},
	{exact: "testkit", prefix: "testkit/", subservice: ""},
	{exact: "testdata", prefix: "testdata/", subservice: ""},
	{exact: "context", prefix: "context/", subservice: "orchestration"},
	{exact: "definitionmapping", prefix: "definitionmapping/", subservice: "orchestration"},
	{exact: "engine", prefix: "engine/", subservice: "orchestration"},
	{exact: "exhaustiontests", prefix: "exhaustiontests/", subservice: "orchestration"},
	{exact: "metrics", prefix: "metrics/", subservice: "orchestration"},
	{exact: "orchestrationowner", prefix: "orchestrationowner/", subservice: "orchestration"},
	{exact: "orchestratorcontract", prefix: "orchestratorcontract/", subservice: "orchestration"},
	{exact: "replayhooks", prefix: "replayhooks/", subservice: "orchestration"},
	{exact: "runtime", prefix: "runtime/", subservice: "orchestration"},
	{exact: "runtimecontract", prefix: "runtimecontract/", subservice: "orchestration"},
	{exact: "scheduler", prefix: "scheduler/", subservice: "orchestration"},
	{exact: "state", prefix: "state/", subservice: "orchestration"},
	{exact: "subsystems", prefix: "subsystems/", subservice: "orchestration"},
	{exact: "throttle", prefix: "throttle/", subservice: "orchestration"},
	{exact: "token", prefix: "token/", subservice: "orchestration"},
	{exact: "token_transformer", prefix: "token_transformer/", subservice: "orchestration"},
}

func factoryRuntimeMapping(packagePath string) (PackageRow, bool) {
	if packagePath == factoryRuntimePackagePrefix {
		return retainRow(packagePath, factoryRuntimeOwner, DestinationKindOwner), true
	}
	const prefix = factoryRuntimePackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	if isFactoryRuntimeCanonicalRetain(rest) {
		return retainRow(packagePath, factoryRuntimeOwner, DestinationKindOwner), true
	}
	subservice, ok := factoryRuntimeMoveSubservice(rest)
	if !ok {
		subservice = ""
	}
	return moveRow(
		packagePath,
		factoryRuntimeOwner,
		factoryRuntimeSuccessor(subservice),
		factoryRuntimeDeletionCondition(subservice),
	), true
}

func isFactoryRuntimeCanonicalRetain(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case rest == "internal" || strings.HasPrefix(rest, "internal/host"):
		return true
	case strings.HasPrefix(rest, "internal/services/orchestration"):
		return true
	case strings.HasPrefix(rest, "internal/services/instance_host"):
		return true
	case strings.HasPrefix(rest, "internal/services/dispatch_planning"):
		return true
	case strings.HasPrefix(rest, "internal/services/checkpoint_recovery"):
		return true
	default:
		return false
	}
}

func factoryRuntimeMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range factoryRuntimeMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func factoryRuntimeSuccessor(subservice string) string {
	if subservice == "" {
		return factoryRuntimePackagePrefix + "/internal"
	}
	return factoryRuntimePackagePrefix + "/internal/services/" + subservice
}

func factoryRuntimeDeletionCondition(subservice string) string {
	if subservice == "" {
		return "delete transitional top-level package after CLN-RUN-FOLD-TOPLEVEL cutover proof"
	}
	return "delete public package after IMP-RUN-" + subservice + " private subservice cutover proof"
}
