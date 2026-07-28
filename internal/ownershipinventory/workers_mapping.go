package ownershipinventory

import "strings"

const (
	workersOwner         = "workers"
	workersPackagePrefix = "pkg/services/workers"
)

type workersMoveRule struct {
	exact      string
	prefix     string
	subservice string // empty means owner/internal root
}

// workersMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules
// for workers transitional debt. Ownership rows keep owner destination with
// concrete successor paths.
var workersMoveRules = []workersMoveRule{
	{exact: "construction", prefix: "construction/", subservice: "runtime_assembly"},
	{exact: "diagnostics", prefix: "diagnostics/"},
	{exact: "interface", prefix: "interface/"},
	{exact: "execution", prefix: "execution/", subservice: "workstations"},
	{exact: "executor", prefix: "executor/", subservice: "workstations"},
	{exact: "invocation", prefix: "invocation/", subservice: "workstations"},
	{exact: "prompting", prefix: "prompting/", subservice: "workstations"},
	{exact: "skippermissions", prefix: "skippermissions/", subservice: "workstations"},
	{exact: "worktree", prefix: "worktree/", subservice: "workstations"},
	{exact: "process", prefix: "process/", subservice: "runners"},
	{exact: "runner", prefix: "runner/", subservice: "runners"},
	{exact: "services/inference", prefix: "services/inference/", subservice: "runners"},
	{exact: "services/testing", prefix: "services/testing/", subservice: "runners"},
	{exact: "services", prefix: "services/", subservice: "runners"},
}

func workersMapping(packagePath string) (PackageRow, bool) {
	if packagePath == workersPackagePrefix {
		return retainRow(packagePath, workersOwner, DestinationKindOwner), true
	}
	const prefix = workersPackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	if isWorkersCanonicalRetain(rest) {
		return retainRow(packagePath, workersOwner, DestinationKindOwner), true
	}
	subservice, ok := workersMoveSubservice(rest)
	if !ok {
		subservice = ""
	}
	return moveRow(
		packagePath,
		workersOwner,
		workersSuccessor(subservice),
		workersDeletionCondition(subservice, rest),
	), true
}

func isWorkersCanonicalRetain(rest string) bool {
	switch {
	case rest == "wire" || strings.HasPrefix(rest, "wire/"):
		return true
	case strings.HasPrefix(rest, "internal/services/runtime_assembly"):
		return true
	case strings.HasPrefix(rest, "internal/services/runners"):
		return true
	case strings.HasPrefix(rest, "internal/services/workstations"):
		return true
	default:
		return false
	}
}

func workersMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range workersMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func workersSuccessor(subservice string) string {
	if subservice == "" {
		return workersPackagePrefix + "/internal"
	}
	return workersPackagePrefix + "/internal/services/" + subservice
}

func workersDeletionCondition(subservice, rest string) string {
	if subservice == "" {
		return "delete transitional top-level package after CLN-WRK-FOLD-TOPLEVEL cutover proof"
	}
	return "delete public package after IMP-WRK-" + subservice + " private subservice cutover proof"
}
