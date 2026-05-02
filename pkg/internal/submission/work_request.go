package submission

import (
	"fmt"
	"maps"
	"strconv"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// WorkRequestFromSubmitRequests wraps internal normalized submit records in the
// canonical FACTORY_REQUEST_BATCH contract.
func WorkRequestFromSubmitRequests(requests []interfaces.SubmitRequest) interfaces.WorkRequest {
	if len(requests) == 0 {
		return interfaces.WorkRequest{Type: interfaces.WorkRequestTypeFactoryRequestBatch}
	}

	requestID := sharedSubmitRequestID(requests)
	usedNames := make(map[string]int, len(requests))
	works := make([]interfaces.Work, 0, len(requests))
	for i, req := range requests {
		itemRequestID := req.RequestID
		if itemRequestID == "" {
			itemRequestID = requestID
		}
		currentChainingTraceID := factory.ResolveWorkRequestCurrentChainingTraceID(req.CurrentChainingTraceID, req.TraceID)
		works = append(works, interfaces.Work{
			Name:                     uniqueSubmitWorkName(req, i, usedNames),
			WorkID:                   req.WorkID,
			RequestID:                itemRequestID,
			WorkTypeID:               req.WorkTypeID,
			State:                    req.TargetState,
			CurrentChainingTraceID:   currentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), req.PreviousChainingTraceIDs...),
			TraceID:                  req.TraceID,
			Payload:                  append([]byte(nil), req.Payload...),
			Tags:                     maps.Clone(req.Tags),
			ExecutionID:              req.ExecutionID,
			RuntimeRelations:         clonePetriRelations(req.Relations),
		})
	}
	currentChainingTraceID := factory.ResolveWorkRequestCurrentChainingTraceID(requests[0].CurrentChainingTraceID, requests[0].TraceID)

	return interfaces.WorkRequest{
		RequestID:              requestID,
		CurrentChainingTraceID: currentChainingTraceID,
		Type:                   interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:                  works,
	}
}

func sharedSubmitRequestID(requests []interfaces.SubmitRequest) string {
	var shared string
	for _, req := range requests {
		if req.RequestID == "" {
			continue
		}
		if shared == "" {
			shared = req.RequestID
			continue
		}
		if shared != req.RequestID {
			return ""
		}
	}
	return shared
}

func uniqueSubmitWorkName(req interfaces.SubmitRequest, index int, used map[string]int) string {
	base := req.Name
	if req.Tags != nil && req.Tags["_work_name"] != "" {
		base = req.Tags["_work_name"]
	}
	if base == "" {
		base = req.WorkID
	}
	if base == "" {
		base = "work-" + strconv.Itoa(index+1)
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func clonePetriRelations(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.Relation, len(relations))
	copy(out, relations)
	return out
}
