package factorysessionexecution

import "fmt"

// InspectionLinksForSession builds API-relative inspection links for one durable session.
func InspectionLinksForSession(sessionID string, includeEvents bool) InspectionLinks {
	base := fmt.Sprintf("/factory-sessions/%s", sessionID)
	links := InspectionLinks{
		Session: base,
		Status:  base,
		Results: base + "/results",
	}
	if includeEvents {
		links.Events = base + "/events"
	}
	return links
}
