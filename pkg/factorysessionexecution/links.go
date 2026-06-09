package factorysessionexecution

import "fmt"

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func InspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session:    base,
		Status:     base,
		Results:    base + "/results",
		Dispatches: base + "/dispatches",
		Artifacts:  base + "/artifacts",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}

// LifecycleControlLinksForSession builds post-control inspection links for one durable session.
func LifecycleControlLinksForSession(sessionID string, includeEvents bool) LifecycleControlLinks {
	inspection := InspectionLinksForSession(sessionID, includeEvents)
	return LifecycleControlLinks{
		Session:    inspection.Session,
		Status:     inspection.Status,
		Results:    inspection.Results,
		Dispatches: inspection.Dispatches,
		Artifacts:  inspection.Artifacts,
		Events:     inspection.Events,
	}
}
