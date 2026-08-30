package runtime_metrics_test

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestRuntimeMetricsSuccessorLineageAfterSourceLifecycleClose proves that a
// closed source recording contributes its usage row to the resumed runtime's
// selected metrics scope, while the source and successor retain distinct
// canonical identities.
func TestRuntimeMetricsSuccessorLineageAfterSourceLifecycleClose(t *testing.T) {
	factoryDir := support.ScaffoldSingleStepFactory(t, "runtime-metrics-successor-lineage")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig("codex", "gpt-5-codex"))
	home := t.TempDir()
	environment := []string{"HOME=" + home, "USERPROFILE=" + home}
	sourcePath := filepath.Join(home, "source.recording.jsonl")
	successorPath := filepath.Join(home, "successor.recording.jsonl")
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdoutWithUsage("source COMPLETE", 11, 7)},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdoutWithUsage("successor COMPLETE", 13, 9)},
	)

	source := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--record", sourcePath},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	support.SubmitDefaultSessionWork(t, source.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPointer("source-lineage-work"),
		Payload:      map[string]any{"subject": "source-lineage"},
		WorkTypeName: "task",
	})
	support.WaitForSessionTerminalStatus(t, source.URL(), factorysessions.DefaultSessionID, 30*time.Second)
	source.Stop(t)
	source.Close(t)

	sourceID, completedCount := readClosedRecordingLifecycle(t, sourcePath)
	if completedCount != 1 {
		t.Fatalf("closed source SESSION_COMPLETED count = %d, want exactly one", completedCount)
	}
	predecessorIDs := readUsageSessionIDs(t, home)
	if len(predecessorIDs) != 1 {
		t.Fatalf("closed source usage session IDs = %#v, want exactly one predecessor", predecessorIDs)
	}
	if _, ok := predecessorIDs[sourceID]; !ok {
		t.Fatalf("closed source usage session IDs = %#v, want durable source ID %q", predecessorIDs, sourceID)
	}

	successor := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env:                       environment,
		Args:                      []string{"--resume", sourcePath, "--record", successorPath},
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	support.SubmitDefaultSessionWork(t, successor.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPointer("successor-lineage-work"),
		Payload:      map[string]any{"subject": "successor-lineage"},
		WorkTypeName: "task",
	})
	support.WaitForSessionTerminalStatus(t, successor.URL(), factorysessions.DefaultSessionID, 30*time.Second)

	allIDs := readUsageSessionIDs(t, home)
	t.Logf("persisted usage session IDs after successor = %#v", allIDs)
	unscopedReport := support.GetJSON[factoryapi.MetricsReport](
		t,
		strings.TrimSuffix(successor.URL(), "/")+"/metrics",
	)
	t.Logf("unscoped metrics usage rows = %s", formatMetricsUsageRows(unscopedReport.UsageRows))
	report := support.GetJSON[factoryapi.MetricsReport](
		t,
		strings.TrimSuffix(successor.URL(), "/")+"/metrics?session_id="+url.QueryEscape(factorysessions.DefaultSessionID),
	)
	if report.Scope.FactorySessionId == nil || *report.Scope.FactorySessionId != factorysessions.DefaultSessionID {
		t.Fatalf("successor metrics scope = %#v, want default public Factory Session", report.Scope)
	}
	if len(report.UsageRows) != 2 {
		t.Fatalf("successor metrics usage rows = %s, want source and successor rows", formatMetricsUsageRows(report.UsageRows))
	}
	if len(allIDs) != 2 {
		t.Fatalf("source and successor usage session IDs = %#v, want two canonical IDs", allIDs)
	}
	if _, ok := allIDs[sourceID]; !ok {
		t.Fatalf("source and successor usage session IDs = %#v, want predecessor %q", allIDs, sourceID)
	}
	rowIDs := make(map[string]struct{}, len(report.UsageRows))
	for _, row := range report.UsageRows {
		if row.FactorySessionId == nil || strings.TrimSpace(*row.FactorySessionId) == "" {
			t.Fatalf("successor metrics usage row = %#v, want canonical Factory Session ID", row)
		}
		rowIDs[*row.FactorySessionId] = struct{}{}
	}
	if len(rowIDs) != 2 {
		t.Fatalf("successor metrics row IDs = %#v, want source and successor IDs", rowIDs)
	}
	for id := range allIDs {
		if _, ok := rowIDs[id]; !ok {
			t.Fatalf("successor metrics row IDs = %#v, missing persisted usage ID %q", rowIDs, id)
		}
	}
	if runner.CallCount() != 2 {
		t.Fatalf("provider command calls across source and successor = %d, want two", runner.CallCount())
	}

	successor.Stop(t)
	successor.Close(t)
	functionalevidence.Covers(t, "rest/getMetrics")
}

func readClosedRecordingLifecycle(t *testing.T, path string) (string, int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("read closed source recording %q: %v", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	sourceID := ""
	completed := 0
	lastDispatchResponseSequence := -1
	completionSequence := -1
	for scanner.Scan() {
		var record struct {
			RecordType string                          `json:"recordType"`
			SessionID  string                          `json:"sessionId"`
			Event      factorydefinitions.FactoryEvent `json:"event"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode closed source recording %q: %v", path, err)
		}
		if record.RecordType == "header" {
			sourceID = strings.TrimSpace(record.SessionID)
			continue
		}
		if record.RecordType != "event" {
			continue
		}
		if record.Event.Type == factorydefinitions.FactoryEventTypeDispatchResponse && record.Event.Context.Sequence > lastDispatchResponseSequence {
			lastDispatchResponseSequence = record.Event.Context.Sequence
		}
		if record.Event.Type != factorydefinitions.FactoryEventTypeSessionCompleted {
			continue
		}
		completed++
		completionSequence = record.Event.Context.Sequence
		if record.Event.Context.SessionID == nil || strings.TrimSpace(*record.Event.Context.SessionID) != factorysessions.DefaultSessionID {
			t.Fatalf("closed source SESSION_COMPLETED session ID = %#v, want detached public scope %q", record.Event.Context.SessionID, factorysessions.DefaultSessionID)
		}
		var payload factorydefinitions.FactorySessionCompletedEventPayload
		if err := record.Event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode closed source SESSION_COMPLETED payload: %v", err)
		}
		if payload.FinalStatus != factorydefinitions.FactorySessionLifecycleStatusSucceeded {
			t.Fatalf("closed source SESSION_COMPLETED final status = %q, want SUCCEEDED", payload.FinalStatus)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan closed source recording %q: %v", path, err)
	}
	if sourceID == "" {
		t.Fatalf("closed source recording %q has no SESSION_COMPLETED session ID", path)
	}
	if lastDispatchResponseSequence < 0 {
		t.Fatalf("closed source recording %q has no DISPATCH_RESPONSE before completion", path)
	}
	if completionSequence <= lastDispatchResponseSequence {
		t.Fatalf("closed source SESSION_COMPLETED sequence = %d, want after final DISPATCH_RESPONSE sequence %d", completionSequence, lastDispatchResponseSequence)
	}
	return sourceID, completed
}

func readUsageSessionIDs(t *testing.T, home string) map[string]struct{} {
	t.Helper()
	reader, err := platformmetrics.NewRuntimeMetricsReader(platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("construct runtime metrics reader: %v", err)
	}
	records, err := reader.Read(t.Context(), platformmetrics.RuntimeMetricsRoot(home))
	if err != nil {
		t.Fatalf("read runtime metrics: %v", err)
	}
	ids := make(map[string]struct{})
	for _, record := range records {
		name, _ := record["metric_name"].(string)
		if name != "provider.input_tokens" && name != "provider.output_tokens" {
			continue
		}
		id, _ := record["session_id"].(string)
		if id = strings.TrimSpace(id); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func formatMetricsUsageRows(rows []factoryapi.MetricsUsageRow) string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		id := ""
		if row.FactorySessionId != nil {
			id = *row.FactorySessionId
		}
		model := ""
		if row.Model != nil {
			model = *row.Model
		}
		values = append(values, id+"/"+model)
	}
	return strings.Join(values, ",")
}

func stringPointer(value string) *string {
	return &value
}
