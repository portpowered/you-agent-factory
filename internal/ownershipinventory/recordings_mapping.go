package ownershipinventory

import "strings"

const (
	recordingsOwner         = "recordings"
	recordingsPackagePrefix = "pkg/services/recordings"
)

type recordingsMoveRule struct {
	exact      string
	prefix     string
	subservice string
}

// recordingsMoveRules mirrors cmd/packagetargetmanifestcheck nestedOwnerMoveRules
// for recordings. Ownership rows keep owner destination with concrete successor paths.
var recordingsMoveRules = []recordingsMoveRule{
	{exact: "internal/artifacts", prefix: "internal/artifacts/", subservice: "artifacts_export"},
	{exact: "internal/events", prefix: "internal/events/", subservice: "canonical_ledger"},
	{exact: "internal/projections", prefix: "internal/projections/", subservice: "projection_query"},
	{exact: "internal/replay", prefix: "internal/replay/", subservice: "replay"},
	{exact: "events", prefix: "events/", subservice: "canonical_ledger"},
	{exact: "projections", prefix: "projections/", subservice: "projection_query"},
	{exact: "replay", prefix: "replay/", subservice: "replay"},
	{exact: "artifacts", prefix: "artifacts/", subservice: "artifacts_export"},
}

func recordingsMapping(packagePath string) (PackageRow, bool) {
	if packagePath == recordingsPackagePrefix {
		return retainRow(packagePath, recordingsOwner, DestinationKindOwner), true
	}
	const prefix = recordingsPackagePrefix + "/"
	if !strings.HasPrefix(packagePath, prefix) {
		return PackageRow{}, false
	}
	rest := strings.TrimPrefix(packagePath, prefix)
	subservice, ok := recordingsMoveSubservice(rest)
	if ok {
		return moveRow(
			packagePath,
			recordingsOwner,
			recordingsSuccessor(subservice),
			recordingsDeletionCondition(subservice),
		), true
	}
	if IsRecordingsCanonicalRetainRest(rest) {
		return retainRow(packagePath, recordingsOwner, DestinationKindOwner), true
	}
	return PackageRow{}, false
}

func recordingsMoveSubservice(rest string) (subservice string, ok bool) {
	for _, rule := range recordingsMoveRules {
		if rest == rule.exact || (rule.prefix != "" && strings.HasPrefix(rest, rule.prefix)) {
			return rule.subservice, true
		}
	}
	return "", false
}

func recordingsSuccessor(subservice string) string {
	return recordingsPackagePrefix + "/internal/services/" + subservice
}

func recordingsDeletionCondition(subservice string) string {
	return "delete public package after IMP-REC-" + subservice + " private subservice cutover proof"
}
