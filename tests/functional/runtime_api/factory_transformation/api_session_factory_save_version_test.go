package factory_transformation

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestSessionFactoryPUT_UpsertCreateAllowsOmittedVersion(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startFactoryTransformationServer(t, rootDir)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
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
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startFactoryTransformationServer(t, rootDir)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	staleBody := upsertNamedFactoryRequestBody(currentFactorySaveDocument(t, "beta", "beta-task", versionDocument(*created.Version)))
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		server.URL(),
		"/factory-sessions/~default/factory",
		staleBody,
		http.StatusConflict,
	)
	var errResp factoryapi.ErrorResponse
	decodeJSONResponse(t, resp, &errResp, "decode stale upsert replace response")
	if errResp.Code != factoryapi.STALEFACTORYVERSION {
		t.Fatalf("error code = %q, want STALE_FACTORY_VERSION", errResp.Code)
	}

	reloaded := getCurrentFactory(t, server.URL())
	assertFactoryWorkType(t, reloaded, "beta-task", "unchanged factory after stale upsert replace")
}

func TestSessionFactoryPUT_UpsertReplaceDoesNotReturnAlreadyExists(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")
	server := startFactoryTransformationServer(t, rootDir)

	created := upsertNamedFactoryFromBody(t, server.URL(), functionalNamedFactoryBody("beta", "beta-task"))
	if created.Version == nil {
		t.Fatal("created factory version = nil, want version metadata")
	}

	freshVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  created.Version.Logical + 1,
		Physical: created.Version.Physical.Add(time.Second),
	}
	replaced := upsertNamedFactoryFromBody(
		t,
		server.URL(),
		currentFactorySaveDocument(t, "beta", "beta-replaced", versionDocument(freshVersion)),
	)
	if replaced.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("replaced factory name = %q, want beta", replaced.Name)
	}
	assertFactoryWorkType(t, replaced, "beta-replaced", "upsert replace response")
	if replaced.Version == nil || replaced.Version.Logical <= created.Version.Logical {
		t.Fatalf("replaced version = %#v, want logical > %#v", replaced.Version, created.Version.Logical)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta")
}

func upsertNamedFactoryFromBody(t *testing.T, serverURL, factoryBody string) factoryapi.Factory {
	t.Helper()
	requestBody := upsertNamedFactoryRequestBody(factoryBody)
	resp := putFactoryForSessionRequestExpectStatusWithClient(
		t,
		http.DefaultClient,
		serverURL,
		"/factory-sessions/~default/factory",
		requestBody,
		http.StatusOK,
	)
	var created factoryapi.Factory
	decodeJSONResponse(t, resp, &created, "decode upsert named factory response")
	return created
}

func upsertNamedFactoryRequestBody(factoryJSON string) string {
	return fmt.Sprintf(`{"mode":"UPSERT_NAMED_AND_ACTIVATE","factory":%s}`, factoryJSON)
}
