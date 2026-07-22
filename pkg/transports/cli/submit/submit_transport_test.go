package submit

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/sessionpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Transport-edge regression tests guard clihttp migration outcomes at the HTTP boundary.

func TestSubmit_Transport_HTTP201CreatedSuccess(t *testing.T) {
	workID := "transport-work-201"
	name := "transport-submit"
	workType := "task"
	traceID := "transport-trace-201"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if err := json.NewEncoder(w).Encode(factoryapi.SubmitWorkResponse{
			TraceId:      traceID,
			WorkId:       &workID,
			Name:         &name,
			WorkTypeName: &workType,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"transport edge task"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	server := mustServerBase(t, srv.URL)
	baseCfg := SubmitConfig{Context: context.Background(),
		Name:         name,
		WorkTypeName: workType,
		Payload:      payloadPath,
		Server:       server,
	}

	t.Run("humanStdout", func(t *testing.T) {
		var out bytes.Buffer
		cfg := baseCfg
		cfg.Output = &out
		if err := Submit(t, cfg); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		got := out.String()
		for _, want := range []string{
			"Submitted: transport-submit (task)\n",
			"traceId: transport-trace-201\n",
			"workId: transport-work-201\n",
			"Verify: you work show transport-work-201\n",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("stdout missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("jsonStdout", func(t *testing.T) {
		var out bytes.Buffer
		cfg := baseCfg
		cfg.JSON = true
		cfg.Output = &out
		if err := Submit(t, cfg); err != nil {
			t.Fatalf("Submit: %v", err)
		}
		var envelope SubmitSuccessResult
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatalf("stdout is not valid submit success JSON: %v\n%s", err, out.String())
		}
		if envelope.WorkID == nil || *envelope.WorkID != workID {
			t.Fatalf("workId = %v, want %q", envelope.WorkID, workID)
		}
		if envelope.Name != name {
			t.Fatalf("name = %q, want %q", envelope.Name, name)
		}
		if envelope.WorkTypeName != workType {
			t.Fatalf("workTypeName = %q, want %q", envelope.WorkTypeName, workType)
		}
		if envelope.TraceID != traceID {
			t.Fatalf("traceId = %q, want %q", envelope.TraceID, traceID)
		}
		if envelope.SessionID != "~default" {
			t.Fatalf("sessionId = %q, want ~default", envelope.SessionID)
		}
		if envelope.EndpointPath != "/factory-sessions/~default/work" {
			t.Fatalf("endpointPath = %q, want /factory-sessions/~default/work", envelope.EndpointPath)
		}
	})
}

func TestSubmit_Transport_UnreachableFactory(t *testing.T) {
	server := "http://127.0.0.1:19999"
	endpointPath := sessionpath.ScopedPath("/work", "")
	endpointURL, err := cliserver.RequestURL(server, endpointPath)
	if err != nil {
		t.Fatalf("RequestURL: %v", err)
	}

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	submitErr := Submit(t, SubmitConfig{Context: context.Background(),
		Name:         "transport-unreachable",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       server,
		Output:       &out,
	})
	if submitErr == nil {
		t.Fatal("expected error when factory is not running")
	}
	wantPrefix := "factory not reachable at " + endpointURL
	if got := submitErr.Error(); !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("error = %q, want prefix %q", got, wantPrefix)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on transport failure", out.String())
	}
}

func TestSubmit_Transport_StructuredAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "workTypeName is required",
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
			Family:  factoryapi.ErrorFamilyBadRequest,
		}); err != nil {
			t.Fatalf("encode error response: %v", err)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	payloadPath := filepath.Join(dir, "work.json")
	if err := os.WriteFile(payloadPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := Submit(t, SubmitConfig{Context: context.Background(),
		Name:         "transport-api-error",
		WorkTypeName: "task",
		Payload:      payloadPath,
		Server:       mustServerBase(t, srv.URL),
		Output:       &out,
	})
	if err == nil {
		t.Fatal("expected structured API error")
	}
	if got := err.Error(); got != "submission failed (400): workTypeName is required" {
		t.Fatalf("error = %q, want submission failed (400): workTypeName is required", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on API failure", out.String())
	}
}
