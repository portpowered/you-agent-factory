package bootstrap_portability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestExportImportSmoke_ExportedFactoryCanBeReimportedThroughCustomerPath(t *testing.T) {
	fixture := newExportImportFixture(t)
	harness := newExportImportSmokeHarness(fixture)

	result := harness.Run(t)

	result.AssertAPIContractSuccess(t, fixture)
	result.AssertDashboardActivationSuccess(t, fixture)

	importedResp := submitWorkAndExpectStatus(
		t,
		result.Server.URL(),
		fixture.Expected.WorkTypeName,
		"reimported-service-simple",
		http.StatusCreated,
	)
	var importedSubmit factoryapi.SubmitWorkResponse
	decodeJSONResponse(t, importedResp, &importedSubmit, "decode reimported work submit response")
	if importedSubmit.TraceId == "" {
		t.Fatal("active-factory drift: imported factory should accept work through POST /work")
	}

	legacyResp := submitWorkAndExpectStatus(
		t,
		result.Server.URL(),
		"legacy-"+fixture.Expected.WorkTypeName,
		"legacy",
		http.StatusBadRequest,
	)
	var legacyErr factoryapi.ErrorResponse
	decodeJSONResponse(t, legacyResp, &legacyErr, "decode legacy work type error response")
	if legacyErr.Code != factoryapi.BADREQUEST {
		t.Fatalf("active-factory drift: legacy work type error code = %q, want BAD_REQUEST", legacyErr.Code)
	}
}

func submitWorkAndExpectStatus(
	t *testing.T,
	serverURL, workTypeName, title string,
	wantStatus int,
) *http.Response {
	t.Helper()

	request := factoryapi.SubmitWorkRequest{
		WorkTypeName: workTypeName,
		Payload:      []byte(`{"title":"` + title + `"}`),
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal submit request: %v", err)
	}
	resp, err := http.Post(serverURL+"/work", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("POST /work status = %d, want %d", resp.StatusCode, wantStatus)
	}
	return resp
}
