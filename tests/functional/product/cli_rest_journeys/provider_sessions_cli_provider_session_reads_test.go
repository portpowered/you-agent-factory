package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type providerSessionReadCase struct {
	name, provider, id, text string
}

var providerSessionReadCases = []providerSessionReadCase{
	{"F11-U1 Codex missing", "codex", "session_fixture_codex_missing_read", "Codex fixture answer COMPLETE"},
	{"F11-U3 Codex truncated", "codex", "session_fixture_codex_truncated_read", "Codex fixture answer COMPLETE"},
	{"F11-U5 Codex open fault", "codex", "session_fixture_codex_open_fault_read", "Codex fixture answer COMPLETE"},
}

// Reads require a terminal Worker Session, not merely a Provider Session ID.
// Each leaf admits Work into its own explicit Factory Session and routes a
// real provider protocol response through the command-runner edge. The shared
// host's reader effects and seeded storage remain immutable across leaves.
func testProvidersessionscliTerminalProviderSessionReadsPreserveTranscriptOutcomes(t *testing.T) {
	fixture := workerSessionsCLIProcess(t)
	for _, test := range providerSessionReadCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			caseFixture := newWorkerSessionsCLICase(t)
			support.WriteAgentConfig(t, caseFixture.factoryDir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
			// Concurrent commands own distinct customer profiles; provider storage
			// still resolves to the immutable home selected during BuildProcess.
			env := functionalEnvironment(t.TempDir())

			caseFixture.registerRoutes(t, test.id)
			sessionID := caseFixture.openSession(t)
			workID := submitWork(t, ctx, fixture.process, env, caseFixture.factoryDir, fixture.baseURL, sessionID, test.id)
			terminal := waitForWorkerSessionState(t, ctx, fixture.process, env, caseFixture.factoryDir, fixture.baseURL, sessionID, workID, "COMPLETED")
			if terminal.ProviderSession == nil || terminal.ProviderSession.ID != test.id || terminal.ProviderSession.Provider != test.provider || terminal.ProviderSession.Kind != "session_id" {
				t.Fatalf("terminal association = %#v, want %s/%s", terminal.ProviderSession, test.provider, test.id)
			}
			assertProviderSessionTerminalIdentity(t, terminal, terminal, sessionID, workID)
			assertProviderTranscriptPublished(t, ctx, fixture, caseFixture, env, sessionID, terminal)
			assertTerminalProviderSessionRead(t, ctx, fixture, caseFixture, env, sessionID, workID, terminal, test)
		})
	}
}

// Live completion precedes durable capture. The public terminal stream joins
// publication before the read; no polling or extra timeout is needed.
func assertProviderTranscriptPublished(t *testing.T, ctx context.Context, fixture *workerSessionsCLISharedFixture,
	caseFixture *workerSessionsCLICase, env []string, sessionID string, terminal workerSessionJSON,
) {
	t.Helper()
	inputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir,
		"--server", fixture.baseURL, "worker-sessions", "stream", "--session", sessionID,
		"--worker-session-id", terminal.WorkerSessionID, "--output", "json")
	for _, line := range nonEmptyLines(inputs.Stdout()) {
		var frame streamFrameJSON
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("decode terminal stream: %v", err)
		}
		if (frame.Delivery == "TERMINAL" || frame.Delivery == "TERMINAL_REPLAY") && frame.WorkerSessionID == terminal.WorkerSessionID &&
			frame.FactorySessionID != nil && *frame.FactorySessionID == sessionID {
			return
		}
	}
	t.Fatalf("stream omitted scoped terminal publication: %s", inputs.Stdout())
}

func assertTerminalProviderSessionRead(t *testing.T, ctx context.Context, fixture *workerSessionsCLISharedFixture,
	caseFixture *workerSessionsCLICase, env []string, sessionID, workID string, terminal workerSessionJSON, test providerSessionReadCase,
) {
	t.Helper()
	args := []string{"--server", fixture.baseURL, "worker-sessions", "read", "--session", sessionID,
		"--provider", test.provider, "--kind", "session_id", "--id", test.id, "--output", "json"}
	showArgs := append([]string(nil), args...)
	showArgs[3] = "show"
	shownInputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir, showArgs...)
	var shown workerSessionJSON
	decodeCLIJSON(t, shownInputs, &shown)
	assertProviderSessionTerminalIdentity(t, shown, terminal, sessionID, workID)
	assertProviderSessionReadSafe(t, shownInputs.Stdout(), fixture.homeDir)
	if shown.Transcript != "UNAVAILABLE" || shown.Parse.EventCount != 0 || shown.Parse.MalformedLineCount != 0 || len(shown.Parse.Errors) != 0 {
		t.Fatalf("canonical show copied optional native transcript diagnostics: %#v", shown)
	}
	// Compatibility lists now use the same captured projection as reads.
	// Native truncated files cannot contribute parse diagnostics.
	if terminal.Parse.MalformedLineCount != 0 || len(terminal.Parse.Errors) != 0 {
		t.Fatalf("captured parse = %#v, want no native diagnostics", terminal.Parse)
	}
	inputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir, args...)
	var transcript struct {
		transcriptJSON
		FactorySessionID *string `json:"factorySessionId"`
	}
	decodeCLIJSON(t, inputs, &transcript)
	if transcript.FactorySessionID == nil || *transcript.FactorySessionID != sessionID {
		t.Fatalf("read Factory Session = %#v, want %s", transcript.FactorySessionID, sessionID)
	}
	if transcript.WorkerSessionID != terminal.WorkerSessionID || transcript.ProviderSession != *terminal.ProviderSession || transcript.State != "COMPLETED" || len(transcript.WorkIDs) != 1 || transcript.WorkIDs[0] != workID {
		t.Fatalf("read changed terminal association: %#v", transcript)
	}
	if len(transcript.Entries) != 2 || transcript.Entries[0].Type != "tool_call" || transcript.Entries[1].Text != test.text || transcript.Entries[1].Type != "assistant_message" {
		t.Fatalf("captured transcript = %#v, want ordered tool call and captured answer", transcript.Entries)
	}
	assertProviderSessionReadSafe(t, inputs.Stdout()+shownInputs.Stdout(), fixture.homeDir)
	// Captured reads leave the terminal association intact despite native faults.
	finalInputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir, showArgs...)
	var final workerSessionJSON
	decodeCLIJSON(t, finalInputs, &final)
	assertProviderSessionTerminalIdentity(t, final, terminal, sessionID, workID)
	assertProviderSessionReadSafe(t, finalInputs.Stdout(), fixture.homeDir)
}

func assertProviderSessionTerminalIdentity(t *testing.T, observed, terminal workerSessionJSON, sessionID, workID string) {
	t.Helper()
	if observed.WorkerSessionID == "" || observed.WorkerSessionID != terminal.WorkerSessionID ||
		observed.ProviderSession == nil || *observed.ProviderSession != *terminal.ProviderSession ||
		observed.FactorySessionID == nil || *observed.FactorySessionID != sessionID || observed.State != "COMPLETED" ||
		observed.WorkID == nil || *observed.WorkID != workID || len(observed.WorkIDs) != 1 || observed.WorkIDs[0] != workID {
		t.Fatalf("terminal association = %#v, want Worker %s, Provider %#v, Factory Session %s and singleton Work %s",
			observed, terminal.WorkerSessionID, terminal.ProviderSession, sessionID, workID)
	}
}

func assertProviderSessionReadSafe(t *testing.T, output, home string) {
	t.Helper()
	for _, forbidden := range []string{home, "/private/operator/session-store", "private-storage-fault"} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("Provider Session output leaked storage fault: %s", output)
		}
	}
}
func resetprovidersessionscli3State() {
	var freshProviderSessionReadCases = []providerSessionReadCase{
		{"F11-U1 Codex missing", "codex", "session_fixture_codex_missing_read", "Codex fixture answer COMPLETE"},
		{"F11-U3 Codex truncated", "codex", "session_fixture_codex_truncated_read", "Codex fixture answer COMPLETE"},
		{"F11-U5 Codex open fault", "codex", "session_fixture_codex_open_fault_read", "Codex fixture answer COMPLETE"},
	}
	providerSessionReadCases = freshProviderSessionReadCases
}
