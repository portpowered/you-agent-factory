package clihttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

type listSessionsResponse struct {
	Sessions []string `json:"sessions"`
}

func TestGetJSON_DecodesSuccessResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/factory-sessions" {
			t.Fatalf("path = %q, want /factory-sessions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(listSessionsResponse{Sessions: []string{"~default"}}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var result listSessionsResponse
	resp, err := GetJSON(
		context.Background(),
		srv.Client(),
		srv.URL+"/factory-sessions",
		&result,
		RequestOptions{EndpointPath: "/factory-sessions", LogLabel: "session list"},
	)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if len(result.Sessions) != 1 || result.Sessions[0] != "~default" {
		t.Fatalf("result = %#v", result)
	}
}

func TestGetJSON_ReturnsResponseForAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "invalid session",
			Code:    factoryapi.ErrorResponseCode("INVALID_REQUEST"),
			Family:  factoryapi.ErrorFamilyBadRequest,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var result listSessionsResponse
	resp, err := GetJSON(
		context.Background(),
		srv.Client(),
		srv.URL,
		&result,
		RequestOptions{EndpointPath: "/factory-sessions", LogLabel: "session list"},
	)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	errResp, ok := DecodeAPIError(resp)
	if !ok {
		t.Fatal("DecodeAPIError = false, want true")
	}
	if errResp.Message != "invalid session" {
		t.Fatalf("message = %q", errResp.Message)
	}
}

func TestGetJSON_PropagatesTransportFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})}

	var diagnostics bytes.Buffer
	_, err := GetJSON(
		context.Background(),
		client,
		"http://127.0.0.1:1/factory-sessions",
		nil,
		RequestOptions{
			Diagnostics:  &diagnostics,
			Verbose:      true,
			EndpointPath: "/factory-sessions",
			LogLabel:     "session list",
		},
	)
	if err == nil {
		t.Fatal("GetJSON: want transport error")
	}
	if !strings.Contains(diagnostics.String(), "session list response endpointPath=/factory-sessions error=unreachable") {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
}

func TestGetJSON_LogsStatusDiagnosticForNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var diagnostics bytes.Buffer
	resp, err := GetJSON(
		context.Background(),
		srv.Client(),
		srv.URL,
		nil,
		RequestOptions{
			Diagnostics:  &diagnostics,
			Verbose:      true,
			EndpointPath: "/work",
			LogLabel:     "work list",
		},
	)
	if err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	defer resp.Body.Close()

	if !strings.Contains(diagnostics.String(), "work list response endpointPath=/work status=500") {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
}

func TestPostJSON_SendsJSONBodyAndDecodesResponse(t *testing.T) {
	var gotMethod string
	var gotContentType string
	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "accepted"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var result map[string]string
	resp, err := PostJSON(
		context.Background(),
		srv.Client(),
		srv.URL+"/work",
		strings.NewReader(`{"name":"demo"}`),
		&result,
		RequestOptions{EndpointPath: "/work", LogLabel: "work submit"},
	)
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	defer resp.Body.Close()

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	if gotBody != `{"name":"demo"}` {
		t.Fatalf("body = %q", gotBody)
	}
	if result["status"] != "accepted" {
		t.Fatalf("result = %#v", result)
	}
}

func TestDecodeAPIError_ReturnsFalseWithoutMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"INVALID_REQUEST","family":"client"}`))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()

	_, ok := DecodeAPIError(resp)
	if ok {
		t.Fatal("DecodeAPIError = true, want false")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
