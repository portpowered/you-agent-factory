package host_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestEmitRuntimeMemoryMetricsRecordsOneCoherentRuntimeSnapshot(t *testing.T) {
	t.Parallel()

	sink := &hostMetricsSinkFake{}
	bundle := &factoryhost.Bundle{MetricsSink: sink}
	bundle.EmitRuntimeMemoryMetrics()

	values, units := runtimeMemorySinkRecords(t, sink)
	assertRuntimeMemoryUnits(t, units)
	assertRuntimeMemoryValues(t, values)
}

func runtimeMemorySinkRecords(t *testing.T, sink *hostMetricsSinkFake) (map[string]float64, map[string]string) {
	t.Helper()
	if len(sink.names) != 7 {
		t.Fatalf("runtime memory metric count = %d, want 7; names = %#v", len(sink.names), sink.names)
	}
	values := make(map[string]float64, len(sink.names))
	units := make(map[string]string, len(sink.names))
	for index, name := range sink.names {
		if sink.kinds[index] != factoryruntime.RuntimeMetricTypeSample {
			t.Fatalf("metric %s type = %q, want sample", name, sink.kinds[index])
		}
		values[name] = sink.values[index]
		units[name] = sink.units[index]
	}
	for _, name := range runtimeMemoryMetricNames() {
		if _, ok := values[name]; !ok {
			t.Fatalf("runtime memory metric %q was not emitted; values = %#v", name, values)
		}
	}
	return values, units
}

func runtimeMemoryMetricNames() []string {
	return []string{
		factoryruntime.RuntimeMemoryHeapAlloc,
		factoryruntime.RuntimeMemoryHeapInuse,
		factoryruntime.RuntimeMemorySys,
		factoryruntime.RuntimeMemoryNumGC,
		factoryruntime.RuntimeMemoryGoroutines,
		factoryruntime.RuntimeMemoryProcessCommit,
		factoryruntime.RuntimeMemoryProcessCommitAvailable,
	}
}

func assertRuntimeMemoryUnits(t *testing.T, units map[string]string) {
	t.Helper()
	want := map[string]string{
		factoryruntime.RuntimeMemoryHeapAlloc:              "bytes",
		factoryruntime.RuntimeMemoryHeapInuse:              "bytes",
		factoryruntime.RuntimeMemorySys:                    "bytes",
		factoryruntime.RuntimeMemoryNumGC:                  "count",
		factoryruntime.RuntimeMemoryGoroutines:             "count",
		factoryruntime.RuntimeMemoryProcessCommit:          "bytes",
		factoryruntime.RuntimeMemoryProcessCommitAvailable: "boolean",
	}
	for name, unit := range want {
		if units[name] != unit {
			t.Fatalf("runtime memory unit for %s = %q, want %q", name, units[name], unit)
		}
	}
}

func assertRuntimeMemoryValues(t *testing.T, values map[string]float64) {
	t.Helper()
	heapAlloc := values[factoryruntime.RuntimeMemoryHeapAlloc]
	heapInuse := values[factoryruntime.RuntimeMemoryHeapInuse]
	sys := values[factoryruntime.RuntimeMemorySys]
	if heapAlloc <= 0 || heapInuse < heapAlloc || sys < heapInuse {
		t.Fatalf("runtime memory heap values = alloc:%v inuse:%v sys:%v, want positive and ordered", heapAlloc, heapInuse, sys)
	}
	if values[factoryruntime.RuntimeMemoryNumGC] < 0 {
		t.Fatalf("NumGC metric = %v, want a non-negative value", values[factoryruntime.RuntimeMemoryNumGC])
	}
	if values[factoryruntime.RuntimeMemoryGoroutines] <= 0 {
		t.Fatalf("Goroutines metric = %v, want a positive value", values[factoryruntime.RuntimeMemoryGoroutines])
	}
	commitAvailable := values[factoryruntime.RuntimeMemoryProcessCommitAvailable]
	if commitAvailable != 0 && commitAvailable != 1 {
		t.Fatalf("ProcessCommitAvailable metric = %v, want 0 or 1", commitAvailable)
	}
	if commitAvailable == 1 && values[factoryruntime.RuntimeMemoryProcessCommit] <= 0 {
		t.Fatalf("ProcessCommit metric = %v, want positive when available", values[factoryruntime.RuntimeMemoryProcessCommit])
	}
}

func TestEmitRuntimeMemoryMetricsSurfacesSinkFailureAfterCompleteAttempt(t *testing.T) {
	t.Parallel()

	sampleErr := errors.New("sink unavailable")
	sink := &hostMetricsSinkFake{
		sampleErr:     sampleErr,
		sampleErrName: factoryruntime.RuntimeMemoryProcessCommitAvailable,
	}
	bundle := &factoryhost.Bundle{MetricsSink: sink}
	err := bundle.EmitRuntimeMemoryMetrics()

	if sink.sampleCalls != 7 {
		t.Fatalf("runtime memory sample attempts = %d, want 7 despite sink errors", sink.sampleCalls)
	}
	if !errors.Is(err, sampleErr) || !strings.Contains(err.Error(), factoryruntime.RuntimeMemoryProcessCommitAvailable) {
		t.Fatalf("EmitRuntimeMemoryMetrics error = %v, want process-commit availability sample failure", err)
	}
	if finalizeErr := factoryhost.FinalizeArtifacts(bundle, clockwork.NewFakeClock()); !errors.Is(finalizeErr, sampleErr) || !strings.Contains(finalizeErr.Error(), factoryruntime.RuntimeMemoryProcessCommitAvailable) {
		t.Fatalf("FinalizeArtifacts error = %v, want retained sample failure", finalizeErr)
	}
}

func TestScriptMetricHelpers_PreferFailureMetadataAndDiagnostics(t *testing.T) {
	t.Parallel()

	timeoutResult := workerexecution.WorkResult{
		FailureMetadata: &workerexecution.WorkFailureMetadata{Type: workerexecution.WorkFailureTypeTimeout},
	}
	if !factoryhost.ScriptMetricTimedOut(timeoutResult) {
		t.Fatal("expected timeout result to report timed out")
	}
	if got := factoryhost.ScriptMetricFailureReason(timeoutResult); got != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("failure reason = %q, want %q", got, workerexecution.WorkFailureTypeTimeout)
	}

	commandResult := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeRejected,
		Diagnostics: &workerexecution.WorkDiagnostics{
			Command: &workerexecution.CommandDiagnostic{
				ExitCode: 7,
				Duration: 250 * time.Millisecond,
			},
		},
	}
	if got := factoryhost.ScriptMetricFailureReason(commandResult); got != "exit_code" {
		t.Fatalf("failure reason = %q, want exit_code", got)
	}
	if duration, ok := factoryhost.ScriptMetricDurationMilliseconds(commandResult); !ok || duration != 250 {
		t.Fatalf("command duration = %v, %v want 250, true", duration, ok)
	}

	outcomeResult := workerexecution.WorkResult{
		Outcome: workerexecution.OutcomeContinue,
		Metrics: workerexecution.WorkMetrics{Duration: 125 * time.Millisecond},
	}
	if duration, ok := factoryhost.ScriptMetricDurationMilliseconds(outcomeResult); !ok || duration != 125 {
		t.Fatalf("metrics duration = %v, %v want 125, true", duration, ok)
	}
	if got := factoryhost.ScriptMetricFailureReason(outcomeResult); got != string(workerexecution.OutcomeContinue) {
		t.Fatalf("fallback failure reason = %q, want %q", got, workerexecution.OutcomeContinue)
	}
}

func TestRecordCompletionMetricsUsesCanonicalWorkerSessionAssociation(t *testing.T) {
	t.Parallel()

	dispatchID := "dispatch-1"
	payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-1"})
	if err != nil {
		t.Fatalf("marshal worker-session association: %v", err)
	}
	ledger := &recordingfixtures.ScriptedRuntimeLedger{
		GenerationID: "metrics-worker-session",
		Events: []interfaces.FactoryEvent{{
			Type:    interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			Payload: payload,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
		}},
	}
	sink := &hostMetricsSinkFake{}
	bundle := &factoryhost.Bundle{EventHistory: ledger, MetricsSink: sink}
	bundle.RecordCompletionMetrics(interfaces.FactoryCompletionRecord{
		DispatchID: "dispatch-1",
		Result: workerexecution.WorkResult{
			DispatchID: "dispatch-1",
			Outcome:    workerexecution.OutcomeAccepted,
		},
	})

	for index, name := range sink.names {
		if name == "dispatch.completed" {
			if sink.fields[index].WorkerSessionID != "worker-session-1" {
				t.Fatalf("completion fields = %#v, want canonical Worker Session ID", sink.fields[index])
			}
			return
		}
	}
	t.Fatalf("metric names = %#v, want dispatch.completed", sink.names)
}

func TestRecordCompletionMetricsUsesResolvedProviderForDispatchFacts(t *testing.T) {
	t.Parallel()

	sink := &hostMetricsSinkFake{}
	bundle := &factoryhost.Bundle{MetricsSink: sink}
	bundle.RecordCompletionMetrics(interfaces.FactoryCompletionRecord{
		DispatchID: "dispatch-provider",
		Result: workerexecution.WorkResult{
			DispatchID: "dispatch-provider",
			Outcome:    workerexecution.OutcomeAccepted,
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{Provider: "codex"},
			},
		},
	})

	for _, name := range []string{"dispatch.completed", "dispatch.duration"} {
		found := false
		for index, emittedName := range sink.names {
			if emittedName != name {
				continue
			}
			found = true
			if sink.fields[index].Provider != "codex" {
				t.Fatalf("%s provider = %q, want resolved codex", name, sink.fields[index].Provider)
			}
		}
		if !found {
			t.Fatalf("metric names = %#v, want %s", sink.names, name)
		}
	}
}

func TestRecordCompletionMetricsDoesNotPersistUnresolvedProvider(t *testing.T) {
	t.Parallel()

	sink := &hostMetricsSinkFake{}
	bundle := &factoryhost.Bundle{MetricsSink: sink}
	bundle.RecordCompletionMetrics(interfaces.FactoryCompletionRecord{
		DispatchID: "dispatch-placeholder",
		Result: workerexecution.WorkResult{
			DispatchID: "dispatch-placeholder",
			Outcome:    workerexecution.OutcomeAccepted,
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{Provider: "${executorProvider}"},
			},
		},
	})

	for index, provider := range sink.fields {
		if provider.Provider != "" {
			t.Fatalf("metric %s provider = %q, want omitted unresolved provider", sink.names[index], provider.Provider)
		}
	}
}

type hostMetricsSinkFake struct {
	names         []string
	kinds         []string
	values        []float64
	units         []string
	fields        []factoryruntime.Fields
	sampleErr     error
	sampleErrName string
	sampleCalls   int
}

func (sink *hostMetricsSinkFake) Counter(_ context.Context, name string, value float64, fields factoryruntime.Fields) error {
	sink.names = append(sink.names, name)
	sink.kinds = append(sink.kinds, factoryruntime.RuntimeMetricTypeCounter)
	sink.values = append(sink.values, value)
	sink.units = append(sink.units, "")
	sink.fields = append(sink.fields, fields)
	return nil
}

func (sink *hostMetricsSinkFake) Gauge(_ context.Context, name string, value float64, fields factoryruntime.Fields) error {
	sink.names = append(sink.names, name)
	sink.kinds = append(sink.kinds, factoryruntime.RuntimeMetricTypeGauge)
	sink.values = append(sink.values, value)
	sink.units = append(sink.units, "")
	sink.fields = append(sink.fields, fields)
	return nil
}

func (sink *hostMetricsSinkFake) Sample(_ context.Context, name string, value float64, unit string, fields factoryruntime.Fields) error {
	sink.sampleCalls++
	sink.names = append(sink.names, name)
	sink.kinds = append(sink.kinds, factoryruntime.RuntimeMetricTypeSample)
	sink.values = append(sink.values, value)
	sink.units = append(sink.units, unit)
	sink.fields = append(sink.fields, fields)
	if sink.sampleErrName == "" || sink.sampleErrName == name {
		return sink.sampleErr
	}
	return nil
}

func (sink *hostMetricsSinkFake) Close() error { return nil }
func (sink *hostMetricsSinkFake) Path() string { return "memory://metrics" }
func (sink *hostMetricsSinkFake) Artifact() factoryruntime.RuntimeMetricsArtifact {
	return factoryruntime.RuntimeMetricsArtifact{Path: sink.Path()}
}
