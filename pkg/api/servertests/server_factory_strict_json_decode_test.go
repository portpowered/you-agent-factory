package apiserver_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

type factoryStrictJSONDecodeCase struct {
	name        string
	body        string
	wantStatus  int
	wantCode    string
	wantMessage string
	wantTargets bool
}

func TestFactoryStrictJSONDecode_ValidateFactory(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{})
	cases := []factoryStrictJSONDecodeCase{
		{
			name:        "valid",
			body:        validNamedFactoryBody("beta", "beta-task"),
			wantStatus:  http.StatusOK,
			wantCode:    "",
			wantMessage: "",
		},
		{
			name:        "unknown_field",
			body:        `{"name":"beta","unknownExtra":1}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "malformed",
			body:        `{"name":"beta"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "multi_object",
			body:        `{"name":"beta"}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/factory-validations", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if tc.wantStatus == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				result := decodeJSONResponse[factoryapi.FactoryValidationResult](t, rec)
				if len(result.Targets) != 0 {
					t.Fatalf("targets = %#v, want empty slice", result.Targets)
				}
				return
			}
			assertFactoryDecodeJSONError(t, rec, tc.wantStatus, tc.wantCode, tc.wantMessage, tc.wantTargets)
		})
	}
}

func TestFactoryStrictJSONDecode_OpenFactorySession(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{})
	cases := []factoryStrictJSONDecodeCase{
		{
			name:       "valid",
			body:       `{"folderPath":"/workspace/fleet","validateOnly":true}`,
			wantStatus: http.StatusOK,
		},
		{
			name:        "unknown_field",
			body:        `{"folderPath":"/workspace/fleet","unknownExtra":1}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "malformed",
			body:        `{"folderPath":"/workspace/fleet"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "multi_object",
			body:        `{"folderPath":"/workspace/fleet"}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPost, "/factory-sessions", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if tc.wantStatus == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertFactoryDecodeJSONError(t, rec, tc.wantStatus, tc.wantCode, tc.wantMessage, tc.wantTargets)
		})
	}
}

func TestFactoryStrictJSONDecode_SaveCurrentFactoryBySessionId(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{})
	cases := []factoryStrictJSONDecodeCase{
		{
			name:       "valid",
			body:       saveFactoryForSessionRequestBody(validNamedFactoryBody("beta", "beta-task")),
			wantStatus: http.StatusOK,
		},
		{
			name:        "unknown_field",
			body:        `{"factory":{"name":"beta"},"unknownExtra":1}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
			wantTargets: true,
		},
		{
			name:        "malformed",
			body:        `{"factory":{"name":"beta"`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
			wantTargets: true,
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
			wantTargets: true,
		},
		{
			name:        "multi_object",
			body:        `{"factory":{"name":"beta"}}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
			wantTargets: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPut, "/factory-sessions/~default/factory", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if tc.wantStatus == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertFactoryDecodeJSONError(t, rec, tc.wantStatus, tc.wantCode, tc.wantMessage, tc.wantTargets)
		})
	}
}

func TestFactoryStrictJSONDecode_PromptTemplateValidation(t *testing.T) {
	t.Parallel()

	srv := newAPITestServer(&testutil.MockFactory{
		CurrentFactory: &factoryapi.Factory{
			Name: "beta",
			Workstations: &[]factoryapi.Workstation{{
				Name:    "Review",
				Worker:  "reviewer",
				Inputs:  []factoryapi.WorkstationIO{{State: "queued", WorkType: "task"}},
				Outputs: &[]factoryapi.WorkstationIO{{State: "reviewed", WorkType: "task"}},
			}},
		},
	})
	cases := []factoryStrictJSONDecodeCase{
		{
			name:       "valid",
			body:       `{"prompt":"hello"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:        "unknown_field",
			body:        `{"prompt":"hello","unknownExtra":1}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "malformed",
			body:        `{"prompt":`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "empty",
			body:        "",
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "invalid request payload",
		},
		{
			name:        "multi_object",
			body:        `{"prompt":"hello"}{}`,
			wantStatus:  http.StatusBadRequest,
			wantCode:    "BAD_REQUEST",
			wantMessage: "request payload must contain one JSON object",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(
				http.MethodPost,
				"/factory-sessions/~default/factory/workstations/Review/prompt-template-validation",
				bytes.NewBufferString(tc.body),
			)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if tc.wantStatus == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				result := decodeJSONResponse[factoryapi.PromptTemplateValidationResult](t, rec)
				if !result.Valid {
					t.Fatalf("validation result = %#v, want valid prompt", result)
				}
				return
			}
			assertFactoryDecodeJSONError(t, rec, tc.wantStatus, tc.wantCode, tc.wantMessage, tc.wantTargets)
		})
	}
}

func assertFactoryDecodeJSONError(
	t *testing.T,
	rec *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
	wantMessage string,
	wantTargets bool,
) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}

	response := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	assertErrorResponsePreservesLegacyFields(
		t,
		response,
		factoryapi.ErrorResponseCode(wantCode),
		factoryapi.ErrorFamilyBadRequest,
		wantMessage,
	)
	if wantTargets {
		if response.Targets == nil || len(*response.Targets) != 1 {
			t.Fatalf("targets = %#v, want one form factory payload target", response.Targets)
		}
		assertHasValidationTargetCode(t, *response.Targets, factoryvalidation.CodeFactoryPayloadInvalid)
	}
}
