package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestList_PerformsGETFactorySessions(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotPath != "/factory-sessions" {
		t.Fatalf("path = %q, want /factory-sessions", gotPath)
	}
}

func TestList_HumanOutputShowsEmptyState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "No live factory sessions were found.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_HumanOutputRendersSessionTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Id:         "~default",
					IsDefault:  true,
					Project:    "root",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindDefault,
					},
				},
				{
					FactoryDir: "/workspace/root/beta",
					FolderPath: "/workspace/root",
					Id:         "session-beta",
					IsDefault:  false,
					Project:    "beta",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindNamed,
						Name: stringPtr("beta"),
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "SESSION ID\tPROJECT\tFOLDER PATH\tFACTORY DIR\tDEFAULT\tTARGET KIND\tTARGET NAME\n" +
		"~default\troot\t/workspace/root\t/workspace/root\tyes\tdefault\t\n" +
		"session-beta\tbeta\t/workspace/root\t/workspace/root/beta\tno\tnamed\tbeta\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_JSONModeEmitsListFactorySessionsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Id != "session-alpha" {
		t.Fatalf("sessions = %#v, want one session-alpha summary", got.Sessions)
	}
	if strings.Contains(out.String(), "SESSION ID") {
		t.Fatalf("JSON output included human table header: %q", out.String())
	}
}

func TestList_UnreachableServiceNamesEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := List(ListConfig{
		Port:   1,
		JSON:   true,
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	if !strings.Contains(err.Error(), "factory sessions endpoint not reachable at http://localhost:1/factory-sessions") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output when --json is set", out.String())
	}
}

func TestList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := List(ListConfig{
		Port:        serverPort(t, srv),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, out.String())
	}
	diag := diagnostics.String()
	for _, want := range []string{
		"session list request",
		"endpointPath=/factory-sessions",
		"session list response",
	} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diag)
		}
	}
}

func sampleSessionSummary() factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		FactoryDir: "/workspace/fleet/alpha",
		FolderPath: "/workspace/fleet",
		Id:         "session-alpha",
		IsDefault:  false,
		Project:    "alpha",
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: stringPtr("alpha"),
		},
	}
}

func stringPtr(value string) *string {
	return &value
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}
