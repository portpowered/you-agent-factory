package workersessions_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Identical captured identities and generations in separate profiles must not
// admit each other's cursor. Replacing the capture in its original profile
// must also reject the old generation. Real recovery/query/storage is observed
// through CLI/HTTP/MCP; only the durable input fixture is controlled. These
// Direct sessions have no Factory Session or provider execution. Sequential
// source/replacement hosts keep the sole storage writer joined.
func runCapturedLogsCursorIsolation(t *testing.T, process support.Process) {
	t.Helper()
	dir := support.ScaffoldSingleStepFactory(t, "logs-cursor-isolation")
	writeLogsCursorCapture(t, dir, "original-generation")
	config := support.FunctionalAPIServerConfig{
		FactoryDir: dir, WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{ProviderCommandRunner: rejectLocalProvider{t: t}, FactorySessionsWorkingDirectory: historyWorkingDirectory(dir)},
	}
	host := support.StartFunctionalAPIServer(t, config)
	session, ctx := startMCP(t, process, host.URL())
	page := logsParityPage(t, ctx, session, host, "updated", "")
	token, ok := page["nextToken"].(string)
	if !ok || token == "" || page["recordingGenerationId"] != "original-generation" {
		t.Fatalf("bounded capture omitted original generation/cursor: %v", page)
	}
	foreignDir := support.ScaffoldSingleStepFactory(t, "logs-cursor-foreign")
	writeLogsCursorCapture(t, foreignDir, "original-generation")
	foreignConfig := config
	foreignConfig.FactoryDir = foreignDir
	foreignConfig.Edges.FactorySessionsWorkingDirectory = historyWorkingDirectory(foreignDir)
	foreign := support.StartFunctionalAPIServer(t, foreignConfig)
	foreignSession, foreignCtx := startMCP(t, process, foreign.URL())
	foreignPage := logsParityPage(t, foreignCtx, foreignSession, foreign, "updated", "")
	if foreignPage["recordingGenerationId"] != page["recordingGenerationId"] || foreignPage["committedPosition"] != page["committedPosition"] {
		t.Fatal("foreign fixture must preserve generation and committed head to isolate profile fencing")
	}
	assertLogsCursorRejected(t, foreignCtx, foreignSession, foreign, token)
	// A rejected foreign cursor must not invalidate its original continuation.
	logsParityPage(t, ctx, session, host, "updated", token)
	host.Close(t)
	writeLogsCursorCapture(t, dir, "replacement-generation")
	replacement := support.StartFunctionalAPIServer(t, config)
	replacementSession, replacementCtx := startMCP(t, process, replacement.URL())
	assertLogsCursorRejected(t, replacementCtx, replacementSession, replacement, token)
	fresh := logsParityPage(t, replacementCtx, replacementSession, replacement, "updated", "")
	if fresh["recordingGenerationId"] != "replacement-generation" {
		t.Fatalf("replacement lost its own capture generation: %v", fresh)
	}
	logsParityPage(t, replacementCtx, replacementSession, replacement, "updated", fresh["nextToken"].(string))
}

func assertLogsCursorRejected(t *testing.T, ctx context.Context, session *mcp.ClientSession, host *support.FunctionalAPIServer, token string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, host.URL()+"/worker-sessions/updated/logs?nextToken="+url.QueryEscape(token), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusBadRequest || body["code"] != "BAD_REQUEST" || body["events"] != nil {
		t.Fatalf("HTTP cursor rejection leaked a page or changed meaning: status=%d body=%v", response.StatusCode, body)
	}
	assertToolError(t, callTool(t, ctx, session, "you.worker_session.read", map[string]any{"workerSessionId": "updated", "view": "logs", "nextToken": token}), "worker_session.invalid_request", false)
	for _, follow := range []bool{false, true} {
		args := []string{"you", "--server", host.URL(), "worker-sessions", "read", "--worker-session-id", "updated", "--view", "logs", "--next-token", token, "--output", "json"}
		if follow {
			args = append(args, "--follow")
		}
		inputs := support.FakeInputs(ctx, args)
		err := host.Execute(t, inputs.Input)
		var failure interface{ CLIErrorCode() string }
		if !errors.As(err, &failure) || failure.CLIErrorCode() != "BAD_REQUEST" || inputs.Stdout() != "" {
			t.Fatalf("CLI cursor rejection follow=%t: error=%v stdout=%s stderr=%s", follow, err, inputs.Stdout(), inputs.Stderr())
		}
	}
}

func writeLogsCursorCapture(t *testing.T, dir, generation string) {
	t.Helper()
	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: "updated-recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: "updated", RecordingGenerationID: generation, Records: metadataCaptureRecords(t, "updated"),
		}},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, ".you-agent-factory", "worker-recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(snapshot.RecordingID))
	if err := os.WriteFile(filepath.Join(root, hex.EncodeToString(digest[:])+".worker.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
