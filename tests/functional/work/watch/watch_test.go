package watch_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const (
	workWatchWorkType = "task"
	workWatchWorkName = "watch-terminal-regression"
)

// TestWorkWatchFollowsStateTransitionsUntilTerminal proves the customer
// outcome for the public watch command: one CLI submission followed by one
// finite CLI watch invocation observes canonical Work transitions through the
// terminal transition and exits successfully.
func TestWorkWatchFollowsStateTransitionsUntilTerminal(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, workWatchFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)
	streamGate := newWorkWatchStreamGate(t, server.URL())

	process := workWatchProcess

	payloadPath := filepath.Join(t.TempDir(), "watch-request.md")
	if err := os.WriteFile(payloadPath, []byte("# observe terminal work\n"), 0o600); err != nil {
		t.Fatalf("write submit payload: %v", err)
	}

	sessionID := factorysessions.DefaultSessionID
	submitInputs := workWatchInputs(t, []string{
		"you", "--server", server.URL(), "--json", "submit",
		"--session", sessionID,
		"--name", workWatchWorkName,
		"--work-type-name", workWatchWorkType,
		"--payload", payloadPath,
	})
	if err := process.Execute(submitInputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(submit) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			submitInputs.Stdout(),
			submitInputs.Stderr(),
		)
	}
	workID := decodeSubmittedWorkID(t, submitInputs.Stdout())

	watchInputs := workWatchInputs(t, []string{
		"you", "--server", streamGate.URL(), "work", "watch",
		"--session", sessionID,
	})
	watchCommand := support.StartProcessCommand(t, process, watchInputs.Input)
	streamGate.wait(t)
	for _, state := range []string{"processing", "complete"} {
		moveInputs := workWatchInputs(t, []string{
			"you", "--server", server.URL(), "--json", "work", "move",
			workID, state, "--session", sessionID,
		})
		if err := process.Execute(moveInputs.Input); err != nil {
			t.Fatalf(
				"Process.Execute(work move %s) error = %v\nstdout:\n%s\nstderr:\n%s",
				state,
				err,
				moveInputs.Stdout(),
				moveInputs.Stderr(),
			)
		}
	}
	awaitWorkWatchCompletion(t, watchCommand, watchInputs)

	lines := decodeWatchLines(t, watchInputs.Stdout())
	wantTransitions := [][2]string{{"init", "processing"}, {"processing", "complete"}}
	assertWorkWatchTransitionLines(t, lines, sessionID, workID, wantTransitions)
	if !strings.HasSuffix(watchInputs.Stdout(), "\n") {
		t.Fatalf("work watch output does not end with a complete line: %q", watchInputs.Stdout())
	}
	functionalevidence.Covers(t, "cli/you.work.watch")
}

// awaitWorkWatchCompletion waits for the finite watch invocation to exit and
// asserts it completed cleanly with no stderr diagnostics.
func awaitWorkWatchCompletion(t *testing.T, watchCommand *support.ProcessCommand, watchInputs *support.CapturedInputs) {
	t.Helper()
	select {
	case <-watchCommand.Done():
	case <-t.Context().Done():
		t.Fatalf("work watch context canceled before finite completion: %v", t.Context().Err())
	}
	if err := watchCommand.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(work watch) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			watchInputs.Stdout(),
			watchInputs.Stderr(),
		)
	}
	if strings.TrimSpace(watchInputs.Stderr()) != "" {
		t.Fatalf("work watch wrote diagnostics without verbose mode: %q", watchInputs.Stderr())
	}
}

// assertWorkWatchTransitionLines asserts the decoded watch lines match the
// expected ordered transitions and terminal placement for workID.
func assertWorkWatchTransitionLines(t *testing.T, lines []workWatchLine, sessionID string, workID string, wantTransitions [][2]string) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("work watch emitted no transition lines")
	}
	if len(lines) != len(wantTransitions) {
		t.Fatalf("work watch transition count = %d, want %d: %#v", len(lines), len(wantTransitions), lines)
	}
	for index, line := range lines {
		if line.SchemaVersion != "you.work.watch.v1" {
			t.Fatalf("watch line %d schemaVersion = %q, want you.work.watch.v1", index, line.SchemaVersion)
		}
		if line.SessionID != sessionID {
			t.Fatalf("watch line %d sessionId = %q, want %q", index, line.SessionID, sessionID)
		}
		if line.EventID == "" || line.Sequence < 0 || line.EventTime.IsZero() {
			t.Fatalf("watch line %d missing canonical event identity/time: %#v", index, line)
		}
		if index > 0 && line.Sequence <= lines[index-1].Sequence {
			t.Fatalf(
				"watch sequences are not strictly increasing at line %d: %d after %d",
				index,
				line.Sequence,
				lines[index-1].Sequence,
			)
		}
		if line.WorkID != workID {
			t.Fatalf("watch line %d workId = %q, want submitted %q", index, line.WorkID, workID)
		}
		if line.WorkTypeName != workWatchWorkType {
			t.Fatalf("watch line %d workTypeName = %q, want %q", index, line.WorkTypeName, workWatchWorkType)
		}
		if line.FromState == "" || line.ToState == "" || line.FromState == line.ToState || line.Source == "" {
			t.Fatalf("watch line %d is not a transition: %#v", index, line)
		}
		if line.FromState != wantTransitions[index][0] || line.ToState != wantTransitions[index][1] {
			t.Fatalf(
				"watch line %d transition = %s -> %s, want %s -> %s",
				index,
				line.FromState,
				line.ToState,
				wantTransitions[index][0],
				wantTransitions[index][1],
			)
		}
		if index < len(lines)-1 && line.Terminal {
			t.Fatalf("watch line %d is terminal before the final line: %#v", index, line)
		}
	}
	if !lines[len(lines)-1].Terminal {
		t.Fatalf("last watch line is not terminal: %#v", lines[len(lines)-1])
	}
}

type workWatchLine struct {
	SchemaVersion    string          `json:"schemaVersion"`
	SessionID        string          `json:"sessionId"`
	EventID          string          `json:"eventId"`
	Sequence         int64           `json:"sequence"`
	EventTime        time.Time       `json:"eventTime"`
	WorkID           string          `json:"workId"`
	WorkTypeName     string          `json:"workTypeName"`
	FromState        string          `json:"fromState"`
	ToState          string          `json:"toState"`
	Source           string          `json:"source"`
	Terminal         bool            `json:"terminal"`
	StructuredResult json.RawMessage `json:"structuredResult"`
}

func decodeSubmittedWorkID(t *testing.T, output string) string {
	t.Helper()
	var response struct {
		WorkID *string `json:"workId"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &response); err != nil {
		t.Fatalf("decode submit response: %v\nstdout:\n%s", err, output)
	}
	if response.WorkID == nil || strings.TrimSpace(*response.WorkID) == "" {
		t.Fatalf("submit response missing workId: %s", output)
	}
	return *response.WorkID
}

func decodeWatchLines(t *testing.T, output string) []workWatchLine {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(output))
	lines := make([]workWatchLine, 0)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			t.Fatalf("work watch emitted a blank stdout line")
		}
		var line workWatchLine
		if err := json.Unmarshal([]byte(scanner.Text()), &line); err != nil {
			t.Fatalf("decode work watch line: %v\nline:\n%s", err, scanner.Text())
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan work watch output: %v", err)
	}
	return lines
}

func workWatchInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = home
	return inputs
}

func workWatchFactoryConfig() map[string]any {
	return map[string]any{
		"name": "work-watch-regression",
		"workTypes": []map[string]any{{
			"name": workWatchWorkType,
			"states": []map[string]any{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
	}
}
