// Functional owner: sessions/chat_sessions/acp.
package acp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"
)

// The customer boundary is Process.Execute + ACP framing. Caller-owned memory
// pipes model EOF; they make no claim about OS pipes, signals or a built CLI.
type memoryACPConnection struct {
	input  *io.PipeWriter
	output *bufio.Reader
	done   chan error
	once   sync.Once
}

func startMemoryACPConnection(t *testing.T, cohort *controlledACPCohort, cwd string) *memoryACPConnection {
	t.Helper()
	in, input := io.Pipe()
	out, output := io.Pipe()
	connection := &memoryACPConnection{input: input, output: bufio.NewReader(out), done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	env := append(os.Environ(), "HOME="+cohort.home, "USERPROFILE="+cohort.home,
		"HOMEDRIVE="+filepath.VolumeName(cohort.home), "HOMEPATH="+strings.TrimPrefix(cohort.home, filepath.VolumeName(cohort.home)),
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=codex", "YOU_DEFAULT_WORKER_MODEL=gpt-5")
	var stderr bytes.Buffer
	go func() {
		err := cohort.process.Execute(root.Input{Args: []string{"you", "server", "acp"}, Env: env,
			Stdin: in, Stdout: output, Stderr: &stderr, Context: ctx, WorkingDirectory: cwd})
		connection.done <- err
		_ = output.Close()
	}()
	t.Cleanup(func() {
		connection.close(t)
		cancel()
		_ = in.Close()
		_ = out.Close()
	})
	return connection
}

func (connection *memoryACPConnection) close(t *testing.T) {
	t.Helper()
	connection.once.Do(func() {
		_ = connection.input.Close()
		// Completion, not elapsed time, releases the witness after EOF cleanup.
		select {
		case err := <-connection.done:
			if err != nil {
				t.Errorf("ACP Execute after EOF: %v", err)
			}
		case <-time.After(60 * time.Second):
			t.Error("ACP Execute did not join after EOF")
		}
	})
}

func (connection *memoryACPConnection) request(t *testing.T, id, method string, params any) (serveACPLine, []acpsdk.SessionNotification) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.input.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	var notifications []acpsdk.SessionNotification
	for {
		var response serveACPLine
		if err := json.Unmarshal(readServeACPLine(t, connection.output), &response); err != nil {
			t.Fatal(err)
		}
		if response.Method == "session/update" {
			notifications = append(notifications, response.Params)
			continue
		}
		if strings.Trim(string(response.ID), `"`) != id || response.Error != nil {
			t.Fatalf("%s response: %+v", method, response)
		}
		return response, notifications
	}
}

func memoryACPSession(t *testing.T, cohort *controlledACPCohort, connection *memoryACPConnection, cwd string) string {
	t.Helper()
	response, _ := connection.request(t, "new", "session/new", map[string]any{"cwd": cwd, "mcpServers": []any{}})
	var created acpsdk.NewSessionResponse
	if err := json.Unmarshal(response.Result, &created); err != nil {
		t.Fatal(err)
	}
	sessionID := string(created.SessionId)
	// Cleanup uses the existing public close helper after all witness connections
	// have joined. It does not close the session at A's disconnect.
	trackChatSessionOnServer(t, controlledACPServerForCohort(t, cohort), sessionID)
	return sessionID
}

func memoryACPPrompt(t *testing.T, connection *memoryACPConnection, id, sessionID, prompt, answer string) (string, []acpsdk.SessionNotification) {
	t.Helper()
	response, notifications := connection.request(t, id, "session/prompt", map[string]any{
		"sessionId": sessionID, "prompt": []map[string]string{{"type": "text", "text": prompt}},
	})
	var result acpsdk.PromptResponse
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.StopReason != acpsdk.StopReasonEndTurn || !strings.Contains(agentMessageText(t, notifications), answer) ||
		!strings.Contains(workerToolCallText(notifications), answer) {
		t.Fatalf("%s result=%+v, updates=%+v, want %q", id, result, notifications, answer)
	}
	for _, notification := range notifications {
		if string(notification.SessionId) != sessionID {
			t.Fatalf("cross-session update: %+v", notification)
		}
	}
	return promptResultAttachmentID(t, response.Result), notifications
}

func retainedOpeningIDs(t *testing.T, notifications []acpsdk.SessionNotification) []string {
	t.Helper()
	var ids []string
	for _, notification := range notifications {
		if call := notification.Update.ToolCall; call != nil {
			if call.ToolCallId == "" {
				t.Fatal("worker opening has no retained identity")
			}
			ids = append(ids, string(call.ToolCallId))
		}
	}
	if len(ids) == 0 {
		raw, _ := json.Marshal(notifications)
		t.Fatalf("no identified worker openings: %s", raw)
	}
	return ids
}

func TestACPDisconnectPreservesPeerAndResumeCursor(t *testing.T) {
	t.Parallel()
	cohort := controlledACPCohortForTest(t)
	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "disconnect-resume")
	a := startMemoryACPConnection(t, cohort, cwd)
	sessionID := memoryACPSession(t, cohort, a, cwd)
	attachmentID, first := memoryACPPrompt(t, a, "first", sessionID, "pursue the first goal", "first turn answer")
	firstIDs := retainedOpeningIDs(t, first)
	b := startMemoryACPConnection(t, cohort, cwd)
	peerSessionID := memoryACPSession(t, cohort, b, cwd)
	if peerSessionID == sessionID {
		t.Fatal("peer session identity reused")
	}
	params := map[string]any{"sessionId": sessionID, "cwd": cwd, "mcpServers": []any{}}
	loaded, before := b.request(t, "peer-load", "session/load", params)
	peerAttachmentID := promptResultAttachmentID(t, loaded.Result)
	if !reflect.DeepEqual(retainedOpeningIDs(t, before), firstIDs) {
		t.Fatal("peer did not read first retained worker opening")
	}
	a.close(t)
	// F-ACP-PEER: A has joined; B still drives its own Factory/Chat Session
	// and keeps its preexisting delivery identity on S.
	_, peerUpdates := memoryACPPrompt(t, b, "peer", peerSessionID, "pursue the third goal", "third turn answer")
	if strings.Contains(agentMessageText(t, peerUpdates), "first turn answer") {
		t.Fatal("peer answered from S")
	}
	again, duplicates := b.request(t, "peer-load-again", "session/load", params)
	if promptResultAttachmentID(t, again.Result) != peerAttachmentID || len(duplicates) != 0 {
		t.Fatal("peer cursor changed after A disconnected")
	}
	c := startMemoryACPConnection(t, cohort, cwd)
	resumed, replay := c.request(t, "resume", "session/resume", map[string]any{
		"sessionId": sessionID, "cwd": cwd, "_meta": map[string]string{"portpowered.infinite-you/attachment-id": attachmentID},
	})
	if promptResultAttachmentID(t, resumed.Result) != attachmentID || len(replay) != 0 {
		t.Fatal("resume lost identity or replayed acknowledged history")
	}
	// F-ACP-RESUME: only the next turn's distinct answer and identities arrive.
	nextAttachmentID, second := memoryACPPrompt(t, c, "second", sessionID, "pursue the second goal", "second turn answer")
	if nextAttachmentID != attachmentID || strings.Contains(agentMessageText(t, second), "first turn answer") {
		t.Fatal("resumed prompt reused earlier answer or changed identity")
	}
	secondIDs := retainedOpeningIDs(t, second)
	for _, earlier := range firstIDs {
		for _, later := range secondIDs {
			if earlier == later {
				t.Fatal("resumed prompt replayed an acknowledged worker opening")
			}
		}
	}
	assertMemoryACPRetainedHistory(t, cohort, cwd, sessionID, append(firstIDs, secondIDs...), before)
	t.Log("F-ACP-PEER/RESUME/LOAD: isolated peer answer, stable attachments, ordered retained update identities and no acknowledged duplicates")
}

func assertMemoryACPRetainedHistory(t *testing.T, cohort *controlledACPCohort, cwd, sessionID string, wantIDs []string, before []acpsdk.SessionNotification) {
	t.Helper()
	d := startMemoryACPConnection(t, cohort, cwd)
	params := map[string]any{"sessionId": sessionID, "cwd": cwd, "mcpServers": []any{}}
	loaded, history := d.request(t, "fresh-load", "session/load", params)
	if ids := retainedOpeningIDs(t, history); !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("retained ordered worker opening IDs = %q, want %q", ids, wantIDs)
	}
	assertRetainedConversation(t, history, before, wantIDs)
	repeated, duplicateHistory := d.request(t, "repeat-load", "session/load", params)
	if len(duplicateHistory) != 0 || promptResultAttachmentID(t, repeated.Result) != promptResultAttachmentID(t, loaded.Result) {
		t.Fatal("repeat load changed identity or redelivered acknowledged history")
	}
}

// Retained user messages and worker openings carry the stable public identities;
// the live primary-result chunk currently has no messageId. Together these
// witnesses preserve answer correctness and the retained aggregate order.
func assertRetainedConversation(t *testing.T, history, before []acpsdk.SessionNotification, wantIDs []string) {
	t.Helper()
	var prompts []string
	var firstUserID string
	for _, notification := range before {
		if chunk := notification.Update.UserMessageChunk; chunk != nil && chunk.MessageId != nil {
			firstUserID = *chunk.MessageId
		}
	}
	var markers []string
	for _, notification := range history {
		if chunk := notification.Update.UserMessageChunk; chunk != nil && chunk.Content.Text != nil {
			if chunk.MessageId == nil || *chunk.MessageId == "" {
				t.Fatal("retained user prompt lacks identity")
			}
			if len(prompts) == 0 && *chunk.MessageId != firstUserID {
				t.Fatal("load changed retained first prompt identity")
			}
			prompts = append(prompts, chunk.Content.Text.Text)
			markers = append(markers, chunk.Content.Text.Text)
		}
		if call := notification.Update.ToolCall; call != nil {
			markers = append(markers, string(call.ToolCallId))
		}
	}
	if len(wantIDs) != 2 || !reflect.DeepEqual(markers, []string{"pursue the first goal", wantIDs[0], "pursue the second goal", wantIDs[1]}) {
		t.Fatalf("retained aggregate order = %q", markers)
	}
	if !reflect.DeepEqual(prompts, []string{"pursue the first goal", "pursue the second goal"}) {
		t.Fatalf("retained prompt order = %q", prompts)
	}
}
