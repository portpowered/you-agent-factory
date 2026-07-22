package apiserver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestValidateFactory_RoutelessCronAndLogicalMove_ReturnMissingOutputRoutesAtOutputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		body        string
		workstation string
	}{
		{
			name:        "routeless_cron",
			body:        factoryfixtures.RoutelessCronFactoryJSON,
			workstation: "cron",
		},
		{
			name:        "routeless_logical_move",
			body:        factoryfixtures.RoutelessLogicalMoveFactoryJSON,
			workstation: "router",
		},
		{
			name:        "routeless_logical_move_cron",
			body:        factoryfixtures.RoutelessLogicalMoveCronFactoryJSON,
			workstation: "trigger-monkey",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := api.NewServer(
				nil, nil, nil, nil, nil, nil, nil, nil,
				programmableFactoryValidator{
					result: factorydefinitions.ValidationResult{Targets: []factorydefinitions.ValidationTarget{
						validationTarget(
							testValidationCodeWorkstationMissingOutputRoutes,
							"workstation requires an output route",
							factorydefinitions.ValidationSubjectTypeWorkstation,
							tc.workstation,
							factorydefinitions.ValidationSubjectLocationOutputs,
						),
					}},
				},
				nil,
				nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			)
			req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("POST /factory-validations status = %d, want 200: %s", rec.Code, rec.Body.String())
			}

			result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
			assertHasValidationTarget(
				t,
				result.Targets,
				testValidationCodeWorkstationMissingOutputRoutes,
				factoryapi.FactoryValidationSubjectTypeWorkstation,
				tc.workstation,
				factoryapi.FactoryValidationSubjectLocationOutputs,
				tc.workstation+" OUTPUTS missingOutputRoutes target",
			)
			for _, target := range result.Targets {
				if target.Code == testValidationCodeWorkstationMissingFailureRoute &&
					target.Subject.Type == factoryapi.FactoryValidationSubjectTypeWorkstation &&
					target.Subject.Id == tc.workstation &&
					target.Subject.Location == factoryapi.FactoryValidationSubjectLocationOnFailure {
					t.Fatalf("targets = %#v, want no missingFailureRoute at ON_FAILURE for routeless workstation", result.Targets)
				}
			}
		})
	}
}
