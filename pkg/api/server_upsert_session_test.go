package api

import (
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestUpsertWorkRequestBySessionId_Returns201AndSubmitsToSessionFactory(t *testing.T) {
	defaultFactory := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	sessionFactory := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}, Net: sessionScopedStateNet()}
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		SessionFactories: map[string]*testutil.MockFactory{
			"~default":      defaultFactory,
			"session-alpha": sessionFactory,
		},
	})

	rec := upsertWorkRequest(t, srv, "/factory-sessions/session-alpha/work-requests/request-scoped-upsert", `{
		"requestId":"request-scoped-upsert",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"scoped-batch","workTypeName":"task","traceId":"trace-scoped-upsert","payload":{"title":"Scoped upsert"}}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /factory-sessions/session-alpha/work-requests/request-scoped-upsert status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
	if resp.RequestId != "request-scoped-upsert" || resp.TraceId == "" {
		t.Fatalf("upsert response = %#v, want request and trace", resp)
	}
	if len(resp.Works) != 1 || resp.Works[0].Name != "scoped-batch" || resp.Works[0].WorkTypeName != "task" {
		t.Fatalf("upsert works = %#v, want scoped-batch task", resp.Works)
	}
	if len(sessionFactory.WorkRequests) != 1 || len(sessionFactory.Submitted) != 1 {
		t.Fatalf("session submissions = workRequests:%d submitted:%d, want 1/1", len(sessionFactory.WorkRequests), len(sessionFactory.Submitted))
	}
	if sessionFactory.Submitted[0].RequestID != "request-scoped-upsert" || sessionFactory.Submitted[0].Name != "scoped-batch" {
		t.Fatalf("session submitted request = %#v, want scoped upsert metadata", sessionFactory.Submitted[0])
	}
	if len(defaultFactory.WorkRequests) != 0 || len(defaultFactory.Submitted) != 0 {
		t.Fatalf("default submissions = workRequests:%d submitted:%d, want 0/0", len(defaultFactory.WorkRequests), len(defaultFactory.Submitted))
	}
}

func TestUpsertWorkRequestBySessionId_UnknownSessionReturnsNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}},
		},
	})

	rec := upsertWorkRequest(t, srv, "/factory-sessions/missing-session/work-requests/request-missing-session", `{
		"requestId":"request-missing-session",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"draft","workTypeName":"task","payload":{"title":"Draft"}}]
	}`)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "factory session not found")
}
