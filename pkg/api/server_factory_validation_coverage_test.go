package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestValidateFactory_OperationalCronRulesRemainConfigScoped(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})

	body := `{
		"name":"alpha",
		"workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"w1"}],
		"workstations":[{
			"name":"daily-refresh",
			"behavior":"CRON",
			"worker":"w1",
			"outputs":[{"workType":"task","state":"init"}]
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	for _, target := range result.Targets {
		if target.Code == "cron-config" || target.Code == "cron-schedule" {
			t.Fatalf("validation-only API targets = %#v, want operational cron rules to stay config-scoped", result.Targets)
		}
	}

	cfg := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Workers: []interfaces.WorkerConfig{{Name: "w1"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "daily-refresh",
			Kind:           interfaces.WorkstationKindCron,
			WorkerTypeName: "w1",
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
		}},
	}
	configFindings := config.NewConfigValidator().Validate(cfg).Findings
	assertConfigFindingRule(t, configFindings, "cron-config")
}

func TestValidateFactory_PreservesCanonicalStructuralCodesFromCrossPathFixture(t *testing.T) {
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
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
	wantCodes := []string{
		factoryvalidation.CodeDuplicateIdentifier,
		factoryvalidation.CodeDanglingWorkerReference,
		factoryvalidation.CodeDanglingPlaceReference,
		factoryvalidation.CodeWorkstationMissingFailureRoute,
		factoryvalidation.CodeWorkTypeMissingCompletionState,
		factoryvalidation.CodeWorkTypeMissingFailureState,
		factoryvalidation.CodeWorkStateMissingTerminalPath,
	}
	for _, code := range wantCodes {
		assertHasValidationTargetCode(t, result.Targets, code)
	}
}

func assertConfigFindingRule(t *testing.T, findings []config.Finding, rule string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Rule == rule {
			return
		}
	}
	t.Fatalf("expected config finding rule %q, got %#v", rule, findings)
}
