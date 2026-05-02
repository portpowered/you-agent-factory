package functional_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestFactoryRequestBatch_PublicBatchShapeStaysAlignedAcrossWatchedFileAndHTTP(t *testing.T) {
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

type batchBoundarySummary struct {
	RequestID string
	Source    string
	Works     []batchBoundaryWork
	Relations []batchBoundaryRelation
}

type batchBoundaryWork struct {
	Name         string
	WorkID       string
	WorkTypeName string
	State        string
	TraceID      string
}

type batchBoundaryRelation struct {
	Type           string
	SourceWorkName string
	TargetWorkName string
	RequiredState  string
}

func runBoundaryBatchSmokeThroughWatchedFile(t *testing.T, batchJSON []byte, requestID string) batchBoundarySummary {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))
	testutil.WriteSeedFile(t, dir, "task", batchJSON)

	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())
	waitForGeneratedWorkIDsComplete(t, server.URL(), []string{
		"work-boundary-parent",
		"work-boundary-prerequisite",
		"work-boundary-child",
	}, 10*time.Second)

	return loadBatchBoundarySummary(t, server, requestID)
}

func runBoundaryBatchSmokeThroughHTTP(t *testing.T, batchJSON []byte, requestID string) batchBoundarySummary {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "factory_request_batch"))
	server := StartFunctionalServer(t, dir, true, factory.WithServiceMode())

	req, err := http.NewRequest(http.MethodPut, server.URL()+"/work-requests/"+requestID, bytes.NewReader(batchJSON))
	if err != nil {
		t.Fatalf("build PUT /work-requests: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /work-requests: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var body bytes.Buffer
		if _, copyErr := body.ReadFrom(resp.Body); copyErr != nil {
			t.Fatalf("read PUT /work-requests failure body: %v", copyErr)
		}
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", resp.StatusCode, body.String())
	}

	var out factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode PUT /work-requests response: %v", err)
	}
	if out.RequestId != requestID {
		t.Fatalf("PUT /work-requests request_id = %q, want %q", out.RequestId, requestID)
	}

	waitForGeneratedWorkIDsComplete(t, server.URL(), []string{
		"work-boundary-parent",
		"work-boundary-prerequisite",
		"work-boundary-child",
	}, 10*time.Second)

	return loadBatchBoundarySummary(t, server, requestID)
}

func loadBatchBoundarySummary(t *testing.T, server *FunctionalServer, requestID string) batchBoundarySummary {
	t.Helper()

	events, err := server.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest || eventString(event.Context.RequestId) != requestID {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST payload: %v", err)
		}

		summary := batchBoundarySummary{
			RequestID: requestID,
			Source:    eventString(payload.Source),
		}
		for _, work := range eventWorks(payload.Works) {
			summary.Works = append(summary.Works, batchBoundaryWork{
				Name:         work.Name,
				WorkID:       eventString(work.WorkId),
				WorkTypeName: eventString(work.WorkTypeName),
				State:        eventString(work.State),
				TraceID:      eventString(work.TraceId),
			})
		}
		for _, relation := range eventRelations(payload.Relations) {
			summary.Relations = append(summary.Relations, batchBoundaryRelation{
				Type:           string(relation.Type),
				SourceWorkName: relation.SourceWorkName,
				TargetWorkName: relation.TargetWorkName,
				RequiredState:  eventString(relation.RequiredState),
			})
		}

		sort.Slice(summary.Works, func(i, j int) bool {
			return summary.Works[i].Name < summary.Works[j].Name
		})
		sort.Slice(summary.Relations, func(i, j int) bool {
			left := summary.Relations[i]
			right := summary.Relations[j]
			if left.Type != right.Type {
				return left.Type < right.Type
			}
			if left.SourceWorkName != right.SourceWorkName {
				return left.SourceWorkName < right.SourceWorkName
			}
			return left.TargetWorkName < right.TargetWorkName
		})

		return summary
	}

	t.Fatalf("missing WORK_REQUEST event for %q", requestID)
	return batchBoundarySummary{}
}
