package api

import (
	"errors"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestUpsertWorkRequest_NormalizesLegacyStringPayloadIntoCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","payload":"legacy text"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 1 || mf.Submitted[0].Content[0].Text != "legacy text" {
		t.Fatalf("submitted content = %#v, want canonical text content", mf.Submitted)
	}
}

func TestUpsertWorkRequest_RejectsInvalidContentPartShape(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","file":"wrong"}]}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].content[0].file is not supported")
}

func TestUpsertWorkRequest_FirstSubmitAndRepeatedRequestID(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	var firstTraceID string
	for i, body := range []string{
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","traceId":"trace-original","payload":{"title":"Draft"}}]}`,
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"changed-draft","workTypeName":"task","traceId":"trace-retry","payload":{"title":"Changed retry"}}]}`,
	} {
		rec := upsertWorkRequest(t, srv, "/work-requests/request-api-1", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
		if resp.RequestId != "request-api-1" || resp.TraceId == "" {
			t.Fatalf("upsert response = %#v, want request and trace", resp)
		}
		if i == 0 {
			firstTraceID = resp.TraceId
		} else if resp.TraceId != firstTraceID {
			t.Fatalf("repeated trace_id = %q, want original %q", resp.TraceId, firstTraceID)
		}
	}

	if len(mf.WorkRequests) != 1 || len(mf.Submitted) != 1 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 1/1", len(mf.WorkRequests), len(mf.Submitted))
	}
	if mf.Submitted[0].RequestID != "request-api-1" || mf.Submitted[0].TraceID != "trace-original" || mf.Submitted[0].Name != "draft" {
		t.Fatalf("submitted request = %#v, want original request metadata", mf.Submitted[0])
	}
}

func TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-batch", `{
		"requestId":"request-api-batch",
		"currentChainingTraceId":"chain-request-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"draft","workTypeName":"task","state":"queued","currentChainingTraceId":"chain-draft","traceId":"chain-draft","payload":{"title":"Draft"}},
			{"name":"review","workTypeName":"review","payload":"review draft"}
		],
		"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"complete"}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	submittedRequest := mf.WorkRequests[0]
	if len(mf.WorkRequests) != 1 || len(submittedRequest.Works) != 2 {
		t.Fatalf("work request submissions = %#v, want one request with two works", mf.WorkRequests)
	}
	if submittedRequest.CurrentChainingTraceID != "chain-request-batch" || submittedRequest.Works[0].CurrentChainingTraceID != "chain-draft" || submittedRequest.Works[1].CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("work request chaining traces = %#v", submittedRequest)
	}
	if submittedRequest.Works[0].WorkTypeID != "task" || submittedRequest.Works[1].WorkTypeID != "review" || submittedRequest.Works[0].State != "queued" {
		t.Fatalf("domain works = %#v, want task/review and queued draft", submittedRequest.Works)
	}
	if len(submittedRequest.Relations) != 1 || submittedRequest.Relations[0].SourceWorkName != "review" || submittedRequest.Relations[0].TargetWorkName != "draft" {
		t.Fatalf("domain relation = %#v, want review depends on draft", submittedRequest.Relations)
	}
	if len(mf.Submitted) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(mf.Submitted))
	}
	relation := mf.Submitted[1].Relations[0]
	if relation.TargetWorkID != "batch-request-api-batch-draft" || relation.RequiredState != "complete" {
		t.Fatalf("normalized relation = %#v, want dependency on draft completion", relation)
	}
}

func TestUpsertWorkRequest_AcceptsParentChildRelationsByWorkName(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-parent-child", `{
		"requestId":"request-api-parent-child",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"parent","workTypeName":"task","traceId":"trace-parent-child","payload":{"title":"Parent"}},
			{"name":"prerequisite","workTypeName":"task","payload":{"title":"Prerequisite"}},
			{"name":"child","workTypeName":"task","payload":{"title":"Child"}}
		],
		"relations":[
			{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},
			{"type":"DEPENDS_ON","sourceWorkName":"child","targetWorkName":"prerequisite"}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Relations) != 2 || mf.WorkRequests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("work request relations = %#v, want parent-child plus dependency", mf.WorkRequests)
	}
	child := submittedRequestNamed(t, mf.Submitted, "child")
	if child.TraceID != "trace-parent-child" || len(child.Relations) != 2 {
		t.Fatalf("normalized child = %#v, want inherited trace and relations", child)
	}
	assertSubmittedChildRelations(t, child.Relations)
}

func TestUpsertWorkRequest_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-legacy", `{"requestId":"request-api-legacy","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","work_type_id":"legacy-task","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].work_type_id is not supported; use workTypeName")
}

func TestUpsertWorkRequest_TargetStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-state-alias", `{"requestId":"request-api-state-alias","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","target_state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].target_state is not supported; use state")
}

func TestUpsertWorkRequest_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-chaining-conflict", `{"requestId":"request-api-chaining-conflict","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","currentChainingTraceId":"chain-a","traceId":"trace-b","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].currentChainingTraceId and traceId must match when both are provided")
}

func TestUpsertWorkRequest_InvalidExplicitStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net: &state.Net{WorkTypes: map[string]*state.WorkType{
			"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}, {Value: "complete", Category: state.StateCategoryTerminal}}},
		}},
	}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-invalid-state", `{"requestId":"request-api-invalid-state","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("draft") references unknown state "queued" for work type name "task"`)
}

func TestUpsertWorkRequestValidationFailures(t *testing.T) {
	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "invalid_json", path: "/work-requests/request-api-1", body: `{"requestId":`, wantMsg: "invalid request payload"},
		{name: "missing_required_request_id", path: "/work-requests/request-api-1", body: `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "requestId is required"},
		{name: "path_body_mismatch", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-2","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "request_id path and requestId body must match"},
		{name: "cycle_error", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"},{"name":"b","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"b"},{"type":"DEPENDS_ON","sourceWorkName":"b","targetWorkName":"a"}]}`, wantMsg: `work_request: dependency cycle detected involving "a"`},
		{name: "malformed_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"missing"}]}`, wantMsg: `work_request: relations[0] references unknown targetWorkName "missing"`},
		{name: "self_parenting_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"a","targetWorkName":"a"}]}`, wantMsg: `work_request: relations[0] has self-parenting on "a"`},
	})

	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "duplicate_parent_child_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"parent","workTypeName":"task"},{"name":"child","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"}]}`, wantMsg: `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`},
		{name: "missing_work_type_name", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft"}]}`, wantMsg: `work_request: works[0] ("draft") is missing workTypeName`},
		{name: "work_type_id_not_supported", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","work_type_id":"legacy-task"}]}`, wantMsg: `works[0].work_type_id is not supported; use workTypeName`},
		{name: "unknown_work_type", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"unknown"}]}`, factory: &testutil.MockFactory{SubmitWorkRequestErr: errors.New(`work_request: works[0] ("draft") references unknown work type "unknown"`)}, wantMsg: `work_request: works[0] ("draft") references unknown work type name "unknown"`},
		{
			name: "invalid_dependency_required_state",
			path: "/work-requests/request-api-1",
			body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"},{"name":"review","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"queued"}]}`,
			factory: &testutil.MockFactory{
				Net: &state.Net{
					WorkTypes: map[string]*state.WorkType{
						"task": {
							ID: "task",
							States: []state.StateDefinition{
								{Value: "init", Category: state.StateCategoryInitial},
								{Value: "complete", Category: state.StateCategoryTerminal},
							},
						},
					},
				},
			},
			wantMsg: `work_request: relations[0] references unknown requiredState "queued" for target work type name "task"`,
		},
	})
}

type upsertValidationFailureCase struct {
	name    string
	path    string
	body    string
	factory *testutil.MockFactory
	wantMsg string
}

func runUpsertValidationFailureCases(t *testing.T, cases []upsertValidationFailureCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mf := tc.factory
			if mf == nil {
				mf = &testutil.MockFactory{}
			}
			mf.Marking = &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}
			srv := newTestServer(mf)

			rec := upsertWorkRequest(t, srv, tc.path, tc.body)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", tc.wantMsg)
			if len(mf.Submitted) != 0 {
				t.Fatalf("submitted count = %d, want 0", len(mf.Submitted))
			}
		})
	}
}
