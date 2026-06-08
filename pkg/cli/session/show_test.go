package session

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestShow_PerformsGETFactorySession(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sampleFactorySession()); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if gotPath != "/factory-sessions/session-beta" {
		t.Fatalf("path = %q, want /factory-sessions/session-beta", gotPath)
	}
}

func TestShow_HumanOutputRendersJavaScriptFactorySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sampleFactorySession()); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Factory session:\tsession-beta",
		"Orchestrator kind:\tJAVASCRIPT",
		"Dynamic workflow:\tJavaScript factory session",
		"Phase:\treview",
		"Child dispatches:\tqueued=1 running=2 completed=4",
		"Checkpoint ref:\tcp-1 (plan) — saved plan checkpoint",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "rawCheckpointBody") {
		t.Fatalf("output leaked raw checkpoint body: %s", output)
	}
}

func TestShow_HumanOutputRendersPetriFactorySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(samplePetriFactorySession()); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "~default",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Orchestrator kind:\tPETRI",
		"Petri marking tokens:\t1",
		"Enabled transition:\ttr-process (worker-a)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Dynamic workflow:") {
		t.Fatalf("Petri output should not include dynamic workflow shorthand: %s", output)
	}
}

func TestShow_JSONModeEmitsFactorySession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(sampleFactorySession()); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}

	var got factoryapi.FactorySession
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if got.Id != "session-beta" || got.Runtime.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("session = %#v, want JavaScript session-beta", got)
	}
}

func TestShow_NotFoundReportsSessionID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"session missing"}`))
	}))
	defer srv.Close()

	err := Show(ShowConfig{
		Server:    srv.URL,
		SessionID: "missing",
		Output:    &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), `factory session "missing" not found`) {
		t.Fatalf("error = %q", err.Error())
	}
}

func sampleFactorySession() factoryapi.FactorySession {
	phase := "review"
	summary := "saved plan checkpoint"
	label := "plan"
	return factoryapi.FactorySession{
		Id:         "session-beta",
		FactoryDir: "/workspace/root/beta",
		FolderPath: "/workspace/root",
		Project:    "beta",
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: stringPtr("beta"),
		},
		Runtime: factoryapi.FactorySessionRuntime{
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Status:           factoryapi.FactorySessionStatusIDLE,
			Progress: factoryapi.FactorySessionProgress{
				FactoryState:  "RUNNING",
				Categories:    factoryapi.StatusCategories{},
				InFlightCount: 0,
				TotalTokens:   0,
			},
			Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
			Javascript: &factoryapi.FactorySessionJavaScriptProjection{
				Phase:  &phase,
				Phases: []string{"plan", "review"},
				ScriptStatus: factoryapi.FactorySessionJavaScriptScriptStatusIDLE,
				ChildDispatchCounts: factoryapi.FactorySessionJavaScriptChildDispatchCounts{
					Queued:    1,
					Running:   2,
					Completed: 4,
				},
				Checkpoints: &[]factoryapi.FactorySessionJavaScriptCheckpointRef{{
					Id:      "cp-1",
					Label:   &label,
					Summary: &summary,
				}},
			},
		},
	}
}

func samplePetriFactorySession() factoryapi.FactorySession {
	return factoryapi.FactorySession{
		Id:         "~default",
		FactoryDir: "/workspace/root",
		FolderPath: "/workspace/root",
		Project:    "root",
		IsDefault:  true,
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindDefault,
		},
		Runtime: factoryapi.FactorySessionRuntime{
			OrchestratorKind: factoryapi.PETRI,
			Status:           factoryapi.FactorySessionStatusIDLE,
			Progress: factoryapi.FactorySessionProgress{
				FactoryState:  "RUNNING",
				Categories:    factoryapi.StatusCategories{},
				InFlightCount: 0,
				TotalTokens:   1,
			},
			Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
			Petri: &factoryapi.FactorySessionPetriProjection{
				Marking: []factoryapi.TokenResponse{{Id: "tok-1", PlaceId: "task:init", TraceId: "trace-1", WorkId: "work-1", WorkType: "task", CreatedAt: time.Now(), EnteredAt: time.Now()}},
				EnabledTransitions: []factoryapi.FactorySessionPetriEnabledTransition{{
					TransitionId: "tr-process",
					WorkerType:   "worker-a",
				}},
			},
		},
	}
}
