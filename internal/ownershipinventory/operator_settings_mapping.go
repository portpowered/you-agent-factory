package ownershipinventory

import "strings"

const (
	operatorSettingsOwner         = "operator_settings"
	operatorSettingsPackagePrefix = "pkg/services/operator_settings"
)

type operatorSettingsMoveRule struct {
	exact     string
	prefix    string
	successor string
}

// operatorSettingsUnexpectedSiblingMoveRules mirrors
// cmd/packagetargetmanifestcheck nestedOwnerMoveRules for unexpected Operator
// Settings public top-level siblings inventoried in INV-SET-TOPLEVEL.
var operatorSettingsUnexpectedSiblingMoveRules = []operatorSettingsMoveRule{
	{exact: "identityinventory", prefix: "identityinventory/", successor: operatorSettingsPackagePrefix + "/internal"},
	{exact: "servicewire", prefix: "servicewire/", successor: operatorSettingsPackagePrefix + "/internal"},
	{exact: "testlink", prefix: "testlink/", successor: operatorSettingsPackagePrefix + "/internal"},
	{exact: "testproviders", prefix: "testproviders/", successor: operatorSettingsPackagePrefix + "/internal"},
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
	if isOperatorSettingsCanonicalRetainRest(rest) {
		return retainRow(packagePath, operatorSettingsOwner, DestinationKindOwner), true
	}
	successor, ok := operatorSettingsUnexpectedSiblingSuccessor(rest)
	if !ok {
		return PackageRow{}, false
	}
	return moveRow(
		packagePath,
		operatorSettingsOwner,
		successor,
		operatorSettingsDeletionCondition(rest),
	), true
}

func isOperatorSettingsCanonicalRetainRest(rest string) bool {
	top, _, _ := strings.Cut(rest, "/")
	if top == "" {
		return false
	}
	switch top {
	case "wire", "transports", "internal":
		return true
	default:
		return false
	}
}

func operatorSettingsUnexpectedSiblingSuccessor(rest string) (string, bool) {
	for _, rule := range operatorSettingsUnexpectedSiblingMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.successor, true
		}
	}
	return "", false
}

func operatorSettingsDeletionCondition(rest string) string {
	top, _, _ := strings.Cut(rest, "/")
	switch top {
	case "identityinventory":
		return "delete public identityinventory package after INV-SET-TOPLEVEL fold into operator_settings/internal completes"
	case "servicewire":
		return "delete public servicewire package after INV-SET-TOPLEVEL fold into operator_settings/internal completes"
	case "testlink":
		return "delete public testlink test-helper package after INV-SET-TOPLEVEL fold into operator_settings/internal completes"
	case "testproviders":
		return "delete public testproviders test-helper package after INV-SET-TOPLEVEL fold into operator_settings/internal completes"
	default:
		return "delete unexpected Operator Settings public sibling after INV-SET-TOPLEVEL private destination cutover completes"
	}
}

func isOperatorSettingsPrivateSuccessor(successor string) bool {
	switch successor {
	case operatorSettingsPackagePrefix + "/internal",
		operatorSettingsPackagePrefix + "/internal/services/document",
		operatorSettingsPackagePrefix + "/internal/services/resolution":
		return true
	default:
		return false
	}
}
