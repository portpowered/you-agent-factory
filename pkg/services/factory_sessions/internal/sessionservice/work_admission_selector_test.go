package service

import (
	"encoding/json"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// A default Factory Session is addressable as ~default and by its exact runtime
// ID. Its WORK_REQUEST events carry one of those forms, so a Work read through
// either selector must see the same admissions (and therefore payloads).
func TestWorkAdmissionsAreSelectorIndependentForDefaultSession(t *testing.T) {
	t.Parallel()

	const exactID = "8cc3988b-717c-4b3c-8710-25d0204a225e"
	session := &livesession.LiveSession{ID: "~default", RuntimeFactorySessionID: exactID}
	payload := json.RawMessage(`{"contract":"synthetic"}`)

	for _, selector := range []string{"~default", exactID} {
		for _, stamped := range []string{"~default", exactID} {
			t.Run(selector+" reads events stamped "+stamped, func(t *testing.T) {
				t.Parallel()
				ledger := &admissionProjectionLedger{}
				ledger.AppendRecordedEvent(admissionProjectionEvent(t, "event-1", stamped, 1,
					work.WorkRequestEventWork{Name: "idea", WorkID: "work-1", Payload: payload},
				))
				ledger.AppendRecordedEvent(admissionProjectionEvent(t, "event-other", "other-session", 2,
					work.WorkRequestEventWork{Name: "foreign", WorkID: "work-foreign"},
				))
				projection := newWorkAdmissionProjection(selector, platformclock.Real{})
				projection.sessionAliases = workSessionAliases(session)
				projection.Bind(ledger)

				assertAdmissions(t, projection.Snapshot(), []work.WorkAdmission{
					{WorkID: "work-1", Name: "idea", Payload: string(payload), Order: 0},
				})
			})
		}
	}
}
