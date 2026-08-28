package models_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestStory001CharacterizesFiveFailingInvokes retains every built-process
// exit and stream for the same controlled failure. It does not assert the
// eventual deterministic diagnostic contract; that is owned by story 004.
func TestStory001CharacterizesFiveFailingInvokes(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failManifest: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	args := []string{"--debug", "models", "invoke", "embed", "--input", "text=" + story001ModelInput}
	results := make([]builtProcessResult, 0, 5)
	for index := 0; index < 5; index++ {
		result := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
		if result.timedOut || !result.processExited || result.exitCode == 0 {
			t.Fatalf("failing invoke %d did not produce a terminal failure: %s", index+1, summarizeProcess(result))
		}
		results = append(results, result)
	}

	for index, result := range results {
		t.Logf("STORY-001-EVIDENCE acceptance=five-failures probe=invoke-%d %s", index+1, summarizeProcess(result))
	}
	t.Logf("STORY-001-EVIDENCE acceptance=five-failures originRequests=%d assetLedger=%s", len(origin.exchangesSnapshot()), compactJSON(origin.assetExchanges()))
}

// TestStory004RendersFiveFailingInvokesDeterministically proves the delivered
// executable owns one safe typed diagnostic and one stable debug cause chain
// for every repeated failure. The controlled source fails before model content
// so the witness remains independent of remote availability.
func TestStory004RendersFiveFailingInvokesDeterministically(t *testing.T) {
	origin := newCharacterizationOrigin(t, characterizationOriginOptions{failManifest: true})
	binaryPath := buildStory001Binary(t)
	workDir := t.TempDir()
	writeStory001Factory(t, workDir)
	homeDir := t.TempDir()
	cacheDir := t.TempDir()
	environment := story001Environment(homeDir, cacheDir, origin.URL())
	args := []string{"--debug", "models", "invoke", "embed", "--input", "text=" + story001ModelInput}

	var baseline story004DiagnosticCapture
	for index := 0; index < 5; index++ {
		result := runStory001Command(t, context.Background(), binaryPath, workDir, environment, args...)
		if result.exitCode != 1 || !result.processExited || result.timedOut {
			t.Fatalf("failing invoke %d did not exit 1: %s", index+1, summarizeProcess(result))
		}
		if len(result.stdout) != 0 {
			t.Fatalf("failing invoke %d stdout = %q, want empty", index+1, result.stdout)
		}
		capture := decodeStory004Diagnostic(t, result.stderr)
		if strings.TrimSpace(string(capture.response.Code)) == "" || strings.TrimSpace(capture.response.Message) == "" || capture.debug == "" {
			t.Fatalf("failing invoke %d capture = %#v, want non-empty code/message/debug", index+1, capture)
		}
		if index == 0 {
			baseline = capture
		} else if capture.response.Code != baseline.response.Code ||
			capture.response.Family != baseline.response.Family ||
			capture.response.Message != baseline.response.Message || capture.debug != baseline.debug {
			t.Fatalf("failing invoke %d capture = %#v, want baseline %#v", index+1, capture, baseline)
		}
		t.Logf("STORY-004-EVIDENCE acceptance=five-failures probe=invoke-%d code=%s family=%s message=%q debugSHA256=%s %s", index+1, capture.response.Code, capture.response.Family, capture.response.Message, sha256Hex([]byte(capture.debug)), summarizeProcess(result))
	}

	jsonResult := runStory001Command(t, context.Background(), binaryPath, workDir, environment,
		"--json", "--debug", "models", "invoke", "embed", "--input", "text="+story001ModelInput,
	)
	if jsonResult.exitCode != 1 || !jsonResult.processExited || jsonResult.timedOut {
		t.Fatalf("JSON failing invoke did not exit 1: %s", summarizeProcess(jsonResult))
	}
	if len(jsonResult.stdout) > 0 {
		var value any
		if err := json.Unmarshal(jsonResult.stdout, &value); err != nil {
			t.Fatalf("JSON failing invoke stdout is neither empty nor valid JSON: %v; stdout=%q", err, jsonResult.stdout)
		}
	}
	jsonCapture := decodeStory004Diagnostic(t, jsonResult.stderr)
	if jsonCapture.response.Code != baseline.response.Code || jsonCapture.response.Message != baseline.response.Message || jsonCapture.debug != baseline.debug {
		t.Fatalf("JSON failing invoke capture = %#v, want baseline %#v", jsonCapture, baseline)
	}
	t.Logf("STORY-004-EVIDENCE acceptance=json-failure stdoutBytes=%d stderrBytes=%d stderrSHA256=%s code=%s family=%s", len(jsonResult.stdout), len(jsonResult.stderr), sha256Hex(jsonResult.stderr), jsonCapture.response.Code, jsonCapture.response.Family)
}

type story004DiagnosticCapture struct {
	response factoryapi.ErrorResponse
	debug    string
}

func decodeStory004Diagnostic(t testing.TB, stderr []byte) story004DiagnosticCapture {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(stderr)), "\n")
	if len(lines) < 2 {
		t.Fatalf("stderr = %q, want typed line and debug cause", stderr)
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
		t.Fatalf("decode typed diagnostic: %v; stderr=%q", err, stderr)
	}
	debugLines := make([]string, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if !strings.HasPrefix(line, "debug: ") {
			t.Fatalf("stderr = %q, want only debug continuation after typed line", stderr)
		}
		debugLines = append(debugLines, line)
	}
	return story004DiagnosticCapture{response: response, debug: strings.Join(debugLines, "\n")}
}
