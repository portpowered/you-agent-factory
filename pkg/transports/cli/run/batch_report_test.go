package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestBuildBatchReportSortsFailuresAndUsesCanonicalReasons(t *testing.T) {
	report := buildBatchReport(factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{
			{WorkID: "work-2", Name: "second Work", WorkTypeID: "task", State: "failed", StateCategory: string(factoryruntime.StateCategoryFailed), TraceID: "trace-2"},
			{WorkID: "work-1", Name: "first Work", WorkTypeID: "task", State: "failed", StateCategory: string(factoryruntime.StateCategoryFailed), TraceID: "trace-1"},
			{WorkID: "done", Name: "successful Work", WorkTypeID: "task", State: "done", StateCategory: string(factoryruntime.StateCategoryTerminal)},
		},
		DispatchHistory: []factoryruntime.CleanInvocationDispatch{
			{Outcome: "FAILED", Reason: "second reason", Outputs: []factoryruntime.CleanInvocationWork{{WorkID: "work-2"}}},
			{Outcome: "FAILED", Reason: "first reason", Outputs: []factoryruntime.CleanInvocationWork{{WorkID: "work-1"}}},
		},
	})

	if report.Status != "FAILED" || len(report.Failures) != 2 {
		t.Fatalf("report = %#v, want two failed Work items", report)
	}
	if got := report.Failures[0]; got.WorkID != "work-1" || got.WorkName != "first Work" || got.WorkState != "task:failed" || got.Reason != "first reason" {
		t.Fatalf("first failure = %#v, want deterministic canonical details", got)
	}
	if got := report.Failures[1]; got.WorkID != "work-2" || got.Reason != "second reason" {
		t.Fatalf("second failure = %#v, want deterministic ordering", got)
	}
}

func TestReportBatchResultJSONIsParseableAndReturnsFailure(t *testing.T) {
	var output bytes.Buffer
	err := reportBatchResult(RunConfig{JSON: true, Output: &output}, factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{{
			WorkID: "work-1", Name: "failing Work", WorkTypeID: "task", State: "failed",
			StateCategory: string(factoryruntime.StateCategoryFailed),
		}},
	})
	if err == nil {
		t.Fatal("reportBatchResult() error = nil, want batch failure")
	}
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || invocationErr.Code != batchFailureCode {
		t.Fatalf("error = %v, want %s InvocationError", err, batchFailureCode)
	}
	var decoded batchReport
	if decodeErr := json.Unmarshal(output.Bytes(), &decoded); decodeErr != nil {
		t.Fatalf("batch JSON = %q is not parseable: %v", output.String(), decodeErr)
	}
	if decoded.Status != "FAILED" || len(decoded.Failures) != 1 {
		t.Fatalf("decoded report = %#v, want one failure", decoded)
	}
	failure := decoded.Failures[0]
	if failure.WorkName != "failing Work" || failure.WorkState != "task:failed" || strings.TrimSpace(failure.Reason) == "" {
		t.Fatalf("decoded failure = %#v, want name, state, and actionable reason", failure)
	}
}

func TestReportBatchResultSuccessHasNoFailuresAndDoesNotReturnError(t *testing.T) {
	var output bytes.Buffer
	err := reportBatchResult(RunConfig{JSONOutput: true, Output: &output}, factoryruntime.CleanInvocationSnapshot{
		Work: []factoryruntime.CleanInvocationWork{{
			WorkID: "work-1", Name: "successful Work", WorkTypeID: "task", State: "done",
			StateCategory: string(factoryruntime.StateCategoryTerminal),
		}},
	})
	if err != nil {
		t.Fatalf("reportBatchResult() error = %v, want nil", err)
	}
	var decoded batchReport
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("success batch JSON = %q is not parseable: %v", output.String(), err)
	}
	if decoded.Status != "COMPLETED" || decoded.Failures == nil || len(decoded.Failures) != 0 {
		t.Fatalf("decoded success report = %#v, want COMPLETED with empty failures", decoded)
	}
}

func TestRunFactoryServiceAndEmitResultLeavesEngineErrorsUnclassified(t *testing.T) {
	wantErr := errors.New("engine failed before terminal report")
	err := runFactoryServiceAndEmitResult(
		context.Background(),
		RunConfig{WorkFile: "work.json"},
		stubFactoryService{run: func(context.Context) error { return wantErr }},
		resolvedRunRecordPath{},
		nil,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want original engine error", err)
	}
}
