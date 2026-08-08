package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRenderWatchTransitionWritesExactEscapedNDJSONContract(t *testing.T) {
	eventTime := time.Date(2026, time.August, 8, 20, 21, 22, 123456789, time.UTC)
	var output bytes.Buffer
	err := RenderWatchTransition(&output, WatchTransition{
		SessionID:     "session/\"beta\"",
		EventID:       "factory-event/\"move\"",
		Sequence:      12,
		EventTime:     eventTime,
		WorkID:        "work\\alpha",
		WorkTypeName:  "review\nitem",
		FromState:     "in-review",
		ToState:       "to-complete",
		Source:        "operator/api",
		Terminal:      false,
		TriggerWorkID: "trigger-1",
		Reason:        "line one\nline two",
	})
	if err != nil {
		t.Fatalf("RenderWatchTransition() error = %v", err)
	}

	if !strings.HasSuffix(output.String(), "\n") || strings.Count(output.String(), "\n") != 1 {
		t.Fatalf("output = %q, want exactly one newline-terminated line", output.String())
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil {
		t.Fatalf("decode rendered line: %v\n%s", err, output.String())
	}
	wantFields := []string{
		"schemaVersion", "sessionId", "eventId", "sequence", "eventTime",
		"workId", "workTypeName", "fromState", "toState", "source", "terminal",
		"triggerWorkId", "reason",
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("field count = %d, want %d (%v)", len(fields), len(wantFields), fields)
	}
	for _, field := range wantFields {
		if _, ok := fields[field]; !ok {
			t.Fatalf("rendered line missing field %q", field)
		}
	}
	var got watchLine
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &got); err != nil {
		t.Fatalf("decode typed rendered line: %v", err)
	}
	if got.SchemaVersion != WatchSchemaVersion || got.SessionID != "session/\"beta\"" ||
		got.EventID != "factory-event/\"move\"" || got.Sequence != 12 ||
		!got.EventTime.Equal(eventTime) || got.WorkID != "work\\alpha" ||
		got.WorkTypeName != "review\nitem" || got.TriggerWorkID != "trigger-1" ||
		got.Reason != "line one\nline two" {
		t.Fatalf("decoded line = %#v, want escaped transition values", got)
	}
}

func TestRenderWatchTransitionOmitsAbsentOptionalFieldsAndWritesOnce(t *testing.T) {
	output := &countingWriter{}
	err := RenderWatchTransition(output, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		Sequence:     0,
		EventTime:    time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
		Terminal:     true,
	})
	if err != nil {
		t.Fatalf("RenderWatchTransition() error = %v", err)
	}
	if output.calls != 1 {
		t.Fatalf("writer calls = %d, want one atomic line write", output.calls)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &fields); err != nil {
		t.Fatalf("decode rendered line: %v", err)
	}
	if _, ok := fields["triggerWorkId"]; ok {
		t.Fatal("rendered line contains absent triggerWorkId")
	}
	if _, ok := fields["reason"]; ok {
		t.Fatal("rendered line contains absent reason")
	}
}

func TestRenderWatchTransitionRejectsInvalidInputBeforeWriting(t *testing.T) {
	output := &countingWriter{}
	err := RenderWatchTransition(output, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		EventTime:    time.Now().UTC(),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
		Sequence:     -1,
	})
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("error = %v, want non-negative sequence validation", err)
	}
	if output.calls != 0 {
		t.Fatalf("writer calls = %d, want zero on validation failure", output.calls)
	}
}

func TestValidateWatchConfigDistinguishesDefaultAndEmptyExplicitSession(t *testing.T) {
	base := WatchConfig{Context: context.Background(), Output: io.Discard}
	if err := ValidateWatchConfig(base); err != nil {
		t.Fatalf("default session config error = %v", err)
	}
	base.SessionID = "session-beta"
	base.SessionIDExplicit = true
	if err := ValidateWatchConfig(base); err != nil {
		t.Fatalf("explicit session config error = %v", err)
	}
	base.SessionID = ""
	if err := ValidateWatchConfig(base); err == nil || !strings.Contains(err.Error(), "--session") {
		t.Fatalf("empty explicit session error = %v, want actionable --session error", err)
	}
}

func TestRenderWatchTransitionReportsShortWrite(t *testing.T) {
	err := RenderWatchTransition(shortWriter{}, WatchTransition{
		SessionID:    "session-1",
		EventID:      "event-1",
		EventTime:    time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC),
		WorkID:       "work-1",
		WorkTypeName: "task",
		FromState:    "init",
		ToState:      "complete",
		Source:       "worker",
	})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v, want io.ErrShortWrite", err)
	}
}

type countingWriter struct {
	bytes.Buffer
	calls int
}

func (w *countingWriter) Write(payload []byte) (int, error) {
	w.calls++
	return w.Buffer.Write(payload)
}

type shortWriter struct{}

func (shortWriter) Write(payload []byte) (int, error) {
	return len(payload) - 1, nil
}
