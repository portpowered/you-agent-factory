//go:build functionallong

package runtime_api

import (
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestFactoryRequestBatch_PublicBatchShapeStaysAlignedAcrossWatchedFileAndHTTP(t *testing.T) {
	support.SkipLongFunctional(t, "slow watched-file and HTTP batch-boundary parity sweep")

	const requestID = "request-boundary-parity"

	batchJSON := []byte(`{
		"requestId": "request-boundary-parity",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "parent",
				"workId": "work-boundary-parent",
				"workTypeName": "task",
				"state": "processing",
				"traceId": "trace-boundary-parity",
				"payload": {"title": "Parent"}
			},
			{
				"name": "prerequisite",
				"workId": "work-boundary-prerequisite",
				"workTypeName": "task",
				"state": "processing",
				"payload": {"title": "Prerequisite"}
			},
			{
				"name": "child",
				"workId": "work-boundary-child",
				"workTypeName": "task",
				"payload": {"title": "Child"}
			}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": "child", "targetWorkName": "parent"},
			{"type": "DEPENDS_ON", "sourceWorkName": "child", "targetWorkName": "prerequisite"}
		]
	}`)

	expected := batchBoundarySummary{
		RequestID: requestID,
		Source:    "external-submit",
		Works: []batchBoundaryWork{
			{Name: "child", WorkID: "work-boundary-child", WorkTypeName: "task", TraceID: "trace-boundary-parity"},
			{Name: "parent", WorkID: "work-boundary-parent", WorkTypeName: "task", State: "processing", TraceID: "trace-boundary-parity"},
			{Name: "prerequisite", WorkID: "work-boundary-prerequisite", WorkTypeName: "task", State: "processing", TraceID: "trace-boundary-parity"},
		},
		Relations: []batchBoundaryRelation{
			{Type: string(factoryapi.RelationTypeDependsOn), SourceWorkName: "child", TargetWorkName: "prerequisite", RequiredState: "complete"},
			{Type: string(factoryapi.RelationTypeParentChild), SourceWorkName: "child", TargetWorkName: "parent"},
		},
	}

	watchedSummary := runBoundaryBatchSmokeThroughWatchedFile(t, batchJSON, requestID)
	httpSummary := runBoundaryBatchSmokeThroughHTTP(t, batchJSON, requestID)

	if !reflect.DeepEqual(watchedSummary, expected) {
		t.Fatalf("watched-file summary = %#v, want %#v", watchedSummary, expected)
	}
	if !reflect.DeepEqual(httpSummary, expected) {
		t.Fatalf("http summary = %#v, want %#v", httpSummary, expected)
	}
	if !reflect.DeepEqual(watchedSummary, httpSummary) {
		t.Fatalf("boundary summaries differ: watched=%#v http=%#v", watchedSummary, httpSummary)
	}
}
