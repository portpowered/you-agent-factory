package http

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestRootErrorResponse_MapsEventReconnectValidationFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		recordings.ErrInvalidSubscribeScope,
		recordings.ErrInvalidReconnectCursor,
		recordings.ErrReconnectCursorNotFound,
		recordings.ErrReconnectCursorExpired,
		recordings.ErrReconnectCursorUnavailable,
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := RootErrorResponse(err, recordingsHTTPOperationEventSubscribe)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed bad request", err)
			}
			if status != http.StatusBadRequest ||
				response.Family != factoryapi.ErrorFamilyBadRequest ||
				response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
				response.Message != "invalid event reconnect cursor" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want bad request reconnect cursor", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsArtifactReadValidationFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		errInvalidArtifactReadScope,
		errInvalidArtifactReadID,
		recordings.ErrInvalidRecordingScope,
		recordings.ErrInvalidProjectionInput,
		recordings.ErrInvalidProjectionScope,
		recordings.ErrMalformedProjectionOrder,
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := RootErrorResponse(err, recordingsHTTPOperationArtifactRead)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed bad request", err)
			}
			if status != http.StatusBadRequest ||
				response.Family != factoryapi.ErrorFamilyBadRequest ||
				response.Code != factoryapi.ErrorResponseCodeBADREQUEST ||
				response.Message != "invalid artifact read request" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want bad request artifact read", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_MapsArtifactMissingTargetFailures(t *testing.T) {
	t.Parallel()

	cases := []error{
		recordings.ErrMissingRecordingTarget,
		recordings.ErrPortableArtifactUnavailable,
		recordings.ErrForeignPortableArtifact,
	}
	for _, err := range cases {
		t.Run(err.Error(), func(t *testing.T) {
			t.Parallel()
			status, response, ok := RootErrorResponse(err, recordingsHTTPOperationArtifactRead)
			if !ok {
				t.Fatalf("RootErrorResponse(%v) = not handled, want typed not found", err)
			}
			if status != http.StatusNotFound ||
				response.Family != factoryapi.ErrorFamilyNotFound ||
				response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
				response.Message != "factory session artifact not found" {
				t.Fatalf("RootErrorResponse(%v) = %d %#v, want not found artifact read", err, status, response)
			}
		})
	}
}

func TestRootErrorResponse_IgnoresCrossOperationTypedFailures(t *testing.T) {
	t.Parallel()

	if _, _, ok := RootErrorResponse(recordings.ErrMissingRecordingTarget, recordingsHTTPOperationEventSubscribe); ok {
		t.Fatal("missing target must not map through event subscribe operation")
	}
	if _, _, ok := RootErrorResponse(recordings.ErrInvalidReconnectCursor, recordingsHTTPOperationArtifactRead); ok {
		t.Fatal("invalid reconnect cursor must not map through artifact read operation")
	}
}

func TestRootErrorResponse_ReturnsFalseForUnmappedFailures(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("pkg/services/recordings/internal/service: boom")
	if _, _, ok := RootErrorResponse(err, recordingsHTTPOperationArtifactRead); ok {
		t.Fatalf("unmapped failure %#v must not be handled", err)
	}
}

func TestWriteRootOrInternalError_SanitizesUnmappedFailures(t *testing.T) {
	t.Parallel()

	adapter := NewAdapter(&rootFake{})
	recorder := httptest.NewRecorder()
	err := errors.New("pkg/services/recordings/internal/service: boom")

	adapter.writeRootOrInternalError(recorder, recordingsHTTPOperationArtifactRead, err)

	body := recorder.Body.String()
	if recorder.Code != http.StatusInternalServerError ||
		!strings.Contains(body, `"code":"INTERNAL_ERROR"`) ||
		!strings.Contains(body, `"family":"INTERNAL_SERVER_ERROR"`) ||
		strings.Contains(body, "pkg/services/recordings") ||
		strings.Contains(body, "boom") {
		t.Fatalf("response = %d %s, want sanitized internal error", recorder.Code, body)
	}
}
