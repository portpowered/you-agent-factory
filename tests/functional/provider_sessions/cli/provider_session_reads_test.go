package cli_test

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
	name, provider, id, text, errorCode string
}

var providerSessionReadCases = []providerSessionReadCase{
	{"F11-U1 Codex missing", "codex", "session_fixture_codex_missing_read", "", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"},
	{"F11-U3 Codex truncated", "codex", "session_fixture_codex_truncated_read", "retained fixture answer", ""},
	{"F11-U5 Codex open fault", "codex", "session_fixture_codex_open_fault_read", "", "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE"},
}

// Reads require a terminal Worker Session, not merely a Provider Session ID.
// Each leaf admits Work into its own explicit Factory Session and routes a
// real provider protocol response through the command-runner edge. The shared
// host's reader effects and seeded storage remain immutable across leaves.
func TestTerminalProviderSessionReadsPreserveTranscriptOutcomes(t *testing.T) {
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
			support.InitializeCustomerHomeWithProcess(t, fixture.process, env, caseFixture.factoryDir)
			caseFixture.registerRoutes(t, test.id)
			sessionID := caseFixture.openSession(t)
			workID := submitWork(t, ctx, fixture.process, env, caseFixture.factoryDir, fixture.baseURL, sessionID, test.id)
			terminal := waitForWorkerSessionState(t, ctx, fixture.process, env, caseFixture.factoryDir, fixture.baseURL, sessionID, workID, "COMPLETED")
			if terminal.ProviderSession == nil || terminal.ProviderSession.ID != test.id || terminal.ProviderSession.Provider != test.provider || terminal.ProviderSession.Kind != "session_id" {
				t.Fatalf("terminal association = %#v, want %s/%s", terminal.ProviderSession, test.provider, test.id)
			}
			assertTerminalProviderSessionRead(t, ctx, fixture, caseFixture, env, sessionID, workID, terminal, test)
		})
	}
}

func assertTerminalProviderSessionRead(t *testing.T, ctx context.Context, fixture *workerSessionsCLISharedFixture,
	caseFixture *workerSessionsCLICase, env []string, sessionID, workID string, terminal workerSessionJSON, test providerSessionReadCase,
) {
	t.Helper()
	args := []string{"--server", fixture.baseURL, "worker-sessions", "read", "--session", sessionID,
		"--provider", test.provider, "--kind", "session_id", "--id", test.id, "--output", "json"}
	if test.errorCode != "" {
		inputs, err := executeCLIExpectError(t, ctx, fixture.process, env, caseFixture.factoryDir, args...)
		if err == nil {
			t.Fatal("unavailable transcript returned success")
		}
		assertFleetJSONErrorCode(t, []byte(inputs.Stderr()), test.errorCode, test.name)
		if strings.TrimSpace(inputs.Stdout()) != "" {
			t.Fatalf("unavailable transcript fabricated stdout: %s", inputs.Stdout())
		}
		assertProviderSessionReadSafe(t, err.Error()+inputs.Stderr(), fixture.homeDir)
		return
	}
	showArgs := append([]string(nil), args...)
	showArgs[3] = "show"
	shownInputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir, showArgs...)
	var shown workerSessionJSON
	decodeCLIJSON(t, shownInputs, &shown)
	if shown.WorkerSessionID != terminal.WorkerSessionID || shown.ProviderSession == nil || *shown.ProviderSession != *terminal.ProviderSession {
		t.Fatalf("show changed terminal identity: %#v", shown)
	}
	if test.provider == "codex" && (shown.Parse.MalformedLineCount != 1 || len(shown.Parse.Errors) != 1 || shown.Parse.Errors[0].Message != "truncated JSON event record") {
		t.Fatalf("mixed rollout parse = %#v, want one truncated diagnostic", shown.Parse)
	}
	for range 2 {
		inputs := executeCLI(t, ctx, fixture.process, env, caseFixture.factoryDir, args...)
		var transcript transcriptJSON
		decodeCLIJSON(t, inputs, &transcript)
		if transcript.WorkerSessionID != terminal.WorkerSessionID || transcript.ProviderSession != *terminal.ProviderSession || !containsString(transcript.WorkIDs, workID) {
			t.Fatalf("read changed terminal association: %#v", transcript)
		}
		encoded, err := json.Marshal(transcript.Entries)
		if err != nil {
			t.Fatal(err)
		}
		if test.text != "" && !strings.Contains(string(encoded), test.text) {
			t.Fatalf("read omitted retained text: %s", encoded)
		}
		if len(transcript.Entries) != 1 {
			t.Fatalf("mixed rollout transcript = %#v, want only the retained valid entry", transcript.Entries)
		}
		assertProviderSessionReadSafe(t, inputs.Stdout()+shownInputs.Stdout(), fixture.homeDir)
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
