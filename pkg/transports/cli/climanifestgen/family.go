package climanifestgen

import (
	"fmt"
	"slices"
)

// RepresentativeFamilyCommandIDs are the only stable command IDs the generator
// may emit for the B10 representative root/session-show cutover family.
var RepresentativeFamilyCommandIDs = []string{
	"you",
	"you.session",
	"you.session.show",
}

// SessionFamilyCommandIDs are the stable command IDs emitted for the complete
// canonical Factory Session command family. The root command remains owned by
// the representative family until the production cutover is widened.
var SessionFamilyCommandIDs = []string{
	"you.session",
	"you.session.create",
	"you.session.list",
	"you.session.show",
	"you.session.delete",
	"you.session.pause",
	"you.session.resume",
	"you.session.dispatches",
}

// IsSessionFamilyCommandID reports whether id belongs to the canonical session family.
func IsSessionFamilyCommandID(id string) bool {
	return slices.Contains(SessionFamilyCommandIDs, id)
}

// AssertSessionFamilyCommandID rejects command IDs outside the canonical session family.
func AssertSessionFamilyCommandID(id string) error {
	if IsSessionFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf("command id %q is outside the session family %v", id, SessionFamilyCommandIDs)
}

// IsRepresentativeFamilyCommandID reports whether id belongs to the representative family.
func IsRepresentativeFamilyCommandID(id string) bool {
	return slices.Contains(RepresentativeFamilyCommandIDs, id)
}

// AssertRepresentativeFamilyCommandID returns an error when id is outside the
// representative family scope.
func AssertRepresentativeFamilyCommandID(id string) error {
	if IsRepresentativeFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the representative family %v",
		id,
		RepresentativeFamilyCommandIDs,
	)
}

// WorkFamilyCommandIDs are the only stable command IDs the generator may emit
// for the work inspection/control family cutover slice.
var WorkFamilyCommandIDs = []string{
	"you.work",
	"you.work.list",
	"you.work.show",
	"you.work.move",
	"you.work.visualize",
}

// WorkersFamilyCommandIDs are the stable command IDs for customer-facing
// worker integration management.
var WorkersFamilyCommandIDs = []string{
	"you.workers",
	"you.workers.list",
	"you.workers.acp",
	"you.workers.acp.add",
	"you.workers.acp.delete",
}

// RunSubmitFamilyCommandIDs are the only stable command IDs the generator may
// emit for the run and submit invocation family.
var RunSubmitFamilyCommandIDs = []string{
	"you.run",
	"you.server",
	"you.submit",
	"you.submit.batch",
}

// IsRunSubmitFamilyCommandID reports whether id belongs to the run/submit family.
func IsRunSubmitFamilyCommandID(id string) bool {
	return slices.Contains(RunSubmitFamilyCommandIDs, id)
}

// AssertRunSubmitFamilyCommandID returns an error when id is outside the
// run/submit family scope.
func AssertRunSubmitFamilyCommandID(id string) error {
	if IsRunSubmitFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the run/submit family %v",
		id,
		RunSubmitFamilyCommandIDs,
	)
}

// IsWorkFamilyCommandID reports whether id belongs to the work family.
func IsWorkFamilyCommandID(id string) bool {
	return slices.Contains(WorkFamilyCommandIDs, id)
}

// AssertWorkFamilyCommandID returns an error when id is outside the work
// family scope.
func AssertWorkFamilyCommandID(id string) error {
	if IsWorkFamilyCommandID(id) {
		return nil
	}
	return fmt.Errorf(
		"command id %q is outside the work family %v",
		id,
		WorkFamilyCommandIDs,
	)
}
