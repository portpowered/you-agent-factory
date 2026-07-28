package ownershipinventory

import "strings"

const (
	operatorSettingsOwner         = "operator_settings"
	operatorSettingsPackagePrefix = "pkg/services/operator_settings"
)

type operatorSettingsMoveRule struct {
	exact      string
	prefix     string
	subservice string // empty means owner/internal root
}

// operatorSettingsMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules
// for operator_settings. Ownership rows keep owner destination with concrete successor paths.
var operatorSettingsMoveRules = []operatorSettingsMoveRule{
	{exact: "identityinventory", prefix: "identityinventory/", subservice: "document"},
	{exact: "servicewire", prefix: "servicewire/", subservice: ""},
	{exact: "testlink", prefix: "testlink/", subservice: ""},
	{exact: "testproviders", prefix: "testproviders/", subservice: ""},
	{exact: "testdata", prefix: "testdata/", subservice: ""},
	{exact: "internal/service", prefix: "internal/service/", subservice: ""},
}

func operatorSettingsMapping(packagePath string) (PackageRow, bool) {
	if packagePath == operatorSettingsPackagePrefix {
		return retainRow(packagePath, operatorSettingsOwner, DestinationKindOwner), true
	}
	const prefix = operatorSettingsPackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	if isOperatorSettingsCanonicalRetain(rest) {
		return retainRow(packagePath, operatorSettingsOwner, DestinationKindOwner), true
	}
	subservice, ok := operatorSettingsMoveSubservice(rest)
	if !ok {
		subservice = ""
	}
	return moveRow(
		packagePath,
		operatorSettingsOwner,
		operatorSettingsSuccessor(subservice),
		operatorSettingsDeletionCondition(subservice),
	), true
}

func isOperatorSettingsCanonicalRetain(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/document"):
		return true
	case strings.HasPrefix(rest, "internal/services/resolution"):
		return true
	default:
		return false
	}
}

func operatorSettingsMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range operatorSettingsMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func operatorSettingsSuccessor(subservice string) string {
	if subservice == "" {
		return operatorSettingsPackagePrefix + "/internal"
	}
	return operatorSettingsPackagePrefix + "/internal/services/" + subservice
}

func operatorSettingsDeletionCondition(subservice string) string {
	if subservice == "" {
		return "delete transitional top-level package after CLN-OPS-FOLD-TOPLEVEL cutover proof"
	}
	return "delete public package after IMP-OPS-" + subservice + " private subservice cutover proof"
}
