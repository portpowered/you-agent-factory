package kiro

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

const (
	newSessionID     = "f2946a26-3735-4b08-8d05-c928010302d5"
	resumedSessionID = "675f9238-5f05-456c-9a9f-f8fe486f49e4"
)

func TestResponseFromOutputNormalizesProviderSessions(t *testing.T) {
	t.Parallel()

	requested := newKiroSession(resumedSessionID)
	testCases := []struct {
		name      string
		stdout    string
		stderr    string
		requested *inference.ProviderSession
		wantID    string
	}{
		{
			name:   "new session",
			stdout: "new session answer",
			stderr: `{"event":"session.created","session_id":"` + newSessionID + `"}`,
			wantID: newSessionID,
		},
		{
			name:      "resumed session is preserved without emitted metadata",
			stdout:    "continued answer",
			requested: &requested,
			wantID:    resumedSessionID,
		},
		{
			name:      "emitted session updates resumed session",
			stdout:    "continued answer",
			stderr:    `{"session_id":"` + newSessionID + `"}`,
			requested: &requested,
			wantID:    newSessionID,
		},
		{
			name:   "absent session stays absent",
			stdout: "answer without session metadata",
		},
		{
			name:   "empty structured session stays absent",
			stdout: "answer with empty session metadata",
			stderr: `{"session_id":""}`,
		},
		{
			name:      "malformed structured session preserves resumed session",
			stdout:    "valid answer",
			stderr:    `{"session_id":"not-a-uuid"}`,
			requested: &requested,
			wantID:    resumedSessionID,
		},
		{
			name:   "arbitrary text is not a session",
			stdout: "answer mentioning session_id: " + newSessionID,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			response := responseFromOutput(
				[]byte(testCase.stdout),
				[]byte(testCase.stderr),
				testCase.requested,
			)
			if response.Content() != testCase.stdout {
				t.Fatalf("Content() = %q, want %q", response.Content(), testCase.stdout)
			}
			session := response.ProviderSession()
			if testCase.wantID == "" {
				if session != nil {
					t.Fatalf("ProviderSession() = %#v, want nil", session)
				}
				return
			}
			if session == nil ||
				session.Provider() != "kiro" ||
				session.Kind() != providerSessionKind ||
				session.ID() != testCase.wantID {
				t.Fatalf("ProviderSession() = %#v, want kiro/session_id/%s", session, testCase.wantID)
			}
		})
	}
}

func TestIntegrationWritesFinalOnlyLifecycleAndClosesOnce(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{result: workerprocess.CommandResult{
		Stdout: []byte("Kiro completed the work."),
		Stderr: []byte(`{"session_id":"` + newSessionID + `"}`),
	}}
	writer := &recordingWriter{}
	integration := NewIntegration(IntegrationDependencies{CommandRunner: runner})

	err := inference.ExecuteInvocation(
		context.Background(),
		integration,
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-kiro-response",
			UserMessage:  "complete the task",
		}),
		writer,
	)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if writer.closes != 1 {
		t.Fatalf("writer closes = %d, want 1", writer.closes)
	}
	if len(writer.events) != 3 {
		t.Fatalf("events = %d, want 3", len(writer.events))
	}
	assertFinalOnlyLifecycle(t, writer.events, "Kiro completed the work.")

	response := writer.completion.Response()
	if response == nil || response.Content() != "Kiro completed the work." {
		t.Fatalf("completion response = %#v", response)
	}
	session := response.ProviderSession()
	if session == nil || session.ID() != newSessionID ||
		session.Provider() != "kiro" || session.Kind() != providerSessionKind {
		t.Fatalf("completion provider session = %#v", session)
	}
}

func TestIntegrationUsesAuthoritativeRequestedSessionForResume(t *testing.T) {
	t.Parallel()

	requested := newKiroSession(resumedSessionID)
	runner := &recordingRunner{result: workerprocess.CommandResult{
		Stdout: []byte("resumed answer"),
	}}
	writer := &recordingWriter{}
	err := NewIntegration(IntegrationDependencies{CommandRunner: runner}).Invoke(
		context.Background(),
		inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID:    "inv-kiro-resume",
			UserMessage:     "continue",
			ProviderSession: &requested,
		}),
		writer,
	)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if !containsArgPair(runner.request.Args, "--resume-id", resumedSessionID) {
		t.Fatalf("command args = %#v, want --resume-id %s", runner.request.Args, resumedSessionID)
	}
	session := writer.completion.Response().ProviderSession()
	if session == nil || session.ID() != resumedSessionID {
		t.Fatalf("completion provider session = %#v, want preserved session", session)
	}
}

func assertFinalOnlyLifecycle(t *testing.T, events []inference.EventDraft, content string) {
	t.Helper()
	drafts := make([]workers.Draft, 0, len(events))
	for _, event := range events {
		drafts = append(drafts, event.Draft())
	}
	if drafts[0].Kind != workers.KindRun || drafts[0].Phase != workers.PhaseStarted ||
		drafts[1].Kind != workers.KindMessage || drafts[1].Phase != workers.PhaseCompleted ||
		drafts[2].Kind != workers.KindRun || drafts[2].Phase != workers.PhaseCompleted {
		t.Fatalf("final-only lifecycle = %#v", drafts)
	}
	var payload workers.MessagePayload
	if err := json.Unmarshal(drafts[1].Payload, &payload); err != nil {
		t.Fatalf("decode message payload: %v", err)
	}
	if len(payload.ContentBlocks) != 1 || payload.ContentBlocks[0].Text != content {
		t.Fatalf("message payload = %#v, want content %q", payload, content)
	}
}

func newKiroSession(id string) inference.ProviderSession {
	return inference.NewProviderSession("kiro", providerSessionKind, id, nil)
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

type recordingRunner struct {
	request workerprocess.CommandRequest
	result  workerprocess.CommandResult
	err     error
	calls   int
}

func (r *recordingRunner) Run(
	_ context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	r.calls++
	r.request = request
	return r.result, r.err
}

type recordingWriter struct {
	events     []inference.EventDraft
	completion inference.Completion
	closes     int
}

func (w *recordingWriter) WriteEvent(_ context.Context, event inference.EventDraft) error {
	w.events = append(w.events, event)
	return nil
}

func (w *recordingWriter) Close(_ context.Context, completion inference.Completion) error {
	w.closes++
	w.completion = completion
	return nil
}
