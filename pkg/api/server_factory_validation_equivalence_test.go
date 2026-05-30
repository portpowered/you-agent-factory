package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestFactoryValidation_EquivalentCanonicalTargetsAcrossPackageConfigAndAPIPaths(t *testing.T) {
	t.Parallel()

	factory, err := factoryvalidation.DecodeCrossPathInvalidFactory()
	if err != nil {
		t.Fatalf("DecodeCrossPathInvalidFactory: %v", err)
	}
	cfg, err := config.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}

	explicit := factoryvalidation.Validate(&cfg)
	packageSignatures := factoryvalidation.CanonicalTargetSignatures(explicit.Targets)

	configFindings := config.CanonicalStructuralFindings(&cfg)
	if len(configFindings) != len(explicit.Targets) {
		t.Fatalf("config findings = %d, package targets = %d, want equivalent coverage",
			len(configFindings), len(explicit.Targets))
	}
	for index, target := range explicit.Targets {
		if configFindings[index].Rule != target.Code {
			t.Fatalf("config finding[%d].Rule = %q, want package target code %q",
				index, configFindings[index].Rule, target.Code)
		}
	}

	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(
		http.MethodPost,
		"/factory-validations",
		bytes.NewBufferString(factoryvalidation.CrossPathInvalidFactoryJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	apiSignatures := factoryvalidation.CanonicalAPITargetSignatures(result.Targets)
	if !factoryvalidation.EquivalentCanonicalTargetSignatures(packageSignatures, apiSignatures) {
		t.Fatalf("package signatures = %#v, api signatures = %#v, want equivalent canonical targets",
			packageSignatures, apiSignatures)
	}
}
