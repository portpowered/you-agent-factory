// Functional owner: sessions/chat_sessions/acp.
package acp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/wiretranscript"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"
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

	t.Parallel()
	cohort := controlledACPCohortForTest(t)
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

// selectedChatWall intentionally has no timer or logical-tick capability.
type selectedChatWall struct{ nanos atomic.Int64 }

func (source *selectedChatWall) Now() time.Time { return time.Unix(0, source.nanos.Load()).UTC() }

func TestSelectedProcessClockStampsChatTurnsAndRuntimeArtifacts(t *testing.T) {
	t.Parallel()
	for _, year := range []int{2041, 2042} {
		t.Run(fmt.Sprint(year), func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			cwd := t.TempDir()
			seedEveryInstalledPackagedFactory(t, home)
			seedProjectPackagedFactory(t, cwd, controlledACPFactory)
			support.SeedACPAgentProfile(t, home, "factory:"+controlledACPFactory, []string{"factory:" + controlledACPFactory})
			source := &selectedChatWall{}
			runner := &controlledACPCommandRunner{}
			base := time.Date(year, 2, 3, 4, 5, 6, 0, time.UTC)
			source.nanos.Store(base.UnixNano())
			// Different selected sources require distinct immutable process graphs.
			process, err := buildChatProcess(t, "selected wall", serviceedges.Edges{
				Clock: source, ProviderCommandRunner: runner,
				FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			closeProcessCleanly(t, process)
			stdin, stdout := startServeACPProcess(t, process, home, cwd)
			sessionID := driveServeACPSessionNew(t, stdin, stdout, cwd)
			for index, instant := range []time.Time{base, base.Add(7 * time.Second)} {
				source.nanos.Store(instant.UnixNano())
				requestID := fmt.Sprintf("selected-time-%d", index)
				response, notifications := driveIdentifiedSessionPrompt(t, stdin, stdout, requestID, sessionID, "pursue the first goal")
				if response.Error != nil {
					t.Fatalf("prompt: %+v", response.Error)
				}
				if !strings.Contains(agentMessageText(t, notifications), "first turn answer") {
					t.Fatal("missing customer answer")
				}
				assertSelectedTranscriptFrame(t, home, base, requestID, instant)
			}

			assertSelectedChatRedelivery(t, stdin, stdout, sessionID, runner, source, base.Add(8*time.Second))
			assertSelectedChatFailureRecovery(t, stdin, stdout, sessionID, source, base)
			// Re-read the first fact after advancing and executing another turn.
			assertSelectedTranscriptFrame(t, home, base, "selected-time-0", base)
		})
	}
}

func assertSelectedTranscriptFrame(t *testing.T, home string, openedAt time.Time, requestID string, instant time.Time) {
	t.Helper()
	pattern := filepath.Join(wiretranscript.Root(home), openedAt.Format("2006"), openedAt.Format("01"), openedAt.Format("02"), openedAt.Format("150405.000000000")+"-*")
	paths, err := filepath.Glob(pattern)
	if err != nil || len(paths) != 1 {
		t.Fatalf("transcript %q = %v, %v", pattern, paths, err)
	}
	file, err := os.Open(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	found := false
	for scanner.Scan() {
		var record wiretranscript.Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		var frame struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(record.Frame, &frame) == nil && frame.ID == requestID {
			if record.Timestamp != instant.Format(time.RFC3339Nano) {
				t.Fatalf("frame %q time = %q, want %v", requestID, record.Timestamp, instant)
			}
			found = true
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("transcript missing request %q", requestID)
	}
}

func assertSelectedChatRedelivery(t *testing.T, stdin *os.File, stdout *bufio.Reader, sessionID string, runner *controlledACPCommandRunner, source *selectedChatWall, advancedAt time.Time) {
	t.Helper()
	before := runner.requestCount()
	source.nanos.Store(advancedAt.UnixNano())
	response, _ := driveIdentifiedSessionPrompt(t, stdin, stdout, "selected-time-1", sessionID, "pursue the first goal")
	if response.Error != nil {
		t.Fatalf("redelivery: %+v", response.Error)
	}
	var decoded acpsdk.PromptResponse
	if err := json.Unmarshal(response.Result, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.StopReason != acpsdk.StopReasonEndTurn || runner.requestCount() != before {
		t.Fatalf("redelivery = %#v; provider calls before=%d after=%d", decoded, before, runner.requestCount())
	}
}

func assertSelectedChatFailureRecovery(t *testing.T, stdin *os.File, stdout *bufio.Reader, sessionID string, source *selectedChatWall, base time.Time) {
	t.Helper()
	source.nanos.Store(base.Add(9 * time.Second).UnixNano())
	failure, _ := driveIdentifiedSessionPrompt(t, stdin, stdout, "selected-failure", sessionID, "please help [cohort-failure]")
	if failure.Error == nil || failure.Error.Code != internalErrorCode {
		t.Fatalf("malformed result = %#v", failure)
	}
	source.nanos.Store(base.Add(10 * time.Second).UnixNano())
	recovered, notifications := driveIdentifiedSessionPrompt(t, stdin, stdout, "selected-recovery", sessionID, "please help [selected-recovery]")
	if recovered.Error != nil || !strings.Contains(agentMessageText(t, notifications), "selected recovery answer") {
		t.Fatalf("later eligible turn = %#v", recovered)
	}
}
