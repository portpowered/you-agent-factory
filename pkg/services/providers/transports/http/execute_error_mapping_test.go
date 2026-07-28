package http_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	providershttp "github.com/portpowered/infinite-you/pkg/services/providers/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestExecuteRootErrorResponse_MapsInvalidExecuteRequestFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.ExecuteRootErrorResponseForTest(providershttp.ErrInvalidExecuteRequest)
	if !ok {
		t.Fatal("ExecuteRootErrorResponse(ErrInvalidExecuteRequest) = not handled, want typed bad request")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "invalid provider execution request" {
		t.Fatalf("ExecuteRootErrorResponse(ErrInvalidExecuteRequest) = %d %#v, want bad request", status, response)
	}
}

func TestExecuteRootErrorResponse_MapsInvalidIDFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.ExecuteRootErrorResponseForTest(providers.ErrInvalidID)
	if !ok {
		t.Fatal("ExecuteRootErrorResponse(ErrInvalidID) = not handled, want typed bad request")
	}
	if status != http.StatusBadRequest ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
		response.Message != "invalid provider id" {
		t.Fatalf("ExecuteRootErrorResponse(ErrInvalidID) = %d %#v, want bad request", status, response)
	}
}

func TestExecuteRootErrorResponse_MapsUnknownProviderFailures(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.ExecuteRootErrorResponseForTest(providers.ErrUnknownProvider)
	if !ok {
		t.Fatal("ExecuteRootErrorResponse(ErrUnknownProvider) = not handled, want typed not found")
	}
	if status != http.StatusNotFound ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
		response.Message != "provider not found" {
		t.Fatalf("ExecuteRootErrorResponse(ErrUnknownProvider) = %d %#v, want not found", status, response)
	}
}

func TestExecuteRootErrorResponse_MapsExecuteFailureKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name: "canceled",
			err: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindCanceled,
				Message: "attempt cancelled by peer policy",
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_EXECUTION_CANCELED",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name:       "timeout",
			err:        providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout},
			wantStatus: http.StatusGatewayTimeout,
			wantCode:   "PROVIDER_EXECUTION_TIMEOUT",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name: "invalid request",
			err: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: "missing user message",
			},
			wantStatus: http.StatusBadRequest,
			wantCode:   "BAD_REQUEST",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name: "authentication",
			err: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindAuthentication,
				Message: "auth failed",
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   "PROVIDER_EXECUTION_AUTHENTICATION",
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
		{
			name:       "throttled",
			err:        providers.ExecuteFailure{Kind: providers.ExecuteFailureKindThrottled},
			wantStatus: http.StatusTooManyRequests,
			wantCode:   "PROVIDER_EXECUTION_THROTTLED",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name:       "dependency",
			err:        providers.ExecuteFailure{Kind: providers.ExecuteFailureKindDependency},
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   "PROVIDER_EXECUTION_DEPENDENCY",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
		{
			name:       "unknown",
			err:        providers.ExecuteFailure{Kind: providers.ExecuteFailureKindUnknown},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "PROVIDER_EXECUTION_FAILED",
			wantFamily: factoryapi.ErrorFamilyInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, response, ok := providershttp.ExecuteRootErrorResponseForTest(tt.err)
			if !ok {
				t.Fatalf("ExecuteRootErrorResponse(%v) = not handled, want typed outcome", tt.err)
			}
			if status != tt.wantStatus ||
				response.Family != tt.wantFamily ||
				string(response.Code) != tt.wantCode {
				t.Fatalf("ExecuteRootErrorResponse(%v) = %d %#v, want %d family=%s code=%s",
					tt.err, status, response, tt.wantStatus, tt.wantFamily, tt.wantCode)
			}
		})
	}
}

func TestExecuteRootErrorResponse_MapsExecuteSentinels(t *testing.T) {
	t.Parallel()

	status, response, ok := providershttp.ExecuteRootErrorResponseForTest(providers.ErrExecuteCancelled)
	if !ok || status != http.StatusInternalServerError || string(response.Code) != "PROVIDER_EXECUTION_CANCELED" {
		t.Fatalf("ExecuteRootErrorResponse(ErrExecuteCancelled) = %d %#v, want canceled outcome", status, response)
	}

	status, response, ok = providershttp.ExecuteRootErrorResponseForTest(providers.ErrExecuteTimeout)
	if !ok || status != http.StatusGatewayTimeout || string(response.Code) != "PROVIDER_EXECUTION_TIMEOUT" {
		t.Fatalf("ExecuteRootErrorResponse(ErrExecuteTimeout) = %d %#v, want timeout outcome", status, response)
	}

	status, response, ok = providershttp.ExecuteRootErrorResponseForTest(providers.ErrExecuteFailed)
	if !ok || status != http.StatusInternalServerError || string(response.Code) != "PROVIDER_EXECUTION_FAILED" {
		t.Fatalf("ExecuteRootErrorResponse(ErrExecuteFailed) = %d %#v, want failed outcome", status, response)
	}
}

func TestExecuteRootErrorResponse_ReturnsFalseForUnmappedFailures(t *testing.T) {
	t.Parallel()

	err := errors.New("pkg/services/providers/internal/execution: boom")
	if _, _, ok := providershttp.ExecuteRootErrorResponseForTest(err); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}
