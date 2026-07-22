package factorysession

import (
	"testing"
	"time"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInternalHelperConversions(t *testing.T) {
	t.Run("session budgets", testSessionBudgetConversions)
	t.Run("session usage", testSessionUsageConversions)
	t.Run("provider session refs", testProviderSessionRefConversions)
}

func testSessionBudgetConversions(t *testing.T) {
	t.Helper()

	if sessionBudgetsToAPI(nil) != nil {
		t.Fatal("sessionBudgetsToAPI(nil) should be nil")
	}
	if sessionBudgetsToAPI(&factorysessionexecution.SessionBudgets{MaxAgents: 0}) != nil {
		t.Fatal("zero maxAgents should be omitted")
	}
	mapped := sessionBudgetsToAPI(&factorysessionexecution.SessionBudgets{MaxAgents: 3})
	if mapped == nil || mapped.MaxAgents == nil || *mapped.MaxAgents != 3 {
		t.Fatalf("mapped budgets = %#v", mapped)
	}

	if sessionBudgetsFromAPI(nil) != nil {
		t.Fatal("sessionBudgetsFromAPI(nil) should be nil")
	}
	zero := 0
	if sessionBudgetsFromAPI(&factoryapi.FactorySessionBudgets{MaxAgents: &zero}) != nil {
		t.Fatal("non-positive maxAgents should be omitted")
	}
	five := 5
	roundTrip := sessionBudgetsFromAPI(&factoryapi.FactorySessionBudgets{MaxAgents: &five})
	if roundTrip == nil || roundTrip.MaxAgents != 5 {
		t.Fatalf("roundTrip budgets = %#v", roundTrip)
	}
}

func testSessionUsageConversions(t *testing.T) {
	t.Helper()

	empty := sessionUsageToAPI(factorysessionexecution.SessionUsage{})
	if empty.Resources == nil || len(empty.Resources) != 0 {
		t.Fatalf("empty usage = %#v", empty)
	}
	usage := sessionUsageToAPI(factorysessionexecution.SessionUsage{
		Resources: []factorysessionexecution.ResourceUsage{{Name: "gpu", Available: 1, Total: 2}},
	})
	if len(usage.Resources) != 1 || usage.Resources[0].Name != "gpu" {
		t.Fatalf("usage = %#v", usage)
	}

	fromNil := sessionUsageFromAPI(nil)
	if fromNil.Resources == nil || len(fromNil.Resources) != 0 {
		t.Fatalf("nil usage round-trip = %#v", fromNil)
	}
	back := sessionUsageFromAPI(&factoryapi.FactorySessionUsage{
		Resources: []factoryapi.ResourceUsage{{Name: "cpu", Available: 4, Total: 8}},
	})
	if len(back.Resources) != 1 || back.Resources[0].Name != "cpu" {
		t.Fatalf("usage back = %#v", back)
	}
}

func testProviderSessionRefConversions(t *testing.T) {
	t.Helper()

	if providerSessionRefsToAPI(nil) != nil {
		t.Fatal("providerSessionRefsToAPI(nil) should be nil")
	}
	refs := providerSessionRefsToAPI([]factorysessionexecution.ProviderSessionRef{{
		Provider: "openai",
		Kind:     "RESPONSES",
		ID:       "prov-1",
	}})
	if refs == nil || len(*refs) != 1 || (*refs)[0].Id != "prov-1" {
		t.Fatalf("refs = %#v", refs)
	}
	if providerSessionRefsFromAPI(nil) != nil {
		t.Fatal("providerSessionRefsFromAPI(nil) should be nil")
	}
	back := providerSessionRefsFromAPI(refs)
	if len(back) != 1 || back[0].ID != "prov-1" {
		t.Fatalf("refs back = %#v", back)
	}
}

func TestInternalProjectionHelpers(t *testing.T) {
	t.Run("result availability", testResultAvailabilityProjection)
	t.Run("dispatch usage warnings and failure", testDispatchUsageWarningsAndFailureProjection)
	t.Run("dispatch detail helpers", testDispatchDetailProjectionHelpers)
	t.Run("artifact helpers", testArtifactProjectionHelpers)
}

func testResultAvailabilityProjection(t *testing.T) {
	t.Helper()

	if resultAvailabilityToAPI(nil) != nil {
		t.Fatal("resultAvailabilityToAPI(nil) should be nil")
	}
	if resultAvailabilityToAPI(&factorysessionexecution.ResultAvailabilityDetail{}) != nil {
		t.Fatal("empty availability should be omitted")
	}
	mapped := resultAvailabilityToAPI(&factorysessionexecution.ResultAvailabilityDetail{
		Reason:    " RESULT_NOT_READY ",
		Message:   " still running ",
		Retryable: true,
	})
	if mapped == nil || mapped.Reason == nil || *mapped.Reason != "RESULT_NOT_READY" {
		t.Fatalf("availability = %#v", mapped)
	}
}

func testDispatchUsageWarningsAndFailureProjection(t *testing.T) {
	t.Helper()

	if dispatchUsageToAPI(nil) != nil {
		t.Fatal("dispatchUsageToAPI(nil) should be nil")
	}
	if dispatchUsageToAPI(&factorysessionexecution.DispatchUsage{}) != nil {
		t.Fatal("empty usage should be omitted")
	}
	usage := dispatchUsageToAPI(&factorysessionexecution.DispatchUsage{
		InputTokens:    1,
		OutputTokens:   2,
		TotalTokens:    3,
		DurationMillis: 4,
		CostUSD:        5.5,
		RetryCount:     6,
	})
	if usage == nil || usage.TotalTokens == nil || *usage.TotalTokens != 3 {
		t.Fatalf("usage = %#v", usage)
	}

	if dispatchWarningsToAPI(nil) != nil {
		t.Fatal("dispatchWarningsToAPI(nil) should be nil")
	}
	warnings := dispatchWarningsToAPI([]factorysessionexecution.DispatchWarning{{Code: "WARN", Message: " trimmed "}})
	if warnings == nil || len(*warnings) != 1 || (*warnings)[0].Message != "trimmed" {
		t.Fatalf("warnings = %#v", warnings)
	}

	if dispatchFailureToAPI(nil) != nil {
		t.Fatal("dispatchFailureToAPI(nil) should be nil")
	}
	if dispatchFailureToAPI(&factorysessionexecution.DispatchFailureDetail{}) != nil {
		t.Fatal("empty failure should be omitted")
	}
	failure := dispatchFailureToAPI(&factorysessionexecution.DispatchFailureDetail{
		Reason:  " TEMP ",
		Message: " msg ",
	})
	if failure == nil || failure.Reason != factoryapi.WorkFailureTypeUnknown {
		t.Fatalf("failure = %#v", failure)
	}
}

func testDispatchDetailProjectionHelpers(t *testing.T) {
	t.Helper()

	if dispatchPetriToAPI(nil) != nil {
		t.Fatal("dispatchPetriToAPI(nil) should be nil")
	}
	petri := dispatchPetriToAPI(&factorysessionexecution.DispatchPetriProjection{
		TransitionID:    "trans-1",
		WorkstationName: " ws ",
		WorkerType:      " worker ",
	})
	if petri == nil || petri.WorkstationName == nil || *petri.WorkstationName != "ws" {
		t.Fatalf("petri = %#v", petri)
	}

	if dispatchJavaScriptToAPI(nil) != nil {
		t.Fatal("dispatchJavaScriptToAPI(nil) should be nil")
	}
	js := dispatchJavaScriptToAPI(&factorysessionexecution.DispatchJavaScriptProjection{
		TaskKind:      "VERIFY",
		TaskLabel:     " review ",
		ExecutionMode: " live ",
	})
	if js == nil || js.TaskLabel == nil || *js.TaskLabel != "review" {
		t.Fatalf("javascript = %#v", js)
	}
	if js.ExecutionMode == nil || *js.ExecutionMode != "live" {
		t.Fatalf("javascript execution mode = %#v", js)
	}

	if dispatchStatusTransitionsToAPI(nil) != nil {
		t.Fatal("dispatchStatusTransitionsToAPI(nil) should be nil")
	}
	transitions := dispatchStatusTransitionsToAPI([]factorysessionexecution.DispatchStatus{
		factorysessionexecution.DispatchStatus("QUEUED"),
		" RUNNING ",
		"",
	})
	if transitions == nil || len(*transitions) != 2 {
		t.Fatalf("status transitions = %#v", transitions)
	}
}

func testArtifactProjectionHelpers(t *testing.T) {
	t.Run("retrieval and capture metadata", testArtifactRetrievalAndMetadataHelpers)
	t.Run("redaction counts and slice helpers", testArtifactCountAndSliceHelpers)
}

func testArtifactRetrievalAndMetadataHelpers(t *testing.T) {
	t.Helper()

	if artifactRetrievalRefToAPI(nil) != nil {
		t.Fatal("artifactRetrievalRefToAPI(nil) should be nil")
	}
	if artifactRetrievalRefToAPI(&factorysessionexecution.ArtifactRetrievalRef{Href: " "}) != nil {
		t.Fatal("blank href should be omitted")
	}
	ref := artifactRetrievalRefToAPI(&factorysessionexecution.ArtifactRetrievalRef{
		Href:   "/artifacts/1",
		Method: " GET ",
	})
	if ref == nil || ref.Method == nil || ref.Href != "/artifacts/1" {
		t.Fatalf("ref = %#v", ref)
	}

	now := time.Date(2026, time.June, 17, 10, 0, 0, 0, time.UTC)
	metadata := artifactCaptureMetadataToAPI(map[string]any{
		"capturedAt":       now,
		"sourceDispatchId": []byte("disp-1"),
		"mimeType":         " text/plain ",
	})
	if metadata == nil || metadata.SourceDispatchId == nil || *metadata.SourceDispatchId != "disp-1" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if artifactCaptureMetadataToAPI(map[string]any{"capturedAt": "bad"}) != nil {
		t.Fatal("invalid-only metadata should be omitted")
	}
	if stringValueFromAny(42) != "" {
		t.Fatal("non-string value should map to empty string")
	}
}

func testArtifactCountAndSliceHelpers(t *testing.T) {
	t.Helper()

	if artifactRedactionCountsToAPI(nil) != nil {
		t.Fatal("artifactRedactionCountsToAPI(nil) should be nil")
	}
	if artifactRedactionCountsToAPI(&factorysessionexecution.ArtifactRedactionCounts{}) != nil {
		t.Fatal("zero redaction counts should be omitted")
	}
	counts := artifactRedactionCountsToAPI(&factorysessionexecution.ArtifactRedactionCounts{
		Paths:   1,
		Secrets: 2,
		Tokens:  3,
	})
	if counts == nil || counts.Tokens == nil || *counts.Tokens != 3 {
		t.Fatalf("counts = %#v", counts)
	}

	if stringSlicePtr(nil) != nil {
		t.Fatal("stringSlicePtr(nil) should be nil")
	}
	slice := stringSlicePtr([]string{" a ", "", "b"})
	if slice == nil || len(*slice) != 2 || (*slice)[0] != "a" || (*slice)[1] != "b" {
		t.Fatalf("slice = %#v", slice)
	}
}

func TestInternalProjectionResponses_CoverOptionalBranches(t *testing.T) {
	t.Run("result response", testResultResponseOptionalBranches)
	t.Run("dispatch responses", testDispatchResponseOptionalBranches)
	t.Run("artifact responses", testArtifactResponseOptionalBranches)
}

func testResultResponseOptionalBranches(t *testing.T) {
	t.Helper()

	result := ResultResponseToAPI(factorysessionexecution.ResultReadResult{
		SessionID:        "dur-sess-1",
		ResultStatus:     factorysessionexecution.ResultStatus("FINAL"),
		SessionStatus:    factorysessionexecution.LifecycleStatusSucceeded,
		Mode:             factorysessionexecution.ResultModePartial,
		IncludeArtifacts: true,
		PrimaryResult:    []byte(`[{"type":"text","text":"done"}]`),
		ArtifactIDs:      []string{" a ", " ", "b"},
		ArtifactRefs: []factorysessionexecution.ArtifactRefSummary{{
			ID:         "art-1",
			Kind:       "LOG",
			Visibility: "PUBLIC",
		}},
		Failure: &factorysessionexecution.FailureSummary{
			Reason:                 "FAILED",
			Message:                "detail",
			PartialResultAvailable: true,
		},
		Availability: &factorysessionexecution.ResultAvailabilityDetail{
			Reason:    "READY",
			Message:   "ok",
			Retryable: true,
		},
	})
	if result.PrimaryResult == nil || result.ArtifactRefs == nil || result.FailureDetail == nil || result.Availability == nil {
		t.Fatalf("result response = %#v", result)
	}
}

func testDispatchResponseOptionalBranches(t *testing.T) {
	t.Helper()

	dispatchList := ListDispatchesResponseToAPI(factorysessionexecution.ListDispatchesResult{
		SessionID: "dur-sess-1",
		Dispatches: []factorysessionexecution.DispatchSummary{{
			ID:           "disp-1",
			Status:       factorysessionexecution.DispatchStatus("FAILED"),
			DispatchKind: "JAVASCRIPT_AGENT",
			Phase:        " run ",
			Label:        " task ",
			Attempt:      2,
			RunnerID:     "runner-1",
			Model:        "gpt",
			Provider:     "openai",
			ProviderSessionRefs: []factorysessionexecution.ProviderSessionRef{{
				Provider: "openai",
				Kind:     "RESPONSES",
				ID:       "prov-1",
			}},
			OutputArtifactIDs: []string{" art-1 "},
			Usage: &factorysessionexecution.DispatchUsage{
				InputTokens:    1,
				OutputTokens:   2,
				TotalTokens:    3,
				DurationMillis: 4,
				CostUSD:        5.5,
				RetryCount:     1,
			},
			Warnings:      []factorysessionexecution.DispatchWarning{{Code: "WARN", Message: " note "}},
			FailureDetail: &factorysessionexecution.DispatchFailureDetail{Reason: "FAIL", Message: "msg"},
		}},
	})
	if len(dispatchList.Dispatches) != 1 || dispatchList.Dispatches[0].Usage == nil || dispatchList.Dispatches[0].FailureDetail == nil {
		t.Fatalf("dispatch list = %#v", dispatchList)
	}

	detail := DispatchDetailResponseToAPI(factorysessionexecution.DispatchDetail{
		DispatchSummary: factorysessionexecution.DispatchSummary{
			ID:           "disp-1",
			Status:       factorysessionexecution.DispatchStatus("COMPLETED"),
			DispatchKind: "PETRI",
			Usage:        &factorysessionexecution.DispatchUsage{TotalTokens: 9},
		},
		SessionID:        "dur-sess-1",
		OrchestratorKind: "PETRI",
		Petri: &factorysessionexecution.DispatchPetriProjection{
			TransitionID:    "transition-1",
			WorkstationName: "triage",
			WorkerType:      "agent",
		},
		JavaScript: &factorysessionexecution.DispatchJavaScriptProjection{
			TaskKind:  "VERIFY",
			TaskLabel: "review",
		},
	})
	if detail.Petri == nil || detail.Javascript == nil || detail.Usage == nil {
		t.Fatalf("dispatch detail = %#v", detail)
	}
}

func testArtifactResponseOptionalBranches(t *testing.T) {
	t.Helper()

	createdAt := time.Date(2026, time.June, 17, 10, 0, 0, 0, time.UTC)
	artifacts := ListArtifactsResponseToAPI(factorysessionexecution.ListArtifactsResult{
		SessionID: "dur-sess-1",
		Artifacts: []factorysessionexecution.ArtifactSummary{{
			ID:          "art-1",
			Kind:        "LOG",
			Visibility:  "PUBLIC",
			Label:       "log",
			ContentHash: "hash",
			SizeBytes:   12,
			CreatedAt:   &createdAt,
			DispatchID:  "disp-1",
			AuditMode:   "SUMMARY",
			RedactionCounts: &factorysessionexecution.ArtifactRedactionCounts{
				Paths: 1,
			},
			RetrievalRef: &factorysessionexecution.ArtifactRetrievalRef{Href: "/artifacts/1", Method: "GET"},
		}},
	})
	if len(artifacts.Artifacts) != 1 || artifacts.Artifacts[0].RetrievalRef == nil || artifacts.Artifacts[0].RedactionCounts == nil {
		t.Fatalf("artifacts response = %#v", artifacts)
	}

	artifactDetail := ArtifactDetailResponseToAPI(factorysessionexecution.ArtifactDetail{
		ArtifactSummary: factorysessionexecution.ArtifactSummary{
			ID:         "art-1",
			Kind:       "LOG",
			Visibility: "PUBLIC",
		},
		SessionID: "dur-sess-1",
		Summary:   "capture",
		Content:   []byte(`[{"type":"text","text":"body"}]`),
		ContentRef: &factorysessionexecution.ArtifactRetrievalRef{
			Href:   "/artifacts/1/content",
			Method: "GET",
		},
		CaptureMetadata: map[string]any{
			"capturedAt":       createdAt.Format(time.RFC3339),
			"sourceDispatchId": "disp-1",
			"mimeType":         "text/plain",
		},
	})
	if artifactDetail.Content == nil || artifactDetail.ContentRef == nil || artifactDetail.CaptureMetadata == nil {
		t.Fatalf("artifact detail response = %#v", artifactDetail)
	}
}
