package session

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestShow_PerformsGETFactorySession(t *testing.T) {
	var gotPaths []string
	srv := httptest.NewServer(sessionShowTestHandler(t, &gotPaths))
	defer srv.Close()

	var out bytes.Buffer
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
		"Stop summary:\tkind=INTERRUPTED session=session-beta work=Review child [work-review-1] state=goal:review",
		"Stop dispatch:\tdispatch-1 status=INTERRUPTED kind=JAVASCRIPT_AGENT workstation=review child",
		"Stop result:\tDispatch interrupted while waiting for review output",
		"Recovery surface:\texisting dispatch retry, work repair, or session workflow controls",
		"Recovery action:\tInspect the interrupted dispatch in Factory Session \"session-beta\", then use the existing retry, repair, or session workflow controls to continue recovery.",
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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

	err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
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
			if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionLiveResult{
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
	workName := "Review child"
	workID := "work-review-1"
	workState := "goal:review"
	stopResult := "Dispatch interrupted while waiting for review output"
	recoverySurface := "existing dispatch retry, work repair, or session workflow controls"
	recoveryAction := "Inspect the interrupted dispatch in Factory Session \"session-beta\", then use the existing retry, repair, or session workflow controls to continue recovery."
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
			StopSummary: &factoryapi.FactoryStopSummary{
				SessionId:                "session-beta",
				StopKind:                 factoryapi.FactoryStopKind("INTERRUPTED"),
				WorkId:                   &workID,
				WorkName:                 &workName,
				WorkState:                &workState,
				LatestResultSummary:      &stopResult,
				SuggestedRecoverySurface: &recoverySurface,
				SuggestedRecoveryAction:  &recoveryAction,
				LatestDispatch: &factoryapi.FactoryStopDispatchSummary{
					DispatchId:      "dispatch-1",
					Status:          factoryapi.FactoryDispatchStatusINTERRUPTED,
					DispatchKind:    factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
					WorkstationName: &dispatchLabel,
				},
			},
			Artifacts: &[]factoryapi.FactoryArtifact{{
				Id:         "artifact-1",
				Kind:       factoryapi.FactoryArtifactKindCHILDRESULT,
				Label:      &artifactLabel,
				Visibility: factoryapi.FactoryArtifactVisibilityPUBLIC,
			}},
			Progress: factoryapi.FactorySessionProgress{
				FactoryState:  "RUNNING",
				Categories:    factoryapi.StatusCategories{},
				InFlightCount: 0,
				TotalTokens:   0,
			},
			Usage: factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{}},
			Javascript: &factoryapi.FactorySessionJavaScriptProjection{
				Phase:        &phase,
				Phases:       []string{"plan", "review"},
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

func TestShow_DurableSessionJSONUsesDurableReadModel(t *testing.T) {
	interruptedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/dur-sess-js-interrupted-001"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionDurableReadModel{
			SessionId:        "dur-sess-js-interrupted-001",
			Status:           factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
			OrchestratorKind: factoryapi.JAVASCRIPT,
			Lifecycle: &factoryapi.FactorySessionDurableLifecycleTimestamps{
				InterruptedAt: &interruptedAt,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show durable JSON: %v", err)
	}

	var session factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(out.Bytes(), &session); err != nil {
		t.Fatalf("decode durable JSON: %v", err)
	}
	if session.SessionId != "dur-sess-js-interrupted-001" {
		t.Fatalf("sessionId = %q", session.SessionId)
	}
	if session.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("status = %q, want INTERRUPTED", session.Status)
	}
}

func TestShow_DurableSessionHumanOutputRendersLifecycleContinuity(t *testing.T) {
	startedAt := time.Date(2026, 6, 30, 11, 55, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 30, 12, 10, 0, 0, time.UTC)
	interruptedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	resumedAt := time.Date(2026, 6, 30, 12, 5, 0, 0, time.UTC)
	sourceRef, sourceHash, phase := "workflows/review.js", "sha256:source", "review"
	checkpointLabel := "review saved"
	policyHash := "sha256:policy"
	maxAgents := 4
	total, completed, inFlight, failed := 5, 3, 1, 1
	phaseTotal, phaseCompleted, phaseFailed := 3, 2, 1
	resultSummary := "Review output is ready"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.FactorySessionDurableReadModel{
			SessionId:        "dur-sess-js-interrupted-001",
			Status:           factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			OrchestratorKind: factoryapi.JAVASCRIPT,
			ResolvedSource: factoryapi.FactorySessionResolvedSourceIdentity{
				Kind: factoryapi.FactorySessionExecutionSourceKindWorkflowFile, SourceRef: &sourceRef,
			},
			SourceHash: &sourceHash,
			Phase:      &phase,
			Lifecycle: &factoryapi.FactorySessionDurableLifecycleTimestamps{
				StartedAt:     &startedAt,
				FinishedAt:    &finishedAt,
				InterruptedAt: &interruptedAt,
				ResumedAt:     &resumedAt,
			},
			Progress: &factoryapi.FactorySessionDurableProgressCounts{
				TotalDispatches: &total, CompletedDispatches: &completed,
				InFlightDispatches: &inFlight, FailedDispatches: &failed,
			},
			PhaseSummaries: &[]factoryapi.FactorySessionDurablePhaseSummary{{
				Phase: "plan", DispatchCount: &phaseTotal, CompletedDispatchCount: &phaseCompleted, FailedDispatchCount: &phaseFailed,
			}},
			LatestCheckpoint:    &factoryapi.FactorySessionCheckpointRef{Id: "checkpoint-1", Label: &checkpointLabel, Phase: &phase},
			EffectivePolicyHash: &policyHash,
			Budgets:             &factoryapi.FactorySessionBudgets{MaxAgents: &maxAgents},
			Usage:               &factoryapi.FactorySessionUsage{Resources: []factoryapi.ResourceUsage{{Name: "agents", Total: 4, Available: 1}}},
			ArtifactRefs:        &[]factoryapi.FactoryArtifactRef{{Id: "artifact-1", Kind: factoryapi.FactoryArtifactKindFINALRESULT}},
			ResultSummary: &factoryapi.FactorySessionDurableResultSummary{
				ResultStatus: factoryapi.FactorySessionResultStatusFinal, Summary: &resultSummary,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := NewShow(testHTTPProtocol(t))(ShowConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Show durable human: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Factory session:\tdur-sess-js-interrupted-001",
		"Lifecycle status:\tSUCCEEDED",
		"Source:\tWORKFLOW_FILE ref=workflows/review.js hash=sha256:source",
		"Duration:\t15m0s",
		"Current phase:\treview",
		"Dispatch counts:\ttotal=5, completed=3, in flight=1, failed=1",
		"Latest checkpoint:\tcheckpoint-1 label=review saved phase=review",
		"Effective policy:\thash=sha256:policy",
		"Budget:\tmax agents=4",
		"Usage:\tagents=3/4",
		"Artifacts:\tartifact-1 (FINAL_RESULT)",
		"Result availability:\tFINAL — Review output is ready",
		"Phase summaries:\n- plan total=3, completed=2, failed=1",
		"Interrupted at:\t2026-06-30T12:00:00Z",
		"Resumed at:\t2026-06-30T12:05:00Z",
		"Result status:\tFINAL",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDispatches_DurableSessionJSONUsesListFactorySessionDispatchesResponse(t *testing.T) {
	label := "step-one"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/factory-sessions/dur-sess-js-interrupted-001/dispatches"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionDispatchesResponse{
			SessionId: "dur-sess-js-interrupted-001",
			Dispatches: []factoryapi.FactorySessionDispatchSummary{
				{
					Id:           "dispatch-1",
					Status:       factoryapi.FactoryDispatchStatusCOMPLETED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
					Label:        &label,
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := NewDispatches(testHTTPProtocol(t))(DispatchesConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Dispatches durable JSON: %v", err)
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(out.Bytes(), &listed); err != nil {
		t.Fatalf("decode dispatches JSON: %v", err)
	}
	if listed.SessionId != "dur-sess-js-interrupted-001" {
		t.Fatalf("sessionId = %q", listed.SessionId)
	}
	if len(listed.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", listed.Dispatches)
	}
}

func TestDispatches_DurableSessionHumanOutputRendersDispatchSummaries(t *testing.T) {
	phase, label, runner, model := "review", "Review child", "runner-1", "model-1"
	attempt := int32(2)
	duration := int64(1250)
	artifacts := []string{"artifact-1"}
	providerRefs := []factoryapi.LoadableProviderSessionRef{{Id: "provider-session-1", Kind: factoryapi.LoadableProviderSessionKind("SESSION_ID"), Provider: factoryapi.LoadableProviderSessionProvider("CLAUDE")}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionDispatchesResponse{
			SessionId: "dur-sess-js-interrupted-001",
			Dispatches: []factoryapi.FactorySessionDispatchSummary{
				{
					Id:           "dispatch-1",
					Status:       factoryapi.FactoryDispatchStatusCOMPLETED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
				},
				{
					Id:           "dispatch-2",
					Status:       factoryapi.FactoryDispatchStatusINTERRUPTED,
					DispatchKind: factoryapi.FactoryDispatchKindJAVASCRIPTAGENT,
					Phase:        &phase, Label: &label, RunnerId: &runner, Model: &model,
					ProviderSessionRefs: &providerRefs, Attempt: &attempt,
					Usage: &factoryapi.FactoryDispatchUsage{DurationMillis: &duration}, OutputArtifactIds: &artifacts,
					FailureDetail: &factoryapi.FailureDetail{Reason: factoryapi.WorkFailureType("PROVIDER_ERROR"), Message: "provider unavailable"},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	if err := NewDispatches(testHTTPProtocol(t))(DispatchesConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "dur-sess-js-interrupted-001",
		Output:    &out,
	}); err != nil {
		t.Fatalf("Dispatches durable human: %v", err)
	}

	output := out.String()
	for _, want := range []string{
		"Factory session dur-sess-js-interrupted-001 dispatches (2):",
		"- dispatch-1 COMPLETED JAVASCRIPT_AGENT",
		"- dispatch-2 INTERRUPTED JAVASCRIPT_AGENT",
		"  Phase:\treview",
		"  Label:\tReview child",
		"  Runner:\trunner-1",
		"  Model:\tmodel-1",
		"  Provider sessions:\tprovider-session-1",
		"  Attempt:\t2",
		"  Duration:\t1250ms",
		"  Artifacts:\tartifact-1",
		"  Failure:\tprovider unavailable",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestDispatches_RejectsNonDurableSessionID(t *testing.T) {
	err := NewDispatches(testHTTPProtocol(t))(DispatchesConfig{Context: context.Background(),
		Server:    "http://127.0.0.1:1",
		SessionID: "session-beta",
		Output:    ioDiscard{},
	})
	if err == nil {
		t.Fatal("expected error for non-durable session id")
	}
	if !strings.Contains(err.Error(), "dur-sess-*") {
		t.Fatalf("error = %q, want durable session requirement", err.Error())
	}
}

func TestDispatchesEndpoint_ForwardsCanonicalFilters(t *testing.T) {
	endpoint, err := dispatchesEndpoint(DispatchesConfig{Context: context.Background(),
		Server: "http://127.0.0.1:3456", SessionID: "dur-sess-filter-001",
		Phase: "build", Status: "FAILED",
	})
	if err != nil {
		t.Fatalf("dispatchesEndpoint: %v", err)
	}
	if endpoint.Query().Get("phase") != "build" || endpoint.Query().Get("status") != "FAILED" {
		t.Fatalf("query = %q, want phase=build and status=FAILED", endpoint.RawQuery)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

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
