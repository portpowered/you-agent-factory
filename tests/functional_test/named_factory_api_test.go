package functional_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

func TestNamedFactoryAPI_PersistsActivatesAndSwitchesWorkSurface(t *testing.T) {
	rootDir := t.TempDir()
	seedNamedFactoryRoot(t, rootDir, "alpha", "alpha-task")

	server := startNamedFactoryTestServer(t, rootDir)

	created := createNamedFactoryFromBody(t, server.httpSrv.URL, "beta", "beta-task")
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertNamedFactoryCurrentPointer(t, rootDir, "beta")

	current := getNamedFactoryCurrent(t, server.httpSrv.URL)
	if current.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("current factory name = %q, want beta", current.Name)
	}

	betaResp := submitWorkAndExpectStatus(t, server.httpSrv.URL, "beta-task", "beta", http.StatusCreated)
	var betaSubmit factoryapi.SubmitWorkResponse
	decodeNamedFactoryJSONResponse(t, betaResp, &betaSubmit, "decode beta-task submit response")
	if betaSubmit.TraceId == "" {
		t.Fatal("expected non-empty trace ID for activated beta-task submission")
	}

	legacyResp := submitWorkAndExpectStatus(t, server.httpSrv.URL, "alpha-task", "alpha", http.StatusBadRequest)
	var legacyErr factoryapi.ErrorResponse
	decodeNamedFactoryJSONResponse(t, legacyResp, &legacyErr, "decode alpha-task error response")
	if legacyErr.Code != factoryapi.BADREQUEST {
		t.Fatalf("alpha-task error code = %q, want BAD_REQUEST", legacyErr.Code)
	}
}

func seedNamedFactoryRoot(t *testing.T, rootDir, name, workType string) {
	t.Helper()

	if _, err := config.PersistNamedFactory(rootDir, name, functionalNamedFactoryPayloadWithWorkType(t, name, workType)); err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(%s): %v", name, err)
	}
}

func startNamedFactoryTestServer(t *testing.T, rootDir string) *FunctionalServer {
	t.Helper()

	return StartFunctionalServerWithConfig(t, rootDir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.NewNop()
	})
}

func createNamedFactoryFromBody(t *testing.T, serverURL, name, workType string) factoryapi.NamedFactory {
	t.Helper()

	var request factoryapi.NamedFactory
	if err := json.Unmarshal([]byte(functionalNamedFactoryBody(name, workType)), &request); err != nil {
		t.Fatalf("unmarshal named factory request: %v", err)
	}
	return createNamedFactory(t, serverURL, request)
}

func getNamedFactoryCurrent(t *testing.T, serverURL string) factoryapi.NamedFactory {
	t.Helper()
	return getCurrentNamedFactory(t, serverURL)
}

func submitWorkAndExpectStatus(t *testing.T, serverURL, workType, title string, wantStatus int) *http.Response {
	t.Helper()

	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewBufferString(`{"workTypeName":"`+workType+`","payload":{"title":"`+title+`"}}`))
	if err != nil {
		t.Fatalf("POST /work %s: %v", workType, err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work %s status = %d, want %d", workType, resp.StatusCode, wantStatus)
	}
	return resp
}

func decodeNamedFactoryJSONResponse(t *testing.T, resp *http.Response, target any, message string) {
	t.Helper()
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertNamedFactoryCurrentPointer(t *testing.T, rootDir, want string) {
	t.Helper()

	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	}
	if got != want {
		t.Fatalf("current factory pointer = %q, want %q", got, want)
	}
}

func functionalNamedFactoryPayloadWithWorkType(t *testing.T, project, workType string) []byte {
	t.Helper()
	return []byte(functionalNamedFactoryPayloadJSON(project, workType))
}

func functionalNamedFactoryBody(name, workType string) string {
	return `{"name":"` + name + `","factory":` + functionalNamedFactoryPayloadJSON(name, workType) + `}`
}

func functionalNamedFactoryPayloadJSON(project, workType string) string {
	return `{
		"project":"` + project + `",
		"workTypes":[{
			"name":"` + workType + `",
			"states":[
				{"name":"init","type":"INITIAL"},
				{"name":"done","type":"TERMINAL"},
				{"name":"failed","type":"FAILED"}
			]
		}],
		"workers":[{
			"name":"planner",
			"type":"MODEL_WORKER",
			"modelProvider":"claude",
			"executorProvider":"script_wrap",
			"model":"claude-sonnet-4-20250514"
		}],
		"workstations":[{
			"name":"plan-task",
			"kind":"STANDARD",
			"type":"MODEL_WORKSTATION",
			"worker":"planner",
			"inputs":[{"workType":"` + workType + `","state":"init"}],
			"outputs":[{"workType":"` + workType + `","state":"done"}]
		}]
	}`
}
