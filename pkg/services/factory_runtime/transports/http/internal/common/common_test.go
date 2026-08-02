package common

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type typedNilRuntimeRoot struct{ factoryruntime.Service }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

func TestRequireRuntimeRootRejectsTypedNil(t *testing.T) {
	var typedNil *typedNilRuntimeRoot
	if root, err := RequireRuntimeRoot(typedNil); root != nil || !errors.Is(err, ErrRuntimeServiceRequired) {
		t.Fatalf("RequireRuntimeRoot(typed nil) = (%v, %v), want required error", root, err)
	}
}

func TestDecodeJSONHandlesEmptyInvalidAndReaderErrors(t *testing.T) {
	if _, err := DecodeRequiredJSON[map[string]string](bytes.NewReader(nil)); !errors.Is(err, ErrRequestBodyRequired) {
		t.Fatalf("required empty body error = %v", err)
	}
	if _, err := DecodeRequiredJSON[map[string]string](failingReader{}); err == nil {
		t.Fatal("required failing reader should return an error")
	}
	if value, err := DecodeRequiredJSON[map[string]string](bytes.NewBufferString(`{"name":"runtime"}`)); err != nil || value["name"] != "runtime" {
		t.Fatalf("required valid JSON = %#v, %v", value, err)
	}
	if _, err := DecodeRequiredJSON[map[string]string](bytes.NewBufferString("{")); err == nil {
		t.Fatal("required invalid JSON should return an error")
	}

	if value, err := DecodeOptionalJSON[map[string]string](bytes.NewReader(nil)); err != nil || value != nil {
		t.Fatalf("optional empty JSON = %#v, %v", value, err)
	}
	if _, err := DecodeOptionalJSON[map[string]string](failingReader{}); err == nil {
		t.Fatal("optional failing reader should return an error")
	}
	if _, err := DecodeOptionalJSON[map[string]string](bytes.NewBufferString("{")); err == nil {
		t.Fatal("optional invalid JSON should return an error")
	}
}

func TestErrorFamilyAndRequestContextOutcomes(t *testing.T) {
	for _, test := range []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "BAD_REQUEST"},
		{http.StatusNotFound, "NOT_FOUND"},
		{http.StatusConflict, "CONFLICT"},
		{http.StatusInternalServerError, "INTERNAL_SERVER_ERROR"},
	} {
		if got := string(ErrorFamilyForStatus(test.status)); got != test.want {
			t.Fatalf("ErrorFamilyForStatus(%d) = %q, want %q", test.status, got, test.want)
		}
	}

	if status, response, handled := RequestContextErrorResponse(nil); handled || status != 0 || response != nil {
		t.Fatalf("nil context error outcome = (%d, %#v, %v)", status, response, handled)
	}
	if status, _, handled := RequestContextErrorResponse(context.Canceled); !handled || status != 0 {
		t.Fatalf("canceled outcome = (%d, %v)", status, handled)
	}
	if status, _, handled := RequestContextErrorResponse(context.DeadlineExceeded); !handled || status != http.StatusGatewayTimeout {
		t.Fatalf("deadline outcome = (%d, %v)", status, handled)
	}
	if _, _, handled := RequestContextErrorResponse(errors.New("other")); handled {
		t.Fatal("unrelated error should not be handled")
	}

	if RequestContextEnded(nil) || IsRequestContextEnded(errors.New("other")) || ShouldEndOnRequestContext(context.Background(), nil) {
		t.Fatal("non-terminal request context state was treated as terminal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !RequestContextEnded(ctx) || !ShouldEndOnRequestContext(context.Background(), context.Canceled) {
		t.Fatal("terminal request context state was not recognized")
	}

	deadline := httptest.NewRecorder()
	if !WriteRequestContextOutcome(deadline, context.DeadlineExceeded) || deadline.Code != http.StatusGatewayTimeout {
		t.Fatalf("deadline response = %d", deadline.Code)
	}
	if WriteRequestContextOutcome(httptest.NewRecorder(), errors.New("other")) {
		t.Fatal("unrelated error should not write a context outcome")
	}
	request := httptest.NewRequest(http.MethodGet, "/", io.Reader(bytes.NewBuffer(nil))).WithContext(ctx)
	if !GuardRequestContext(httptest.NewRecorder(), request) {
		t.Fatal("canceled request should be guarded")
	}
}
