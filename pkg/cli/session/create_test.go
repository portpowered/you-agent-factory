package session

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestCreate_PerformsPOSTFactorySessions(t *testing.T) {
	var gotMethod, gotPath string
	var gotRequest factoryapi.OpenFactorySessionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPtr("beta"),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/fleet",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/factory-sessions" {
		t.Fatalf("path = %q, want /factory-sessions", gotPath)
	}
	if gotRequest.FolderPath != "/workspace/fleet" {
		t.Fatalf("folderPath = %q, want /workspace/fleet", gotRequest.FolderPath)
	}
}

func TestCreate_HumanOutputPrintsOpenedSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPtr("beta"),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/fleet",
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	want := "Opened factory session session-beta\n" +
		"Project: beta\n" +
		"Folder path: /workspace/fleet\n" +
		"Default: no\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCreate_JSONModeEmitsOpenFactorySessionResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Id:         "session-beta",
				IsDefault:  false,
				Project:    "beta",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindNamed,
					Name: stringPtr("beta"),
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/fleet",
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var got factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if got.Session == nil || got.Session.Id != "session-beta" {
		t.Fatalf("session = %#v, want session-beta", got.Session)
	}
}

func TestCreate_ValidateOnlySendsRequestField(t *testing.T) {
	var gotRequest factoryapi.OpenFactorySessionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Create(CreateConfig{
		Port:         serverPort(t, srv),
		Dir:          "/workspace/fleet",
		ValidateOnly: true,
		Output:       io.Discard,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotRequest.ValidateOnly == nil || !*gotRequest.ValidateOnly {
		t.Fatalf("validateOnly = %#v, want true", gotRequest.ValidateOnly)
	}
	if gotRequest.InitNewFactory != nil {
		t.Fatalf("initNewFactory = %#v, want omitted", gotRequest.InitNewFactory)
	}
}

func TestCreate_InitNewFactorySendsRequestField(t *testing.T) {
	var gotRequest factoryapi.OpenFactorySessionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{
				Id:         "session-new",
				FolderPath: "/workspace/new",
				Project:    "default",
				Target: factoryapi.FactorySessionTargetRef{
					Kind: factoryapi.FactorySessionTargetRefKindDefault,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Create(CreateConfig{
		Port:           serverPort(t, srv),
		Dir:            "/workspace/new",
		InitNewFactory: true,
		Output:         io.Discard,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotRequest.InitNewFactory == nil || !*gotRequest.InitNewFactory {
		t.Fatalf("initNewFactory = %#v, want true", gotRequest.InitNewFactory)
	}
	if gotRequest.ValidateOnly != nil {
		t.Fatalf("validateOnly = %#v, want omitted", gotRequest.ValidateOnly)
	}
}

func TestCreate_TargetKindAndNameMapToRequestRef(t *testing.T) {
	var gotRequest factoryapi.OpenFactorySessionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Session: &factoryapi.FactorySessionSummary{Id: "session-beta"},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Create(CreateConfig{
		Port:       serverPort(t, srv),
		Dir:        "/workspace/fleet",
		TargetKind: "named",
		TargetName: "beta",
		Output:     io.Discard,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gotRequest.Target == nil {
		t.Fatal("target = nil, want named beta")
	}
	if gotRequest.Target.Kind != factoryapi.FactorySessionTargetRefKindNamed {
		t.Fatalf("target kind = %q, want named", gotRequest.Target.Kind)
	}
	if gotRequest.Target.Name == nil || *gotRequest.Target.Name != "beta" {
		t.Fatalf("target name = %#v, want beta", gotRequest.Target.Name)
	}
}

func TestCreate_MultiTargetHumanOutputExitsNonZero(t *testing.T) {
	targets := []factoryapi.FactorySessionTarget{
		{
			Label: "default",
			Ref: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindDefault,
			},
		},
		{
			Label: "alpha",
			Ref: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindNamed,
				Name: stringPtr("alpha"),
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Targets: &targets,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/fleet",
		Output: &out,
	})
	if !errorsIsTargetSelection(err) {
		t.Fatalf("error = %v, want target selection required", err)
	}

	want := "Multiple factory targets are available; choose one with --target-kind and --target-name:\n" +
		"LABEL\tREF\n" +
		"default\tdefault\n" +
		"alpha\tnamed:alpha\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestCreate_MultiTargetJSONEmitsResponseAndExitsNonZero(t *testing.T) {
	targets := []factoryapi.FactorySessionTarget{
		{
			Label: "alpha",
			Ref: factoryapi.FactorySessionTargetRef{
				Kind: factoryapi.FactorySessionTargetRefKindNamed,
				Name: stringPtr("alpha"),
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
			Targets: &targets,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/fleet",
		JSON:   true,
		Output: &out,
	})
	if !errorsIsTargetSelection(err) {
		t.Fatalf("error = %v, want target selection required", err)
	}

	var got factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if got.Targets == nil || len(*got.Targets) != 1 {
		t.Fatalf("targets = %#v, want one alpha target", got.Targets)
	}
}

func TestCreate_BadRequestSurfacesAPIMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{
			Message: "folder validation failed",
			Code:    factoryapi.BADREQUEST,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	err := Create(CreateConfig{
		Port:   serverPort(t, srv),
		Dir:    "/workspace/missing",
		Output: io.Discard,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "open factory session failed (400): folder validation failed") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestCreate_UnreachableServiceNamesEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := Create(CreateConfig{
		Port:   1,
		Dir:    "/workspace/fleet",
		JSON:   true,
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	want := "factory sessions endpoint not reachable at http://localhost:1/factory-sessions"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output when --json is set", out.String())
	}
}

func TestCreate_RejectsMutuallyExclusiveFlags(t *testing.T) {
	err := Create(CreateConfig{
		Port:           8080,
		Dir:            "/workspace/fleet",
		InitNewFactory: true,
		ValidateOnly:   true,
	})
	if err == nil {
		t.Fatal("expected flag validation error")
	}
	if !strings.Contains(err.Error(), "init-new-factory cannot be combined with validate-only") {
		t.Fatalf("error = %q", err.Error())
	}
}

func TestCreate_RejectsMissingDir(t *testing.T) {
	err := Create(CreateConfig{Port: 8080, Dir: "   "})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "folder path is required") {
		t.Fatalf("error = %q", err.Error())
	}
}

func errorsIsTargetSelection(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrFactorySessionTargetsRequireSelection.Error())
}
