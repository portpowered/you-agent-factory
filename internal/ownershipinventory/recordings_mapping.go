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
	if IsRecordingsCanonicalRetainRest(rest) {
		return retainRow(packagePath, recordingsOwner, DestinationKindOwner), true
	}
	subservice, ok := recordingsMoveSubservice(rest)
	if !ok {
		return PackageRow{}, false
	}
	return moveRow(
		packagePath,
		recordingsOwner,
		recordingsSuccessor(subservice),
		recordingsDeletionCondition(subservice),
	), true
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
