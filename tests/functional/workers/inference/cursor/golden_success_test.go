package cursor

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	_ "modernc.org/sqlite"
)

const cursorGoldenSuccessWorkspaceHash = "cursor-fixture-workspace"

const cursorGoldenExpectedProviderSessionDetailFile = "expected-provider-session-detail.json"

// TestCursorGoldenTextSuccessAndSessionIdentity replays the sanitized Cursor text-success
// golden through the public process boundary and proves successful text output,
// stable Provider Session identity, and matching public metadata goldens.
// golden: tests/functional/internal/support/testdata/provider-sessions/cursor/success/manifest.json
func TestCursorGoldenTextSuccessAndSessionIdentity(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("cursor", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "cursor-text-success" {
		t.Fatalf("manifest.ID = %q, want cursor-text-success", loaded.Manifest.ID)
	}

	var request struct {
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatal("request.model must be non-empty")
	}
	if request.SessionID == "" {
		t.Fatal("request.session_id must be non-empty")
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", cursorGoldenWorkerConfig(request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"cursor golden success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload, dispatchOutput := cursorGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
		t.Fatal("inference response missing provider session identity")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
		t.Fatalf(
			"provider session id = %q, want golden session %q",
			got,
			request.SessionID,
		)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf(
			"inference response text = %#v, want dispatch output %q",
			inferencePayload.Response,
			dispatchOutput,
		)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeCursorProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeCursorResponseEventGoldens(responseEvents),
		InvocationResult: observeCursorInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestCursorGoldenReadableProviderSessionDetails replays the sanitized Cursor success
// golden through the public process boundary and proves Provider Session details are
// readable on the public lookup surface for the success session identity.
// golden: tests/functional/internal/support/testdata/provider-sessions/cursor/success/manifest.json
func TestCursorGoldenReadableProviderSessionDetails(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("cursor", "success")))

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}

	var request struct {
		Model     string `json:"model"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.SessionID == "" {
		t.Fatal("request.session_id must be non-empty")
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", cursorGoldenWorkerConfig(request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"cursor golden success detail"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	homeDir := t.TempDir()
	writeCursorGoldenSuccessStorageFixture(t, homeDir, request.SessionID)

	_, listed, events, baseURL, stopDaemon := runCursorGoldenSuccessReplay(
		t,
		dir,
		homeDir,
		serviceedges.Edges{
			ProviderCommandRunner:               runner,
			ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
		},
		20*time.Second,
	)
	defer stopDaemon()

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload, _ := cursorGoldenInferenceObservation(t, events)
	if inferencePayload.ProviderSession == nil || inferencePayload.ProviderSession.Id == nil {
		t.Fatal("inference response missing provider session identity")
	}
	if got := support.StringPointerValue(inferencePayload.ProviderSession.Id); got != request.SessionID {
		t.Fatalf("provider session id = %q, want golden session %q", got, request.SessionID)
	}

	detail := support.GetJSON[factoryapi.ProviderSessionDetailResponse](
		t,
		providerSessionDetailURL(baseURL, request.SessionID),
	)
	if detail.ProviderSession.Id != request.SessionID {
		t.Fatalf("detail provider session id = %q, want %q", detail.ProviderSession.Id, request.SessionID)
	}
	if len(detail.Transcript) == 0 {
		t.Fatal("provider session detail transcript is empty, want readable success-session content")
	}
	hasReadableText := false
	for _, entry := range detail.Transcript {
		if entry.Text != nil && strings.TrimSpace(*entry.Text) != "" {
			hasReadableText = true
			break
		}
	}
	if !hasReadableText {
		t.Fatalf("provider session detail transcript = %#v, want readable text", detail.Transcript)
	}

	observed := observeCursorProviderSessionDetailGolden(detail)
	if err := compareOrUpdateCursorProviderSessionDetailGolden(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("compareOrUpdateCursorProviderSessionDetailGolden: %v", err)
	}
}

func runCursorGoldenSuccessReplay(
	t *testing.T,
	dir string,
	homeDir string,
	overrides serviceedges.Edges,
	timeout time.Duration,
) (factoryapi.FactorySession, factoryapi.ListWorkResponse, []factoryapi.FactoryEvent, string, func()) {
	t.Helper()

	server := support.NewProcessAPIServer()
	overrides.APIServerStarter = server.Start
	process := support.BuildProcess(t, overrides)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("daemon stderr:\n%s", stderr)
		}
		if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
			t.Logf("daemon stdout:\n%s", stdout)
		}
	})
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	support.WaitForTerminalStatus(t, baseURL, timeout)

	session := support.GetDefaultSession(t, baseURL)
	work := support.ListDefaultSessionWork(t, baseURL)
	events := support.GetFactoryEventsAt(t, baseURL)
	stop := func() { daemon.Stop(t) }
	return session, work, events, baseURL, stop
}

func providerSessionDetailURL(baseURL, sessionID string) string {
	query := url.Values{}
	query.Set("provider", string(factoryapi.Cursor))
	query.Set("kind", string(factoryapi.LoadableProviderSessionKindSessionID))
	query.Set("id", sessionID)
	return strings.TrimSuffix(baseURL, "/") + "/provider-sessions/detail?" + query.Encode()
}

func writeCursorGoldenSuccessStorageFixture(t *testing.T, homeDir, sessionID string) {
	t.Helper()

	chatsRoot := filepath.Join(homeDir, ".cursor", "chats")
	dbPath := filepath.Join(chatsRoot, cursorGoldenSuccessWorkspaceHash, sessionID, "store.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir cursor storage: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open cursor storage sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	schema := `
CREATE TABLE blobs (key TEXT PRIMARY KEY, value TEXT);
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create cursor storage tables: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO blobs (key, value) VALUES (?, ?)`,
		"bubble-success",
		`{"bubbleId":"bubble-success","chatId":"chat-success","text":"Cursor fixture answer COMPLETE","timestamp":1000,"type":1}`,
	); err != nil {
		t.Fatalf("insert cursor storage bubble: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"0",
		`{"createdAt":1000,"agentId":"`+sessionID+`","name":"Cursor golden success fixture session"}`,
	); err != nil {
		t.Fatalf("insert cursor storage meta: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?)`,
		"1",
		`{"usage":{"inputTokens":12,"outputTokens":34,"cacheReadTokens":5,"cacheWriteTokens":2}}`,
	); err != nil {
		t.Fatalf("insert cursor storage usage meta: %v", err)
	}
}

func observeCursorProviderSessionDetailGolden(
	detail factoryapi.ProviderSessionDetailResponse,
) json.RawMessage {
	transcript := make([]map[string]any, 0, len(detail.Transcript))
	for _, entry := range detail.Transcript {
		record := map[string]any{
			"order": entry.Order,
			"type":  string(entry.Type),
		}
		if entry.SourceType != nil {
			record["sourceType"] = *entry.SourceType
		}
		if entry.Text != nil {
			record["text"] = *entry.Text
		}
		if entry.Timestamp != nil {
			record["timestamp"] = entry.Timestamp.UTC().Format(time.RFC3339Nano)
		}
		transcript = append(transcript, record)
	}

	var tokenUsage map[string]any
	if detail.Parse.TokenUsage != nil {
		tokenUsage = map[string]any{}
		if detail.Parse.TokenUsage.InputTokens != nil {
			tokenUsage["inputTokens"] = *detail.Parse.TokenUsage.InputTokens
		}
		if detail.Parse.TokenUsage.OutputTokens != nil {
			tokenUsage["outputTokens"] = *detail.Parse.TokenUsage.OutputTokens
		}
		if detail.Parse.TokenUsage.CachedInputTokens != nil {
			tokenUsage["cachedInputTokens"] = *detail.Parse.TokenUsage.CachedInputTokens
		}
		if detail.Parse.TokenUsage.CacheWriteTokens != nil {
			tokenUsage["cacheWriteTokens"] = *detail.Parse.TokenUsage.CacheWriteTokens
		}
		if detail.Parse.TokenUsage.TotalTokens != nil {
			tokenUsage["totalTokens"] = *detail.Parse.TokenUsage.TotalTokens
		}
	}

	record := map[string]any{
		"providerSession": map[string]any{
			"provider": string(detail.ProviderSession.Provider),
			"kind":     string(detail.ProviderSession.Kind),
			"id":       detail.ProviderSession.Id,
		},
		"source": map[string]any{
			"relativePath": detail.Source.RelativePath,
			"sizeBytes":    detail.Source.SizeBytes,
		},
		"parse": map[string]any{
			"eventCount": detail.Parse.EventCount,
			"lineCount":  detail.Parse.LineCount,
		},
		"transcript": transcript,
	}
	if detail.Source.ModifiedAt != nil {
		record["source"].(map[string]any)["modifiedAt"] = detail.Source.ModifiedAt.UTC().Format(time.RFC3339Nano)
	}
	if tokenUsage != nil {
		record["parse"].(map[string]any)["tokenUsage"] = tokenUsage
	}
	return mustMarshalJSON(record)
}

func compareOrUpdateCursorProviderSessionDetailGolden(
	loaded support.ProviderSessionCase,
	observed json.RawMessage,
) error {
	expectedPath := filepath.Join(loaded.CaseDir, cursorGoldenExpectedProviderSessionDetailFile)
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if !support.ProviderSessionFunctionalGoldensUpdateEnabled() {
			return &support.ProviderSessionLoadError{
				CaseID: loaded.Manifest.ID,
				Role:   "expected-provider-session-detail",
				Path:   expectedPath,
				Detail: "required expected-provider-session-detail fixture is missing",
			}
		}
		encoded, err := json.MarshalIndent(json.RawMessage(observed), "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(expectedPath, append(encoded, '\n'), 0o644); err != nil {
			return err
		}
		return &support.ProviderSessionGoldensUpdatedError{
			CaseID: loaded.Manifest.ID,
			Paths:  []string{cursorGoldenExpectedProviderSessionDetailFile},
		}
	}

	err = support.CompareProviderSessionJSON(
		loaded.Manifest.ID,
		"expected-provider-session-detail",
		loaded.Manifest.NormalizedFields,
		expected,
		observed,
	)
	if err == nil {
		return nil
	}
	if !support.ProviderSessionFunctionalGoldensUpdateEnabled() {
		return err
	}
	encoded, marshalErr := json.MarshalIndent(json.RawMessage(observed), "", "  ")
	if marshalErr != nil {
		return marshalErr
	}
	if writeErr := os.WriteFile(expectedPath, append(encoded, '\n'), 0o644); writeErr != nil {
		return writeErr
	}
	return &support.ProviderSessionGoldensUpdatedError{
		CaseID: loaded.Manifest.ID,
		Paths:  []string{cursorGoldenExpectedProviderSessionDetailFile},
	}
}

func cursorGoldenInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, string) {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
		dispatchOutput   string
	)
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := support.AsInferenceResponseObservation(event)
			if err != nil {
				t.Fatalf("decode inference response: %v", err)
			}
			if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
				continue
			}
			inferencePayload = payload
			foundInference = true
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response: %v", err)
			}
			if payload.Output != nil && *payload.Output != "" {
				dispatchOutput = *payload.Output
			}
		}
	}
	if !foundInference {
		t.Fatal("missing succeeded INFERENCE_RESPONSE in factory events")
	}
	if dispatchOutput == "" {
		t.Fatal("missing dispatch output in factory events")
	}
	return inferencePayload, dispatchOutput
}

func observeCursorProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderCursor)
	sessionID := ""
	if payload.ProviderSession != nil {
		if payload.ProviderSession.Provider != nil {
			provider = support.StringPointerValue(payload.ProviderSession.Provider)
		}
		if payload.ProviderSession.Id != nil {
			sessionID = support.StringPointerValue(payload.ProviderSession.Id)
		}
	}
	record := map[string]string{
		"provider":          provider,
		"providerSessionId": sessionID,
		"fidelityClass":     manifest.FidelityClass,
		"status":            status,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return encoded
}

func observeCursorResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseDelta:
				delta, err := event.Payload.AsFactoryResponseEventMessageDeltaPayload()
				if err != nil || delta.TextDelta == nil {
					continue
				}
				record := map[string]any{
					"type":             "message.delta",
					"eventId":          event.EventId,
					"factorySessionId": event.FactorySessionId,
					"runId":            event.RunId,
					"itemId":           cursorGoldenItemID(event),
					"text":             *delta.TextDelta,
				}
				records = append(records, mustMarshalJSON(record))
			case factoryapi.FactoryResponseEventPhaseCompleted:
				message, err := event.Payload.AsFactoryResponseEventMessagePayload()
				if err != nil {
					continue
				}
				text := cursorGoldenMessageText(message)
				if text == "" {
					continue
				}
				record := map[string]any{
					"type":             "message.completed",
					"eventId":          event.EventId,
					"factorySessionId": event.FactorySessionId,
					"runId":            event.RunId,
					"itemId":           cursorGoldenItemID(event),
					"text":             text,
					"finishReason":     "stop",
				}
				records = append(records, mustMarshalJSON(record))
			}
		case factoryapi.FactoryResponseEventKindTool:
			tool, err := event.Payload.AsFactoryResponseEventToolPayload()
			if err != nil {
				continue
			}
			recordType := ""
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseStarted:
				recordType = "tool.started"
			case factoryapi.FactoryResponseEventPhaseCompleted:
				recordType = "tool.completed"
			default:
				continue
			}
			record := map[string]any{
				"type":             recordType,
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           cursorGoldenItemID(event),
				"toolName":         tool.ToolName,
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeCursorInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	record := map[string]any{
		"ok":           ok,
		"content":      content,
		"finishReason": "stop",
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		panic(err)
	}
	return encoded
}

func cursorGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func cursorGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range message.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func cursorGoldenWorkerConfig(model string) string {
	return strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderCursor, model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
