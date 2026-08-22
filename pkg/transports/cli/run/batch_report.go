package run

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const batchFailureCode = "RUN_BATCH_FAILED"

type batchReportProvider interface {
	CleanInvocationSnapshot(context.Context) (factoryruntime.CleanInvocationSnapshot, error)
}

type batchReport struct {
	Status   string         `json:"status"`
	Failures []batchFailure `json:"failures"`
}

type batchFailure struct {
	WorkID    string `json:"workId,omitempty"`
	WorkName  string `json:"workName"`
	WorkState string `json:"workState"`
	Reason    string `json:"reason"`
}

func reportBatchResult(
	cfg RunConfig,
	snapshot factoryruntime.CleanInvocationSnapshot,
) error {
	report := buildBatchReport(snapshot)
	output := cfg.Output
	if output == nil {
		output = cfg.StartupOutput
	}
	if output == nil {
		return fmt.Errorf("write batch result: process output is required")
	}

	if cfg.JSON || cfg.JSONOutput {
		if err := json.NewEncoder(output).Encode(report); err != nil {
			return fmt.Errorf("write batch JSON result: %w", err)
		}
	} else if err := writeHumanBatchReport(output, report); err != nil {
		return err
	}

	if len(report.Failures) == 0 {
		return nil
	}
	return &InvocationError{
		Code:    batchFailureCode,
		Message: batchFailureMessage(report.Failures),
	}
}

func buildBatchReport(snapshot factoryruntime.CleanInvocationSnapshot) batchReport {
	failuresByKey := make(map[string]batchFailure)
	for _, work := range snapshot.Work {
		if work.StateCategory != string(factoryruntime.StateCategoryFailed) {
			continue
		}
		failure := batchFailure{
			WorkID:    strings.TrimSpace(work.WorkID),
			WorkName:  batchWorkName(work),
			WorkState: batchWorkState(work),
			Reason:    batchFailureReason(work, snapshot.DispatchHistory),
		}
		key := failure.WorkID
		if key == "" {
			key = failure.WorkName + "\x00" + failure.WorkState
		}
		if current, exists := failuresByKey[key]; !exists || batchFailureLess(failure, current) {
			failuresByKey[key] = failure
		}
	}

	failures := make([]batchFailure, 0, len(failuresByKey))
	for _, failure := range failuresByKey {
		failures = append(failures, failure)
	}
	sort.Slice(failures, func(i, j int) bool {
		return batchFailureLess(failures[i], failures[j])
	})
	status := "COMPLETED"
	if len(failures) > 0 {
		status = "FAILED"
	}
	return batchReport{Status: status, Failures: failures}
}

func batchFailureLess(left, right batchFailure) bool {
	if left.WorkID != right.WorkID {
		return left.WorkID < right.WorkID
	}
	if left.WorkName != right.WorkName {
		return left.WorkName < right.WorkName
	}
	if left.WorkState != right.WorkState {
		return left.WorkState < right.WorkState
	}
	return left.Reason < right.Reason
}

func batchWorkName(work factoryruntime.CleanInvocationWork) string {
	if name := strings.TrimSpace(work.Name); name != "" {
		return name
	}
	if workID := strings.TrimSpace(work.WorkID); workID != "" {
		return workID
	}
	return "<unnamed Work>"
}

func batchWorkState(work factoryruntime.CleanInvocationWork) string {
	state := strings.TrimSpace(work.State)
	if state == "" {
		state = strings.ToLower(strings.TrimSpace(work.StateCategory))
	}
	if state == "" {
		state = "failed"
	}
	workTypeID := strings.TrimSpace(work.WorkTypeID)
	if workTypeID == "" {
		return state
	}
	return workTypeID + ":" + state
}

func batchFailureReason(
	work factoryruntime.CleanInvocationWork,
	dispatches []factoryruntime.CleanInvocationDispatch,
) string {
	for index := len(dispatches) - 1; index >= 0; index-- {
		dispatch := dispatches[index]
		if dispatch.Outcome != "FAILED" || !batchDispatchMatches(work, dispatch) {
			continue
		}
		if reason := strings.TrimSpace(dispatch.Reason); reason != "" {
			return reason
		}
		if failureType := strings.TrimSpace(dispatch.FailureType); failureType != "" {
			return "worker dispatch failed (" + failureType + ")"
		}
	}
	return "Work reached a failed terminal state; inspect the latest dispatch for recovery guidance."
}

func batchDispatchMatches(
	work factoryruntime.CleanInvocationWork,
	dispatch factoryruntime.CleanInvocationDispatch,
) bool {
	for _, candidate := range dispatch.Consumed {
		if batchWorkMatches(work, candidate) {
			return true
		}
	}
	for _, candidate := range dispatch.Outputs {
		if batchWorkMatches(work, candidate) {
			return true
		}
	}
	return false
}

func batchWorkMatches(left, right factoryruntime.CleanInvocationWork) bool {
	if left.WorkID != "" && right.WorkID != "" {
		return left.WorkID == right.WorkID
	}
	if left.TraceID != "" && right.TraceID != "" {
		return left.TraceID == right.TraceID
	}
	return left.Name != "" && left.Name == right.Name && left.WorkTypeID == right.WorkTypeID
}

func writeHumanBatchReport(output io.Writer, report batchReport) error {
	if len(report.Failures) == 0 {
		if _, err := fmt.Fprintln(output, "Batch completed successfully."); err != nil {
			return fmt.Errorf("write batch result: %w", err)
		}
		return nil
	}
	if _, err := fmt.Fprintln(output, "Batch failed:"); err != nil {
		return fmt.Errorf("write batch result: %w", err)
	}
	for _, failure := range report.Failures {
		if _, err := fmt.Fprintf(
			output,
			"Work %q reached failed terminal state %s: %s\n",
			failure.WorkName,
			failure.WorkState,
			failure.Reason,
		); err != nil {
			return fmt.Errorf("write batch result: %w", err)
		}
	}
	return nil
}

func batchFailureMessage(failures []batchFailure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		parts = append(parts, fmt.Sprintf(
			"Work %q reached %s: %s",
			failure.WorkName,
			failure.WorkState,
			failure.Reason,
		))
	}
	return strings.Join(parts, "; ")
}
