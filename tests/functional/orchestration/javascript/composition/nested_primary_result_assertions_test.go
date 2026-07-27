package composition_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type nestedPipelineStage struct {
	Index  int             `json:"index"`
	Status string          `json:"status"`
	Result json.RawMessage `json:"result"`
}

type nestedPipelineItemResult struct {
	Status string                `json:"status"`
	Stages []nestedPipelineStage `json:"stages"`
}

type nestedCompletionEvidence struct {
	ItemStatus                  string                   `json:"itemStatus"`
	StageCount                  int                      `json:"stageCount"`
	NestedParallelLabels        []string                 `json:"nestedParallelLabels"`
	StageOneParallelDispatchIDs []string                 `json:"stageOneParallelDispatchIds"`
	ReviewDispatchFromStageTwo  string                   `json:"reviewDispatchFromStageTwo"`
	Results                     []nestedPipelineItemResult `json:"results"`
}

type nestedFailureEvidence struct {
	ItemStatus                  string                   `json:"itemStatus"`
	NestedFailureStageIndex     int                      `json:"nestedFailureStageIndex"`
	NestedFailureStageStatus    string                   `json:"nestedFailureStageStatus"`
	FailedNestedChildLabel      string                   `json:"failedNestedChildLabel"`
	FailedNestedChildStatus     string                   `json:"failedNestedChildStatus"`
	FailedNestedChildDiagnostic string                   `json:"failedNestedChildDiagnostic"`
	Results                     []nestedPipelineItemResult `json:"results"`
}

func decodeNestedPrimaryResultJSON(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
) []byte {
	t.Helper()
	if result == nil || result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want FINAL Factory Session result", result)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode primary result JSON: %v", err)
	}
	return encoded
}

func assertNestedCompletionPrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	alphaDispatchID string,
	betaDispatchID string,
	reviewDispatchID string,
) {
	t.Helper()
	encoded := decodeNestedPrimaryResultJSON(t, result)
	var evidence nestedCompletionEvidence
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode nested completion primary result: %v", err)
	}
	assertNestedCompletionEvidence(
		t, evidence, alphaDispatchID, betaDispatchID, reviewDispatchID,
	)
}

func assertNestedCompletionEvidence(
	t *testing.T,
	evidence nestedCompletionEvidence,
	alphaDispatchID string,
	betaDispatchID string,
	reviewDispatchID string,
) {
	t.Helper()
	if evidence.ItemStatus != "COMPLETED" || evidence.StageCount != 2 {
		t.Fatalf(
			"nested completion evidence = %#v, want one completed item with two pipeline stages",
			evidence,
		)
	}
	wantParallelLabels := []string{nestedParallelAlphaLabel, nestedParallelBetaLabel}
	if len(evidence.NestedParallelLabels) != len(wantParallelLabels) {
		t.Fatalf("nested parallel labels = %#v, want %#v", evidence.NestedParallelLabels, wantParallelLabels)
	}
	for index, wantLabel := range wantParallelLabels {
		if evidence.NestedParallelLabels[index] != wantLabel {
			t.Fatalf(
				"nested parallel labels[%d] = %q, want %q",
				index,
				evidence.NestedParallelLabels[index],
				wantLabel,
			)
		}
	}
	if len(evidence.StageOneParallelDispatchIDs) != 2 {
		t.Fatalf(
			"stage-one parallel dispatch ids = %#v, want two nested parallel child ids",
			evidence.StageOneParallelDispatchIDs,
		)
	}
	if evidence.StageOneParallelDispatchIDs[0] != alphaDispatchID ||
		evidence.StageOneParallelDispatchIDs[1] != betaDispatchID {
		t.Fatalf(
			"stage-one parallel dispatch ids = %#v, want alpha=%q beta=%q",
			evidence.StageOneParallelDispatchIDs,
			alphaDispatchID,
			betaDispatchID,
		)
	}
	if evidence.ReviewDispatchFromStageTwo != reviewDispatchID {
		t.Fatalf(
			"stage-two review dispatch id = %q, want %q",
			evidence.ReviewDispatchFromStageTwo,
			reviewDispatchID,
		)
	}
	if len(evidence.Results) != 1 || evidence.Results[0].Status != "COMPLETED" {
		t.Fatalf("pipeline item results = %#v, want one completed item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want exactly 2", len(evidence.Results[0].Stages))
	}
	for index, stage := range evidence.Results[0].Stages {
		if stage.Index != index || stage.Status != "COMPLETED" {
			t.Fatalf("pipeline stage[%d] = %#v, want completed stage index %d", index, stage, index)
		}
	}
	assertNestedCompletionStageOneParallel(t, evidence.Results[0].Stages[0].Result)
}

func assertNestedCompletionStageOneParallel(t *testing.T, stageResult json.RawMessage) {
	t.Helper()
	wantParallelLabels := []string{nestedParallelAlphaLabel, nestedParallelBetaLabel}
	var stageOneParallel []struct {
		Label      string `json:"label"`
		Status     string `json:"status"`
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(stageResult, &stageOneParallel); err != nil {
		t.Fatalf("decode stage-one parallel results: %v", err)
	}
	if len(stageOneParallel) != 2 {
		t.Fatalf("stage-one parallel results = %#v, want two nested parallel child results", stageOneParallel)
	}
	for index, wantLabel := range wantParallelLabels {
		child := stageOneParallel[index]
		if child.Label != wantLabel || child.Status != "COMPLETED" {
			t.Fatalf(
				"stage-one parallel[%d] = label=%q status=%q, want label=%q status=COMPLETED",
				index,
				child.Label,
				child.Status,
				wantLabel,
			)
		}
	}
}

func assertNestedFailurePrimaryResult(
	t *testing.T,
	result *factoryapi.FactorySessionResult,
	alphaDispatchID string,
	_ string,
	reviewDispatchID string,
) {
	t.Helper()
	encoded := decodeNestedPrimaryResultJSON(t, result)
	var evidence nestedFailureEvidence
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode nested failure primary result: %v", err)
	}
	assertNestedFailureEvidenceSummary(t, evidence)
	assertNestedFailureParallelStage(t, evidence.Results[0].Stages[0], alphaDispatchID)
	assertNestedFailureReviewStage(t, evidence.Results[0].Stages[1], reviewDispatchID)
}

func assertNestedFailureEvidenceSummary(
	t *testing.T,
	evidence nestedFailureEvidence,
) {
	t.Helper()
	if evidence.NestedFailureStageIndex != nestedFailureNestedStageIndex {
		t.Fatalf(
			"nested failure stage index = %d, want pipeline stage %d",
			evidence.NestedFailureStageIndex,
			nestedFailureNestedStageIndex,
		)
	}
	if evidence.NestedFailureStageStatus != "COMPLETED" {
		t.Fatalf(
			"nested failure stage status = %q, want COMPLETED parallel stage with one failed nested child",
			evidence.NestedFailureStageStatus,
		)
	}
	if evidence.FailedNestedChildLabel != nestedParallelBetaLabel {
		t.Fatalf(
			"failed nested child label = %q, want %q",
			evidence.FailedNestedChildLabel,
			nestedParallelBetaLabel,
		)
	}
	if evidence.FailedNestedChildStatus != "FAILED" {
		t.Fatalf(
			"failed nested child status = %q, want FAILED",
			evidence.FailedNestedChildStatus,
		)
	}
	if !strings.Contains(evidence.FailedNestedChildDiagnostic, nestedFailureChildDiagnosticToken) {
		t.Fatalf(
			"failed nested child diagnostic = %q, want customer-readable token %q",
			evidence.FailedNestedChildDiagnostic,
			nestedFailureChildDiagnosticToken,
		)
	}
	if len(evidence.Results) != 1 {
		t.Fatalf("pipeline item results = %#v, want one item", evidence.Results)
	}
	if len(evidence.Results[0].Stages) != 2 {
		t.Fatalf("pipeline stage count = %d, want exactly 2 after nested parallel failure", len(evidence.Results[0].Stages))
	}
}

func assertNestedFailureParallelStage(
	t *testing.T,
	stageZero nestedPipelineStage,
	alphaDispatchID string,
) {
	t.Helper()
	if stageZero.Index != nestedFailureNestedStageIndex || stageZero.Status != "COMPLETED" {
		t.Fatalf(
			"pipeline stage[0] = index=%d status=%q, want index=%d status=COMPLETED",
			stageZero.Index,
			stageZero.Status,
			nestedFailureNestedStageIndex,
		)
	}
	var stageOneParallel []struct {
		Label      string `json:"label"`
		Status     string `json:"status"`
		Diagnostic string `json:"diagnostic"`
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(stageZero.Result, &stageOneParallel); err != nil {
		t.Fatalf("decode stage-one parallel results: %v", err)
	}
	if len(stageOneParallel) != 2 {
		t.Fatalf("stage-one parallel results = %#v, want two nested parallel child results", stageOneParallel)
	}
	if stageOneParallel[0].Label != nestedParallelAlphaLabel || stageOneParallel[0].Status != "COMPLETED" {
		t.Fatalf(
			"stage-one parallel[0] = label=%q status=%q, want label=%q status=COMPLETED",
			stageOneParallel[0].Label,
			stageOneParallel[0].Status,
			nestedParallelAlphaLabel,
		)
	}
	if stageOneParallel[0].DispatchID != alphaDispatchID {
		t.Fatalf(
			"stage-one parallel alpha dispatch id = %q, want %q",
			stageOneParallel[0].DispatchID,
			alphaDispatchID,
		)
	}
	failedChild := stageOneParallel[1]
	if failedChild.Label != nestedParallelBetaLabel || failedChild.Status != "FAILED" {
		t.Fatalf(
			"stage-one parallel[1] = label=%q status=%q, want label=%q status=FAILED",
			failedChild.Label,
			failedChild.Status,
			nestedParallelBetaLabel,
		)
	}
	if !strings.Contains(failedChild.Diagnostic, nestedFailureChildDiagnosticToken) {
		t.Fatalf(
			"stage-one parallel beta diagnostic = %q, want token %q",
			failedChild.Diagnostic,
			nestedFailureChildDiagnosticToken,
		)
	}
}

func assertNestedFailureReviewStage(
	t *testing.T,
	stageOne nestedPipelineStage,
	reviewDispatchID string,
) {
	t.Helper()
	if stageOne.Index != 1 || stageOne.Status != "COMPLETED" {
		t.Fatalf(
			"pipeline stage[1] = index=%d status=%q, want index=1 status=COMPLETED after nested failure",
			stageOne.Index,
			stageOne.Status,
		)
	}
	var stageTwoResult struct {
		DispatchID string `json:"dispatchId"`
	}
	if err := json.Unmarshal(stageOne.Result, &stageTwoResult); err != nil {
		t.Fatalf("decode stage-two review result: %v", err)
	}
	if stageTwoResult.DispatchID != reviewDispatchID {
		t.Fatalf(
			"stage-two review dispatch id = %q, want %q",
			stageTwoResult.DispatchID,
			reviewDispatchID,
		)
	}
}
