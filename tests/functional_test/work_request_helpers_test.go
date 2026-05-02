package functional_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func writeWorkRequestFileForFunctionalTest(t *testing.T, path string, request interfaces.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(workRequestFromSubmitRequests([]interfaces.SubmitRequest{request}))
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func normalizeSubmitRequestsForFunctionalTest(requests []interfaces.SubmitRequest) []interfaces.SubmitRequest {
	if len(requests) == 0 {
		return nil
	}
	normalized := make([]interfaces.SubmitRequest, len(requests))
	copy(normalized, requests)
	traceID := ""
	for _, request := range normalized {
		if request.TraceID != "" {
			traceID = request.TraceID
			break
		}
	}
	if traceID == "" {
		traceID = fmt.Sprintf("trace-functional-%d", time.Now().UnixNano())
	}
	for i := range normalized {
		if normalized[i].TraceID == "" {
			normalized[i].TraceID = traceID
		}
	}
	return normalized
}

func workRequestFromSubmitRequests(requests []interfaces.SubmitRequest) interfaces.WorkRequest {
	if len(requests) == 0 {
		return interfaces.WorkRequest{Type: interfaces.WorkRequestTypeFactoryRequestBatch}
	}

	requestID := sharedSubmitRequestIDForFunctionalTest(requests)
	usedNames := make(map[string]int, len(requests))
	works := make([]interfaces.Work, 0, len(requests))
	for i, req := range requests {
		itemRequestID := req.RequestID
		if itemRequestID == "" {
			itemRequestID = requestID
		}
		works = append(works, interfaces.Work{
			Name:             uniqueSubmitWorkNameForFunctionalTest(req, i, usedNames),
			WorkID:           req.WorkID,
			RequestID:        itemRequestID,
			WorkTypeID:       req.WorkTypeID,
			State:            req.TargetState,
			TraceID:          req.TraceID,
			Payload:          append([]byte(nil), req.Payload...),
			Tags:             maps.Clone(req.Tags),
			ExecutionID:      req.ExecutionID,
			RuntimeRelations: cloneRelationsForFunctionalTest(req.Relations),
		})
	}

	return interfaces.WorkRequest{
		RequestID: requestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     works,
	}
}

func sharedSubmitRequestIDForFunctionalTest(requests []interfaces.SubmitRequest) string {
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

func uniqueSubmitWorkNameForFunctionalTest(req interfaces.SubmitRequest, index int, used map[string]int) string {
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

func cloneRelationsForFunctionalTest(relations []interfaces.Relation) []interfaces.Relation {
	if relations == nil {
		return nil
	}
	out := make([]interfaces.Relation, len(relations))
	copy(out, relations)
	return out
}
