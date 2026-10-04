package support

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ReadWorkAtState waits for all selected identities in one public Work snapshot.
// A DispatchResponse can arrive while its tick still holds the engine lock;
// Work reads then deliberately return the previous published snapshot. There is
// no public tick-publication acknowledgement to await on the event stream.
// Re-read through HTTP until the observable state arrives, using the caller's
// existing context as a failure ceiling, without sleeps or a stability window.
func ReadWorkAtState(ctx context.Context, endpoint, state string, workIDs ...string) (factoryapi.ListWorkResponse, error) {
	for {
		listed, err := readWorkSnapshot(ctx, endpoint)
		if err != nil {
			return listed, err
		}
		if allWorkAtState(listed, state, workIDs) {
			return listed, nil
		}
	}
}

func readWorkSnapshot(ctx context.Context, endpoint string) (factoryapi.ListWorkResponse, error) {
	var listed factoryapi.ListWorkResponse
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return listed, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return listed, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return listed, fmt.Errorf("GET Work status = %d, want 200", response.StatusCode)
	}
	err = json.NewDecoder(response.Body).Decode(&listed)
	return listed, err
}

func allWorkAtState(listed factoryapi.ListWorkResponse, state string, workIDs []string) bool {
	for _, id := range workIDs {
		if !slices.ContainsFunc(listed.Results, func(item factoryapi.Work) bool {
			return StringPointerValue(item.WorkId) == id && item.State != nil && item.State.Name == state
		}) {
			return false
		}
	}
	return true
}

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
