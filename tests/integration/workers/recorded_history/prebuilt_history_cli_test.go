package recordedhistory_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	historyIntegrationBinaryEnv  = "INFINITE_YOU_INTEGRATION_BINARY"
	historyPrebuiltBinaryEnv     = "INFINITE_YOU_PREBUILT_ARTIFACT"
	historyRequirePrebuiltBinary = "INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT"
	historyRecordingSessionID    = "a6925b33-db77-4e62-ab6f-2a8d06783e5e"
	historyKnownWorkID           = "work-recorded"
)

func TestPrebuiltHistoryCLIExitStatusMatchesResult(t *testing.T) {
	t.Parallel()

	binaryPath := resolvePrebuiltHistoryBinary(t)
	root := t.TempDir()
	artifactPath := filepath.Join(root, "history.json")
	if err := os.WriteFile(artifactPath, historyArtifactWithKnownWork(t), 0o600); err != nil {
		t.Fatalf("write offline history fixture: %v", err)
	}

	tests := []struct {
		name       string
		recording  string
		workID     string
		wantStatus string
		wantExit   int
	}{
		{name: "available", recording: artifactPath, workID: historyKnownWorkID, wantStatus: "AVAILABLE", wantExit: 0},
		{name: "missing Work", recording: artifactPath, workID: "work-not-recorded", wantStatus: "WORK_NOT_FOUND", wantExit: 1},
		{name: "unavailable recording", recording: filepath.Join(root, "missing.json"), workID: historyKnownWorkID, wantStatus: "UNAVAILABLE", wantExit: 1},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			workspace := filepath.Join(root, strings.ReplaceAll(testCase.name, " ", "-"), "workspace")
			home := filepath.Join(root, strings.ReplaceAll(testCase.name, " ", "-"), "home")
			if err := os.MkdirAll(workspace, 0o700); err != nil {
				t.Fatalf("create isolated history CLI workspace: %v", err)
			}
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatalf("create isolated history CLI profile: %v", err)
			}
			assertPrebuiltHistoryCommand(t, binaryPath, workspace, home, testCase.recording, testCase.workID, testCase.wantStatus, testCase.wantExit)
		})
	}
}

func assertPrebuiltHistoryCommand(
	t *testing.T,
	binaryPath, workspace, home, recording, workID, wantStatus string,
	wantExit int,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath,
		"worker-sessions", "history", "--recording", recording,
		"--session", historyRecordingSessionID, "--work-id", workID, "--output", "json",
	)
	command.Dir = workspace
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(home)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if wantExit == 0 {
		if err != nil {
			t.Fatalf("prebuilt history CLI exit = %v, want 0; stdout=%s stderr=%s", err, stdout.String(), stderr.String())
		}
	} else {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) || exitError.ExitCode() != wantExit {
			t.Fatalf("prebuilt history CLI exit = %v, want %d; stdout=%s stderr=%s", err, wantExit, stdout.String(), stderr.String())
		}
	}
	var result struct {
		Status    string `json:"status"`
		ErrorCode string `json:"errorCode"`
	}
	if decodeErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); decodeErr != nil {
		t.Fatalf("decode prebuilt history CLI result: %v; stdout=%q stderr=%q", decodeErr, stdout.String(), stderr.String())
	}
	if result.Status != wantStatus {
		t.Fatalf("prebuilt history CLI status = %q, want %q; stdout=%s", result.Status, wantStatus, stdout.String())
	}
	if wantExit == 0 && result.ErrorCode != "" {
		t.Fatalf("AVAILABLE result has errorCode %q, want none", result.ErrorCode)
	}
	if wantExit == 1 && result.ErrorCode == "" {
		t.Fatalf("failure result has no errorCode: %s", stdout.String())
	}
}

func historyArtifactWithKnownWork(t *testing.T) []byte {
	t.Helper()
	dispatchID := "dispatch-known"
	sessionID := "~default"
	workIDs := []string{historyKnownWorkID}
	eventPayload, err := json.Marshal(factorydefinitions.DispatchRequestEventPayload{
		TransitionID: "build",
		Inputs:       []factorydefinitions.DispatchConsumedWorkRef{{WorkID: historyKnownWorkID}},
	})
	if err != nil {
		t.Fatalf("marshal history dispatch request: %v", err)
	}
	event := factorydefinitions.FactoryEvent{
		Context: factorydefinitions.FactoryEventContext{
			DispatchID: &dispatchID,
			EventTime:  time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
			Sequence:   0,
			SessionID:  &sessionID,
			Tick:       1,
			WorkIDs:    &workIDs,
		},
		Id:            "history-event-dispatch-known",
		Payload:       eventPayload,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          factorydefinitions.FactoryEventTypeDispatchRequest,
	}
	artifact, err := json.Marshal(struct {
		SchemaVersion string                            `json:"schemaVersion"`
		RecordedAt    time.Time                         `json:"recordedAt"`
		Events        []factorydefinitions.FactoryEvent `json:"events"`
	}{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		Events:        []factorydefinitions.FactoryEvent{event},
	})
	if err != nil {
		t.Fatalf("marshal offline history fixture: %v", err)
	}
	return artifact
}

func resolvePrebuiltHistoryBinary(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(os.Getenv(historyIntegrationBinaryEnv))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(historyPrebuiltBinaryEnv))
	}
	if path == "" {
		if strings.EqualFold(strings.TrimSpace(os.Getenv(historyRequirePrebuiltBinary)), "1") {
			t.Fatalf("%s or %s is required; this integration test never builds the CLI", historyIntegrationBinaryEnv, historyPrebuiltBinaryEnv)
		}
		t.Skip("prebuilt history CLI artifact unavailable; set INFINITE_YOU_INTEGRATION_BINARY")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve prebuilt history CLI artifact %q: %v", path, err)
	}
	info, err := os.Stat(absolute)
	if err != nil || info.IsDir() {
		t.Fatalf("prebuilt history CLI artifact %q is unavailable: %v", absolute, err)
	}
	return absolute
}
