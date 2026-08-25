package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerWithRuntimeReturnsPlausibleSnapshotAndExactCommit(t *testing.T) {
	const wantCommit = uint64(70 * 1024 * 1024 * 1024)
	server := httptest.NewServer(HandlerWithRuntime(
		http.NotFoundHandler(),
		func() (uint64, error) { return wantCommit, nil },
	))
	defer server.Close()

	response, err := http.Get(server.URL + runtimeDiagnosticsPath)
	if err != nil {
		t.Fatalf("GET runtime diagnostics: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime diagnostics status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var snapshot RuntimeSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode runtime diagnostics: %v", err)
	}
	if snapshot.HeapAllocBytes == 0 || snapshot.HeapInuseBytes < snapshot.HeapAllocBytes ||
		snapshot.SysBytes < snapshot.HeapInuseBytes || snapshot.Goroutines <= 0 {
		t.Fatalf("runtime snapshot = %+v, want plausible live runtime values", snapshot)
	}
	if !snapshot.ProcessCommitAvailable || snapshot.ProcessCommitBytes != wantCommit {
		t.Fatalf("process commit = (%t, %d), want (true, %d)", snapshot.ProcessCommitAvailable, snapshot.ProcessCommitBytes, wantCommit)
	}
	if response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", response.Header.Get("Content-Type"))
	}
}

func TestHandlerWithRuntimeReturnsNilForNilApplicationHandler(t *testing.T) {
	if handler := HandlerWithRuntime(nil, nil); handler != nil {
		t.Fatalf("HandlerWithRuntime(nil) = %T, want nil", handler)
	}
}

func TestHandlerWithRuntimePreservesGoFieldsWhenCommitUnavailable(t *testing.T) {
	server := httptest.NewServer(HandlerWithRuntime(
		http.NotFoundHandler(),
		func() (uint64, error) { return 0, errors.New("commit read failed") },
	))
	defer server.Close()

	response, err := http.Get(server.URL + runtimeDiagnosticsPath)
	if err != nil {
		t.Fatalf("GET runtime diagnostics: %v", err)
	}
	defer response.Body.Close()
	var snapshot RuntimeSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode runtime diagnostics: %v", err)
	}
	if snapshot.HeapAllocBytes == 0 || snapshot.HeapInuseBytes == 0 || snapshot.SysBytes == 0 || snapshot.Goroutines <= 0 {
		t.Fatalf("runtime snapshot = %+v, want Go runtime fields preserved", snapshot)
	}
	if snapshot.ProcessCommitAvailable || snapshot.ProcessCommitBytes != 0 {
		t.Fatalf("unavailable process commit = (%t, %d), want (false, 0)", snapshot.ProcessCommitAvailable, snapshot.ProcessCommitBytes)
	}
}

func TestHandlerWithRuntimeRejectsNonGetWithoutCollectingACommit(t *testing.T) {
	called := false
	handler := HandlerWithRuntime(http.NotFoundHandler(), func() (uint64, error) {
		called = true
		return 1, nil
	})
	request := httptest.NewRequest(http.MethodPost, runtimeDiagnosticsPath, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST runtime diagnostics status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", response.Header().Get("Allow"))
	}
	if called {
		t.Fatal("commit reader called for a non-GET runtime request")
	}
}

func TestHandlerWithRuntimeKeepsUnderlyingRoute(t *testing.T) {
	underlying := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/application" {
			t.Fatalf("underlying path = %q, want /application", request.URL.Path)
		}
		_, _ = writer.Write([]byte("application"))
	})
	server := httptest.NewServer(HandlerWithRuntime(underlying, nil))
	defer server.Close()

	response, err := http.Get(server.URL + "/application")
	if err != nil {
		t.Fatalf("GET underlying route: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read underlying route: %v", err)
	}
	if string(body) != "application" {
		t.Fatalf("underlying body = %q, want application", body)
	}
}

func TestRuntimeSnapshotHandlerReturnsOnResponseWriterFailure(t *testing.T) {
	writer := &failingRuntimeResponseWriter{header: make(http.Header)}
	request := httptest.NewRequest(http.MethodGet, runtimeDiagnosticsPath, nil)
	runtimeSnapshotHandler(func() (uint64, error) { return 123, nil }).ServeHTTP(writer, request)
	if writer.writes != 1 {
		t.Fatalf("response writes = %d, want one attempted JSON write", writer.writes)
	}
}

type failingRuntimeResponseWriter struct {
	header http.Header
	writes int
}

func (writer *failingRuntimeResponseWriter) Header() http.Header { return writer.header }

func (writer *failingRuntimeResponseWriter) Write([]byte) (int, error) {
	writer.writes++
	return 0, errors.New("response writer failed")
}

func (*failingRuntimeResponseWriter) WriteHeader(int) {}
