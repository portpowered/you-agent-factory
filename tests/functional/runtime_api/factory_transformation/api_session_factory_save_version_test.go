package factory_transformation

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSessionFactoryPUT_UpsertCreateAllowsOmittedVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startDocumentTransformationServer(t, rootDir, "alpha")

	created := upsertNamedFactoryFromBodyForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("upsert create response version = nil, want assigned version metadata")
	}
	if created.Version.Logical.Int64() < 1 {
		t.Fatalf("upsert create version logical = %d, want >= 1", created.Version.Logical.Int64())
	}
	assertFactoryWorkType(t, created, "beta-task", "upsert create response")
}

func TestSessionFactoryPUT_UpsertReplaceRejectsStaleVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startDocumentTransformationServer(t, rootDir, "alpha")

	created := upsertNamedFactoryFromBodyForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	staleBody := upsertNamedFactoryRequestBody(currentFactorySaveDocument(t, "beta", "beta-task", versionDocument(*created.Version)))
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		server.URL(),
		factorySessionPath(server.SessionID()),
		staleBody,
		http.StatusConflict,
	)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode stale upsert replace response")
	if errResp.Code != factoryapi.ErrorResponseCodeSTALEFACTORYVERSION {
		t.Fatalf("error code = %q, want STALE_FACTORY_VERSION", errResp.Code)
	}

	reloaded := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	assertFactoryWorkType(t, reloaded, "beta-task", "unchanged factory after stale upsert replace")
}

func TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists(t *testing.T) {
	rootDir := t.TempDir()
	seedDocumentNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startDocumentTransformationServer(t, rootDir, "alpha")

	created := upsertNamedFactoryFromBodyForSession(t, server.URL(), server.SessionID(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	freshVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced := upsertNamedFactoryFromBodyForSession(
		t,
		server.URL(),
		server.SessionID(),
		currentFactorySaveDocument(t, "beta", "beta-replaced", versionDocument(freshVersion)),
	)
	if replaced.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("replaced factory name = %q, want beta", replaced.Name)
	}
	assertFactoryWorkType(t, replaced, "beta-replaced", "upsert replace response")
	if replaced.Version == nil || replaced.Version.Logical <= created.Version.Logical {
		t.Fatalf("replaced version = %#v, want logical > %#v", replaced.Version, created.Version.Logical)
	}
	current := getCurrentFactoryForSession(t, server.URL(), server.SessionID())
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}
}

func upsertNamedFactoryFromBodyForSession(
	t *testing.T,
	serverURL,
	sessionID,
	factoryBody string,
) factoryapi.Factory {
	t.Helper()
	requestBody := upsertNamedFactoryRequestBody(factoryBody)
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/"+url.PathEscape(sessionID)+"/factory",
		requestBody,
		http.StatusOK,
	)
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode session upsert named factory response")
	return created
}

func upsertNamedFactoryRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, factoryJSON)
}
