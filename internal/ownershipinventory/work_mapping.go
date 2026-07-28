package ownershipinventory

import "strings"

const (
	workOwner         = "work"
	workPackagePrefix = "pkg/services/work"
)

type workMoveRule struct {
	exact      string
	prefix     string
	subservice string // empty means owner/internal root
}

// workMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules for work.
// Confirmed against docs/internal/packaged-service-structure/work-top-level-inventory.md
// (INV-WORK-TOPLEVEL). Transitional public service/ facades use
// legacyServiceImplementationMapping in map.go; every other unexpected Work
// top-level sibling maps move with a private successor (never retain→work).
var workMoveRules = []workMoveRule{
	{exact: "internal/contenturl", prefix: "internal/contenturl/", subservice: ""},
	{exact: "internal/invocationreturnpolicy", prefix: "internal/invocationreturnpolicy/", subservice: ""},
	{exact: "internal/requestadmission", prefix: "internal/requestadmission/", subservice: ""},
	{exact: "materialize", prefix: "materialize/", subservice: "content_materialization"},
	{exact: "testdata", prefix: "testdata/", subservice: ""},
}

func workMapping(packagePath string) (PackageRow, bool) {
	if packagePath == workPackagePrefix {
		return retainRow(packagePath, workOwner, DestinationKindOwner), true
	}
	const prefix = workPackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	if isWorkCanonicalRetain(rest) {
		return retainRow(packagePath, workOwner, DestinationKindOwner), true
	}
	subservice, ok := workMoveSubservice(rest)
	if !ok {
		subservice = ""
	}
	return moveRow(
		packagePath,
		workOwner,
		workSuccessor(subservice),
		workDeletionCondition(subservice),
	), true
}

func isWorkCanonicalRetain(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case rest == "transports" || strings.HasPrefix(rest, "transports/"):
		return true
	case strings.HasPrefix(rest, "internal/services/admission"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_staging"):
		return true
	case strings.HasPrefix(rest, "internal/services/content_materialization"):
		return true
	case strings.HasPrefix(rest, "internal/services/state_access"):
		return true
	default:
		return false
	}
}

func workMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range workMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func workSuccessor(subservice string) string {
	if subservice == "" {
		return workPackagePrefix + "/internal"
	}
	return workPackagePrefix + "/internal/services/" + subservice
}

func workDeletionCondition(subservice string) string {
	if subservice == "" {
		return "delete transitional top-level package after CLN-WORK-FOLD-TOPLEVEL cutover proof"
	}
	return "delete public package after IMP-WORK-" + subservice + " private subservice cutover proof"
}
