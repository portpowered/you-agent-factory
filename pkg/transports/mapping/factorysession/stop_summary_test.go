package factorysession_test

import (
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

func TestStopSummaryToAPIOnlyConvertsDetachedOwnerResult(t *testing.T) {
	lifecycle, workID, workstation := "PAUSED", "work-1", "review"
	summary := &factorysessions.StopSummary{
		SessionID: "session-1", StopKind: factorysessions.StopKindPaused,
		SessionLifecycleStatus: &lifecycle, WorkID: &workID,
		LatestDispatch: &factorysessions.StopDispatchSummary{
			DispatchID: "dispatch-1", Status: factorysessions.StopDispatchStatusInterrupted,
			DispatchKind: "JAVASCRIPT_AGENT", WorkstationName: &workstation,
			FailureDetail: &factorysessions.StopFailureDetail{Reason: "timeout", Message: "provider timeout"},
		},
	}

	mapped := factorysessionmapping.StopSummaryToAPI(summary)
	if mapped == nil || mapped.StopKind != factoryapi.PAUSED || mapped.WorkId == nil || *mapped.WorkId != workID {
		t.Fatalf("mapped summary = %#v", mapped)
	}
	if mapped.LatestDispatch == nil || mapped.LatestDispatch.Status != factoryapi.FactoryDispatchStatusINTERRUPTED || mapped.LatestDispatch.FailureDetail == nil || mapped.LatestDispatch.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("mapped dispatch = %#v", mapped.LatestDispatch)
	}

	detached := factorysessionmapping.StopSummaryFromAPI(mapped)
	if detached == nil || detached.StopKind != factorysessions.StopKindPaused || detached.LatestDispatch == nil || detached.LatestDispatch.Status != factorysessions.StopDispatchStatusInterrupted {
		t.Fatalf("detached summary = %#v", detached)
	}
}
