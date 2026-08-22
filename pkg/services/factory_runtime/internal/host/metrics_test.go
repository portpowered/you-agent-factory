package host_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
	names  []string
	fields []factoryruntime.Fields
}

func (sink *hostMetricsSinkFake) Counter(_ context.Context, name string, _ float64, fields factoryruntime.Fields) error {
	sink.names = append(sink.names, name)
	sink.fields = append(sink.fields, fields)
	return nil
}

func (sink *hostMetricsSinkFake) Gauge(_ context.Context, name string, _ float64, fields factoryruntime.Fields) error {
	sink.names = append(sink.names, name)
	sink.fields = append(sink.fields, fields)
	return nil
}

func (sink *hostMetricsSinkFake) Sample(_ context.Context, name string, _ float64, _ string, fields factoryruntime.Fields) error {
	sink.names = append(sink.names, name)
	sink.fields = append(sink.fields, fields)
	return nil
}

func (sink *hostMetricsSinkFake) Close() error { return nil }
func (sink *hostMetricsSinkFake) Path() string { return "memory://metrics" }
func (sink *hostMetricsSinkFake) Artifact() factoryruntime.RuntimeMetricsArtifact {
	return factoryruntime.RuntimeMetricsArtifact{Path: sink.Path()}
}
