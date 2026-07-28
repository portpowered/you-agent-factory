package ownershipinventory

import "strings"

const (
	factoryDefinitionsOwner         = "factory_definitions"
	factoryDefinitionsPackagePrefix = "pkg/services/factory_definitions"
)

type factoryDefinitionsMoveRule struct {
	exact      string
	prefix     string
	subservice string // empty means owner/internal root
}

// factoryDefinitionsMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules
// for factory_definitions. Ownership rows keep owner destination with concrete successor paths.
var factoryDefinitionsMoveRules = []factoryDefinitionsMoveRule{
	{exact: "authoredlayout", prefix: "authoredlayout/", subservice: "authoring_layout"},
	{exact: "namedfactories", prefix: "namedfactories/", subservice: "catalog"},
	{exact: "namedpaths", prefix: "namedpaths/", subservice: "catalog"},
	{exact: "persistence", prefix: "persistence/", subservice: "catalog"},
	{exact: "resource", prefix: "resource/", subservice: "catalog"},
	{prefix: "internal/namedfactories", subservice: "catalog"},
	{exact: "definition", prefix: "definition/", subservice: "compilation"},
	{exact: "loading", prefix: "loading/", subservice: "compilation"},
	{exact: "loadedsource", prefix: "loadedsource/", subservice: "compilation"},
	{exact: "validation", prefix: "validation/", subservice: "validation"},
	{exact: "snapshotcapture", prefix: "snapshotcapture/", subservice: "snapshots_portability"},
	{exact: "portableconfig", prefix: "portableconfig/", subservice: "snapshots_portability"},
	{exact: "editable", prefix: "editable/", subservice: "snapshots_portability"},
	{exact: "packagedinstallation", prefix: "packagedinstallation/", subservice: "distribution"},
	{exact: "packages", prefix: "packages/", subservice: "distribution"},
	{exact: "decisionenvelope", prefix: "decisionenvelope/"},
	{exact: "invocationinterpolation", prefix: "invocationinterpolation/"},
	{exact: "invocationoutput", prefix: "invocationoutput/"},
	{exact: "invocationworktype", prefix: "invocationworktype/"},
	{exact: "quorumpolicy", prefix: "quorumpolicy/"},
	{exact: "workpropagation", prefix: "workpropagation/"},
	{exact: "workstationexecution", prefix: "workstationexecution/"},
	{exact: "ttsobservability", prefix: "ttsobservability/"},
	{exact: "runtimeconfig", prefix: "runtimeconfig/"},
	{exact: "replayconfig", prefix: "replayconfig/"},
	{exact: "contracts", prefix: "contracts/"},
	{exact: "workers", prefix: "workers/"},
	{prefix: "internal/testcomposition"},
}

func factoryDefinitionsMapping(packagePath string) (PackageRow, bool) {
	if packagePath == factoryDefinitionsPackagePrefix {
		return retainRow(packagePath, factoryDefinitionsOwner, DestinationKindOwner), true
	}
	const prefix = factoryDefinitionsPackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	if isFactoryDefinitionsCanonicalRetain(rest) {
		return retainRow(packagePath, factoryDefinitionsOwner, DestinationKindOwner), true
	}
	subservice, ok := factoryDefinitionsMoveSubservice(rest)
	if !ok {
		subservice = ""
	}
	return moveRow(
		packagePath,
		factoryDefinitionsOwner,
		factoryDefinitionsSuccessor(subservice),
		factoryDefinitionsDeletionCondition(subservice, rest),
	), true
}

func isFactoryDefinitionsCanonicalRetain(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/catalog"):
		return true
	case strings.HasPrefix(rest, "internal/services/validation"):
		return true
	default:
		return false
	}
}

func factoryDefinitionsMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range factoryDefinitionsMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func factoryDefinitionsSuccessor(subservice string) string {
	if subservice == "" {
		return factoryDefinitionsPackagePrefix + "/internal"
	}
	return factoryDefinitionsPackagePrefix + "/internal/services/" + subservice
}

func factoryDefinitionsDeletionCondition(subservice, rest string) string {
	if strings.HasPrefix(rest, "contracts") {
		return "delete contracts mega-barrel after CLN-DEF-CONTRACTS cutover proof"
	}
	if subservice == "" {
		return "delete transitional top-level package after CLN-DEF-FOLD-TOPLEVEL cutover proof"
	}
	return "delete public package after IMP-DEF-" + subservice + " private subservice cutover proof"
}
