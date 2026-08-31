package realclient_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCharacterizePinnedAcpxFailureClassifications(t *testing.T) {
	t.Run("output classifications", func(t *testing.T) {
		characterizeOutputFailures(t)
	})
	t.Run("process classifications", func(t *testing.T) {
		characterizeProcessFailures(t)
	})
	t.Run("cleanup classifications", func(t *testing.T) {
		characterizeCleanupFailures(t)
	})
}

func characterizeOutputFailures(t *testing.T) {
	t.Run("malformed session JSON", func(t *testing.T) {
		_, err := parseAcpxSessionResult([]byte(`{"action":`), "session_ensured")
		assertCharacterizationError(t, err, "acpx session_ensured output was not machine-readable JSON")
	})

	t.Run("missing session action", func(t *testing.T) {
		_, err := parseAcpxSessionResult([]byte(`{"action":"other"}`), "session_ensured")
		assertCharacterizationError(t, err, "acpx did not report session_ensured")
	})

	t.Run("malformed prompt JSON", func(t *testing.T) {
		assertCharacterizationError(t, validatePromptEvidence([]byte(`{"method":`)), "acpx prompt output was not machine-readable JSON")
	})

	t.Run("protocol error", func(t *testing.T) {
		output := []byte(`{"error":{"code":-1,"message":"controlled"}}`)
		assertCharacterizationError(t, validatePromptEvidence(output), "acpx prompt reported a protocol error")
	})

	t.Run("missing terminal result", func(t *testing.T) {
		assertCharacterizationError(t, validatePromptEvidence([]byte(promptEvidenceFrames())), "exactly one successful end_turn")
	})

	t.Run("duplicate terminal result", func(t *testing.T) {
		output := promptEvidenceFrames() + "\n" + `{"result":{"stopReason":"end_turn"}}` + "\n" + `{"result":{"stopReason":"end_turn"}}`
		assertCharacterizationError(t, validatePromptEvidence([]byte(output)), "exactly one successful end_turn")
	})

	t.Run("valid session and prompt evidence remains accepted", func(t *testing.T) {
		result, err := parseAcpxSessionResult([]byte(`{"action":"session_ensured","created":true,"acpxRecordId":"record","acpxSessionId":"session"}`), "session_ensured")
		if err != nil {
			t.Fatalf("valid session evidence was rejected: %v", err)
		}
		if !result.Created || result.RecordID == "" || result.ACPSessionID == "" {
			t.Fatalf("valid session evidence lost its identities: %+v", result)
		}
		if err := validatePromptEvidence([]byte(promptEvidenceFrames() + "\n" + `{"result":{"stopReason":"end_turn"}}`)); err != nil {
			t.Fatalf("valid prompt evidence was rejected: %v", err)
		}
	})
}

func characterizeProcessFailures(t *testing.T) {
	t.Run("launch failure", func(t *testing.T) {
		directory := t.TempDir()
		_, err := runBoundedCommandWithTimeout(directory, nil, "characterize-launch", time.Second, filepath.Join(directory, "missing-executable"))
		assertCharacterizationError(t, err, "during characterize-launch: launch")
	})

	t.Run("process ownership failure cleans up without another process", func(t *testing.T) {
		terminated, waited := false, false
		err := cleanupStartedCommandAfterOwnershipFailure(
			"characterize-ownership",
			func() error {
				terminated = true
				return errors.New("controlled termination result")
			},
			func() error {
				waited = true
				return errors.New("controlled wait result")
			},
		)
		assertCharacterizationError(t, err, "during characterize-ownership: process ownership")
		if !terminated || !waited {
			t.Fatalf("ownership failure cleanup = terminated:%t waited:%t, want both cleanup operations", terminated, waited)
		}
	})
}

func characterizeCleanupFailures(t *testing.T) {
	t.Run("partial session close is attempted and clears state", func(t *testing.T) {
		calls := 0
		scenario := &pinnedAcpxScenario{
			sessionMayExist: true,
			cleanupCloser: func() error {
				calls++
				return nil
			},
		}
		if err := scenario.closeSessionForCleanup(); err != nil {
			t.Fatalf("partial-session cleanup was rejected: %v", err)
		}
		if calls != 1 || scenario.sessionMayExist {
			t.Fatalf("partial-session cleanup state = calls:%d sessionMayExist:%t, want one close and cleared state", calls, scenario.sessionMayExist)
		}
	})

	t.Run("partial session close failure stays fail-closed", func(t *testing.T) {
		scenario := &pinnedAcpxScenario{
			sessionMayExist: true,
			cleanupCloser:   func() error { return errors.New("controlled close failure") },
		}
		err := scenario.closeSessionForCleanup()
		assertCharacterizationError(t, err, "real ACP evidence cleanup failed: close disposable session")
		if !scenario.sessionMayExist {
			t.Fatal("failed partial-session cleanup incorrectly cleared session ownership")
		}
	})

	t.Run("queue owner remains a cleanup failure", func(t *testing.T) {
		scenario := &pinnedAcpxScenario{home: t.TempDir()}
		queueDirectory := filepath.Join(scenario.home, ".acpx", "queues")
		if err := os.MkdirAll(queueDirectory, 0o755); err != nil {
			t.Fatalf("create controlled queue directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(queueDirectory, "owner"), []byte("active"), 0o600); err != nil {
			t.Fatalf("create controlled queue owner: %v", err)
		}
		assertCharacterizationError(t, scenario.queueOwnerStopped(), "disposable acpx queue owner remained active")
	})
}

func promptEvidenceFrames() string {
	return strings.Join([]string{
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call"}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"tool_call_update","content":[]}}}`,
		`{"method":"session/update","params":{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ok"}}}}`,
	}, "\n")
}

func assertCharacterizationError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected characterization error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("characterization error = %q, want substring %q", err.Error(), want)
	}
}
