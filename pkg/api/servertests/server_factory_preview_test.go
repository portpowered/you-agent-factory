package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestPreviewFactory_ReturnsSharedFactoryPreviewContract(t *testing.T) {
	projectRoot := t.TempDir()
	writeWorkflowPreviewFixture(t, projectRoot, "review.js", validWorkflowPreviewSource)

	body, err := json.Marshal(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.FactoryPreviewRequestSourceKindWORKFLOWNAME,
		ProjectRoot: stringPtr(projectRoot),
		SourceValue: stringPtr("review"),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	srv := newAPITestServer(nil)
	req := httptest.NewRequest(http.MethodPost, "/factories/preview", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryPreviewResult](t, rec)
	if !result.Valid {
		t.Fatalf("result = %#v, want valid preview", result)
	}
	if result.SourceResolution.SourceHash == nil || *result.SourceResolution.SourceHash == "" {
		t.Fatal("expected source hash")
	}
	if result.PolicyPreview.PolicyHash == "" {
		t.Fatal("expected policy hash")
	}
	if result.ResultConstraints.ArtifactUriScheme != "you-artifact" {
		t.Fatalf("artifact scheme = %q, want you-artifact", result.ResultConstraints.ArtifactUriScheme)
	}
}
