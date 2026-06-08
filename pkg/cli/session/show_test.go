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
	var gotPaths []string
	srv := httptest.NewServer(sessionShowTestHandler(t, &gotPaths))
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
	if len(gotPaths) == 0 || gotPaths[0] != "/factory-sessions/session-beta" {
		t.Fatalf("paths = %#v, want session show path first", gotPaths)
	}
}

func TestShow_HumanOutputRendersJavaScriptFactorySession(t *testing.T) {
	srv := httptest.NewServer(sessionShowTestHandler(t, nil))
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
		"Session started:\t",
		"Dispatch:\tdispatch-1 (review child) status=RECONCILED kind=JAVASCRIPT_AGENT",
		"Artifact ref:\tartifact-1 (review output) kind=CHILD_RESULT visibility=PUBLIC",
		"Partial result ref:\tartifact-partial (FINDING)",
		"Final result ref:\tartifact-final (FINAL_RESULT)",
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
	srv := httptest.NewServer(sessionShowTestHandler(t, nil))
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

func sessionShowTestHandler(t *testing.T, gotPaths *[]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if gotPaths != nil {
			*gotPaths = append(*gotPaths, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/factory-sessions/session-beta/partial-result":
			if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionPartialResult{
				PartialResultArtifactRef: &factoryapi.FactoryArtifactRef{
					Id:   "artifact-partial",
					Kind: factoryapi.FactoryArtifactKindFINDING,
				},
				SessionId: "session-beta",
			}); err != nil {
				t.Fatalf("encode partial result: %v", err)
			}
		case "/factory-sessions/session-beta/result":
			if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionResult{
				ResultArtifactRef: &factoryapi.FactoryArtifactRef{
					Id:   "artifact-final",
					Kind: factoryapi.FactoryArtifactKindFINALRESULT,
				},
				SessionId: "session-beta",
				Status:    factoryapi.FactorySessionStatusIDLE,
			}); err != nil {
				t.Fatalf("encode result: %v", err)
			}
		default:
			if err := json.NewEncoder(w).Encode(sampleFactorySession()); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		}
	}
}

func sampleFactorySession() factoryapi.FactorySession {
	phase := "review"
	summary := "saved plan checkpoint"
	label := "plan"
	dispatchLabel := "review child"
	artifactLabel := "review output"
	startedAt := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 8, 14, 5, 0, 0, time.UTC)
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
			Lifecycle: factoryapi.FactorySessionLifecycle{
				StartedAt: startedAt,
				UpdatedAt: updatedAt,
			},
			Artifacts: &[]factoryapi.FactoryArtifact{{
				Id:         "artifact-1",
				Kind:       factoryapi.FactoryArtifactKindCHILDRESULT,
				Label:      &artifactLabel,
				Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
			}},
			Dispatches: &[]factoryapi.FactoryDispatch{{
				DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
				Id:           "dispatch-1",
				Label:        &dispatchLabel,
				Status:       factoryapi.FactoryDispatchStatus("RECONCILED"),
			}},
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
			Lifecycle: factoryapi.FactorySessionLifecycle{
				StartedAt: time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2026, 6, 8, 14, 5, 0, 0, time.UTC),
			},
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
