package runtime

import (
	"context"
	"strings"
	"testing"

	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
)

type recordingLogger struct {
	entries []logEntry
}

type logEntry struct {
	level   string
	message string
	fields  map[string]any
}

func (l *recordingLogger) Debug(msg string, keysAndValues ...any) {
	l.record("debug", msg, keysAndValues...)
}

func (l *recordingLogger) Info(msg string, keysAndValues ...any) {
	l.record("info", msg, keysAndValues...)
}

func (l *recordingLogger) Warn(msg string, keysAndValues ...any) {
	l.record("warn", msg, keysAndValues...)
}

func (l *recordingLogger) Error(msg string, keysAndValues ...any) {
	l.record("error", msg, keysAndValues...)
}

func (l *recordingLogger) Verbose(msg string, keysAndValues ...any) {
	l.record("verbose", msg, keysAndValues...)
}

func (l *recordingLogger) record(level, msg string, keysAndValues ...any) {
	fields := map[string]any{}
	for index := 0; index+1 < len(keysAndValues); index += 2 {
		key, ok := keysAndValues[index].(string)
		if !ok {
			continue
		}
		fields[key] = keysAndValues[index+1]
	}
	l.entries = append(l.entries, logEntry{
		level:   level,
		message: msg,
		fields:  fields,
	})
}

func TestPauseResume_DiagnosticsLogAcceptedTransitions(t *testing.T) {
	logger := &recordingLogger{}
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logger),
		factory.WithWorkflowContext(&factory_context.FactoryContext{SessionID: "session-diagnostics"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	pauseEntry := findLogEntry(t, logger.entries, "factory runtime lifecycle control", "operation", "PAUSE")
	if pauseEntry.fields["outcome"] != "ACCEPTED" {
		t.Fatalf("pause outcome = %#v, want ACCEPTED", pauseEntry.fields["outcome"])
	}
	if pauseEntry.fields["session_id"] != "session-diagnostics" {
		t.Fatalf("pause session_id = %#v, want session-diagnostics", pauseEntry.fields["session_id"])
	}

	resumeEntry := findLogEntry(t, logger.entries, "factory runtime lifecycle control", "operation", "RESUME")
	if resumeEntry.fields["outcome"] != "ACCEPTED" {
		t.Fatalf("resume outcome = %#v, want ACCEPTED", resumeEntry.fields["outcome"])
	}
}

func TestResume_DiagnosticsLogPostResumeBufferedDrain(t *testing.T) {
	logger := &recordingLogger{}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logger),
		factory.WithWorkflowContext(&factory_context.FactoryContext{SessionID: "session-drain"}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	impl := f.(*factoryImpl)
	if !impl.resultBuffer.Write(ctx, interfaces.WorkResult{
		DispatchID: "dispatch-drain",
		Outcome:    interfaces.OutcomeAccepted,
	}) {
		t.Fatal("buffered result write failed")
	}

	if err := f.Resume(ctx); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	findLogEntry(t, logger.entries, "factory runtime resume buffered results pending drain", "buffered_result_count", 1)

	impl.observePostResumeBufferedDrain(1)
	drainEntry := findLogEntry(t, logger.entries, "factory runtime resume buffered results drained", "drained_result_count", 1)
	if drainEntry.fields["session_id"] != "session-drain" {
		t.Fatalf("drain session_id = %#v, want session-drain", drainEntry.fields["session_id"])
	}
}

func TestObservePostResumeBufferedDrain_IgnoresDrainWhenResumeWasNotPending(t *testing.T) {
	logger := &recordingLogger{}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	impl := f.(*factoryImpl)
	impl.observePostResumeBufferedDrain(2)
	for _, entry := range logger.entries {
		if entry.message == "factory runtime resume buffered results drained" {
			t.Fatalf("unexpected drain log without pending resume: %#v", entry)
		}
	}
}

func TestPauseResume_DiagnosticsAvoidPayloadAndPathFields(t *testing.T) {
	logger := &recordingLogger{}
	f, err := New(
		factory.WithNet(buildMoveControlNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	for _, entry := range logger.entries {
		for key, value := range entry.fields {
			text, ok := value.(string)
			if !ok {
				continue
			}
			if strings.Contains(text, "/Users/") || strings.Contains(strings.ToLower(key), "payload") {
				t.Fatalf("diagnostic log leaked sensitive field %q=%q in %q", key, text, entry.message)
			}
		}
	}
}

func findLogEntry(t *testing.T, entries []logEntry, message, fieldKey string, want any) logEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.message != message {
			continue
		}
		if entry.fields[fieldKey] == want {
			return entry
		}
	}
	t.Fatalf("log entry %q with %q=%#v not found in %#v", message, fieldKey, want, entries)
	return logEntry{}
}

var _ logging.Logger = (*recordingLogger)(nil)
