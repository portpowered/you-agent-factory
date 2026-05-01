package functional_test

import (
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
