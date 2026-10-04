package support

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// WorkCustomerLocation joins a customer work type and authored state name into
// the workType:state location key used by functional observations.
func WorkCustomerLocation(workType, state string) string {
	if workType == "" || state == "" {
		return ""
	}
	return workType + ":" + state
}

// WorkItemCustomerLocation reads the customer-visible workType:state location
// from one public Work listing item.
func WorkItemCustomerLocation(item factoryapi.Work) string {
	if item.WorkTypeName == nil || item.State == nil {
		return ""
	}
	return WorkCustomerLocation(*item.WorkTypeName, item.State.Name)
}

// CountWorkAtCustomerState counts listed Work items currently occupying the
// customer workType:state location. It does not read Petri markings.
func CountWorkAtCustomerState(listed factoryapi.ListWorkResponse, location string) int {
	if location == "" {
		return 0
	}
	count := 0
	for _, item := range listed.Results {
		if WorkItemCustomerLocation(item) == location {
			count++
		}
	}
	return count
}

// HasWorkAtCustomerState reports whether one Work ID currently occupies the
// customer workType:state location in a public Work listing.
func HasWorkAtCustomerState(listed factoryapi.ListWorkResponse, workID, location string) bool {
	if workID == "" || location == "" {
		return false
	}
	for _, item := range listed.Results {
		if StringPointerValue(item.WorkId) != workID {
			continue
		}
		return WorkItemCustomerLocation(item) == location
	}
	return false
}
