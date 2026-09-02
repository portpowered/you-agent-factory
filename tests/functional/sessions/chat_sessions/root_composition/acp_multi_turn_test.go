// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

// TestACPSessionAnswersEachTurnWithThatTurnsOwnResult proves the property a
// conversation is: turn two is answered by turn two's work, not turn one's.
//
// An ACP session is multi-turn by construction -- one session/new, then a
// prompt per user message -- so every turn after the first exercises a
// distinct case from the single-shot CLI invocation: the previous turn's
// work is already sitting in the session's world state, terminal and
// matching the Factory's configured return policy, while this turn's own
// work is still running.
//
// The failure this pins was silent and total. The Worker genuinely ran and
// its real output was visible inside its own tool call, so a client saw a
// correct-looking turn -- but the assistant message it delivered, and the
// prompt result's attachment identity, were byte-for-byte the previous
// turn's. Every answer from turn two onward was the answer to turn one.
//
// Both halves are asserted deliberately: the Worker's tool call must carry
// this turn's output (proving the Factory really ran, so a regression cannot
// pass by failing to dispatch at all), and the assistant message must carry
// it too (proving the result reached the customer).
func TestACPSessionAnswersEachTurnWithThatTurnsOwnResult(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test driving root.BuildProcess through the you server acp CLI command")
	}

	turns := []struct {
		id     string
		prompt string
		answer string
	}{
		{id: "turn-1", prompt: "pursue the first goal", answer: "first turn answer"},
		{id: "turn-2", prompt: "pursue the second goal", answer: "second turn answer"},
		{id: "turn-3", prompt: "pursue the third goal", answer: "third turn answer"},
	}

	cohort := newControlledACPCohort(t, "multi-turn")
	t.Parallel()
	cwd := controlledACPWorkingDirectoryForCohort(t, cohort, "multi-turn")
	stdin, stdout := startControlledServeACPHarness(t, cohort, cwd)
	sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
	if sessionID == "" {
		t.Fatal("session/new returned a blank sessionId")
	}

	var attachmentIDs []string
	for index, turn := range turns {
		response, notifications := driveIdentifiedSessionPrompt(t, stdin, stdout, turn.id, sessionID, turn.prompt)
		if response.Error != nil {
			t.Fatalf("turn %d (%s) response error = %+v, want a successful result", index+1, turn.id, response.Error)
		}

		var decoded acpsdk.PromptResponse
		if err := json.Unmarshal(response.Result, &decoded); err != nil {
			t.Fatalf("turn %d unmarshal PromptResponse: %v", index+1, err)
		}
		if decoded.StopReason != acpsdk.StopReasonEndTurn {
			t.Fatalf("turn %d stopReason = %q, want %q", index+1, decoded.StopReason, acpsdk.StopReasonEndTurn)
		}

		workerOutput := workerToolCallText(notifications)
		if !strings.Contains(workerOutput, turn.answer) {
			t.Fatalf("turn %d Worker tool-call content = %q, want it to carry this turn's own output %q",
				index+1, workerOutput, turn.answer)
		}

		assistantText := agentMessageText(t, notifications)
		if !strings.Contains(assistantText, turn.answer) {
			t.Fatalf("turn %d assistant text = %q, want this turn's own answer %q",
				index+1, assistantText, turn.answer)
		}
		for _, earlier := range turns[:index] {
			if strings.Contains(assistantText, earlier.answer) {
				t.Fatalf("turn %d assistant text = %q, want it to carry only this turn's answer, not the earlier %q",
					index+1, assistantText, earlier.answer)
			}
		}

		attachmentIDs = append(attachmentIDs, promptResultAttachmentID(t, response.Result))
	}

	// The attachment identity is the session's own resume handle, so it is
	// expected to be stable across turns. Asserting it explicitly keeps this
	// cell honest about which identity may repeat and which may not: the
	// answer must change every turn, the attachment must not.
	for index := 1; index < len(attachmentIDs); index++ {
		if attachmentIDs[index] != attachmentIDs[0] {
			t.Fatalf("turn %d attachment id = %q, want the session's stable %q",
				index+1, attachmentIDs[index], attachmentIDs[0])
		}
	}
}

// driveIdentifiedSessionPrompt is driveServeACPSessionPrompt with a
// caller-chosen request id, so several turns can run on one connection
// without two of them sharing an id.
func driveIdentifiedSessionPrompt(
	t *testing.T,
	stdin *os.File,
	stdout *bufio.Reader,
	requestID, sessionID, text string,
) (serveACPLine, []acpsdk.SessionNotification) {
	t.Helper()
	defer releaseChatACPHomeForInput(stdin)

	params, err := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"prompt":    []map[string]any{{"type": "text", "text": text}},
	})
	if err != nil {
		t.Fatalf("marshal session/prompt params: %v", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":"session/prompt","params":%s}`, requestID, params) + "\n"
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatalf("write session/prompt request: %v", err)
	}

	var notifications []acpsdk.SessionNotification
	for {
		raw := readServeACPLine(t, stdout)
		var decoded serveACPLine
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("unmarshal you server acp line %q: %v", raw, err)
		}
		if decoded.Method == "session/update" {
			notifications = append(notifications, decoded.Params)
			continue
		}
		if strings.Trim(string(decoded.ID), `"`) == requestID {
			return decoded, notifications
		}
		t.Fatalf("unexpected you server acp line before the session/prompt response: %s", raw)
	}
}

// workerToolCallText concatenates the text a turn's Worker delivered inside
// its own tool call, which is where Worker output belongs.
func workerToolCallText(notifications []acpsdk.SessionNotification) string {
	var text string
	for _, notification := range notifications {
		update := notification.Update.ToolCallUpdate
		if update == nil {
			continue
		}
		for _, content := range update.Content {
			if content.Content == nil || content.Content.Content.Text == nil {
				continue
			}
			text += content.Content.Content.Text.Text
		}
	}
	return text
}

// promptResultAttachmentID reads the transport's own resume-attachment
// identity out of one prompt result's _meta.
func promptResultAttachmentID(t *testing.T, result json.RawMessage) string {
	t.Helper()
	var decoded struct {
		Meta map[string]any `json:"_meta"`
	}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("unmarshal prompt result _meta: %v", err)
	}
	id, _ := decoded.Meta["portpowered.infinite-you/attachment-id"].(string)
	if id == "" {
		t.Fatalf("prompt result carries no attachment id: %s", result)
	}
	return id
}
