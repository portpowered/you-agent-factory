package output_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	humanTextStreamScenarioTimeout = 30 * time.Second
	textStreamPrimaryResult        = "mock worker accepted"
	textStreamPromptRunWorkType    = "prompt-task"
)

// TestCLITextStreamSurfacesIncrementalMessages proves a human response-stream
// CLI run surfaces lifecycle progress on stdout while the invocation is still
// in flight, before the terminal primary result is written.
func TestCLITextStreamSurfacesIncrementalMessages(t *testing.T) {
	writer := newFirstChunkGatedStdoutWriter()
	runGoalHumanResponseStreamWithStdout(t, writer)

	waitForFirstChunkStdoutContent(t, writer)
	if !containsHumanLifecycleLine(writer.String()) {
		t.Fatalf("stdout missing incremental human lifecycle output before completion:\n%s", writer.String())
	}
	select {
	case <-writer.done:
		t.Fatal("invocation completed before releasing stdout gate")
	default:
	}

	writer.release()

	select {
	case <-writer.done:
	case <-time.After(humanTextStreamScenarioTimeout):
		t.Fatalf("timed out waiting for invocation to finish after releasing stdout gate")
	}
	if writer.err != nil {
		t.Fatalf("Process.Execute error = %v\nstdout:\n%s", writer.err, writer.String())
	}
	if writer.diagnosticText() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", writer.diagnosticText())
	}

	stdout := writer.String()
	lines := nonEmptyStdoutLines(stdout)
	if len(lines) < 3 {
		t.Fatalf("stdout lines = %#v, want lifecycle, separator, and final response", lines)
	}
	if lines[len(lines)-2] != "--- primary result ---" {
		t.Fatalf("penultimate stdout line = %q, want primary-result separator\nstdout:\n%s", lines[len(lines)-2], stdout)
	}
	if lines[len(lines)-1] != textStreamPrimaryResult {
		t.Fatalf("final stdout line = %q, want %q\nstdout:\n%s", lines[len(lines)-1], textStreamPrimaryResult, stdout)
	}
	for _, line := range lines[:len(lines)-2] {
		if !isHumanFactoryLifecycleLine(line) {
			t.Fatalf("stdout line %q is not canonical customer lifecycle output\nstdout:\n%s", line, stdout)
		}
	}
}

// TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise proves human text
// presentation on stdout stays free of NDJSON envelopes, single-JSON
// InvocationResponse wrappers, retired automation record shapes, and operator
// lifecycle chatter that clean invocation output must suppress.
func TestCLITextStreamDoesNotPrintStructuredEnvelopeNoise(t *testing.T) {
	t.Run("human response-stream lifecycle presentation", func(t *testing.T) {
		stdout := runGoalHumanInvocation(t, []string{"--output", "response-stream"})
		assertHumanStdoutFreeOfStructuredEnvelopeNoise(t, stdout)
		for _, line := range nonEmptyStdoutLines(stdout) {
			if line == "--- primary result ---" || line == textStreamPrimaryResult {
				continue
			}
			if !isHumanFactoryLifecycleLine(line) {
				t.Fatalf("stdout line %q is not canonical customer lifecycle output\nstdout:\n%s", line, stdout)
			}
		}
	})

	t.Run("quiet clean primary result", func(t *testing.T) {
		stdout := runGoalHumanInvocation(t, []string{"--quiet"})
		assertHumanStdoutFreeOfStructuredEnvelopeNoise(t, stdout)
		if strings.TrimSpace(stdout) != textStreamPrimaryResult {
			t.Fatalf("stdout = %q, want only raw primary result %q", stdout, textStreamPrimaryResult)
		}
	})
}

// TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet proves
// a non-quiet operator continuous CLI run with --with-server reports Factory
// initiated and Dashboard URL startup output on stdout.
func TestCLITextStreamOperatorContinuousRunReportsStartupOutputWithoutQuiet(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI operator continuous text-stream startup")
	}

	dir := support.ScaffoldFactory(t, textStreamPromptRunFactoryConfig())
	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	configuredURL := reserveTextStreamLoopbackURL(t)
	wantDashboardURL := configuredURL + "/dashboard/ui"
	wantInitiated := "Factory initiated: " + dir

	mockWorkersPath := writeTextStreamDefaultMockWorkersConfig(t)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	stdout := newInterruptibleStdoutCapture()
	homeDir := t.TempDir()
	args := []string{
		"you", "--server", configuredURL,
		"run", "--factory", factoryPath,
		"--with-mock-workers", "--no-record", "--with-server", "--continuously",
		mockWorkersPath,
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = &stdout.diagnostic
	process := support.BuildProcess(t, serviceedges.Edges{})

	go func() {
		stdout.err = process.Execute(inputs.Input)
		close(stdout.done)
	}()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		output := stdout.String()
		if strings.Contains(output, wantInitiated) && strings.Contains(output, "Dashboard URL: "+wantDashboardURL) {
			cancel()
			goto waitForShutdown
		}
		if err := waitForTextStreamServerReady(ctx, configuredURL); err == nil {
			output = stdout.String()
			if strings.Contains(output, wantInitiated) && strings.Contains(output, "Dashboard URL: "+wantDashboardURL) {
				cancel()
				goto waitForShutdown
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf(
		"timed out waiting for operator startup stdout\nstdout:\n%s\nstderr:\n%s",
		stdout.String(),
		stdout.diagnosticText(),
	)

waitForShutdown:
	select {
	case <-stdout.done:
	case <-time.After(humanTextStreamScenarioTimeout):
		t.Fatalf("timed out waiting for continuous run cancellation\nstdout:\n%s", stdout.String())
	}
}

// TestCLITextStreamInterruptedRunDoesNotClaimCompletion proves interrupting a
// human response-stream CLI run ends with the documented cancellation outcome
// and does not print successful-completion or primary-result claims on stdout.
func TestCLITextStreamInterruptedRunDoesNotClaimCompletion(t *testing.T) {
	externalWork := newCancellableExternalWorkRunner()
	stdout := newInterruptibleStdoutCapture()
	runArgs := runGoalHumanInterruptibleResponseStream(t, externalWork, stdout)

	waitForExternalWorkStart(t, externalWork)
	waitForInterruptibleStdoutLifecycle(t, stdout)
	select {
	case <-stdout.done:
		t.Fatal("invocation completed before interrupt")
	default:
	}

	stdout.cancel()

	select {
	case <-stdout.done:
	case <-time.After(humanTextStreamScenarioTimeout):
		t.Fatalf("timed out waiting for interrupted invocation to finish\nstdout:\n%s", stdout.String())
	}
	if stdout.err == nil {
		t.Fatalf("Process.Execute error = nil, want canceled invocation failure\nstdout:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.err.Error(), "INVOCATION_CANCELED") {
		t.Fatalf("Process.Execute error = %v, want INVOCATION_CANCELED\nstdout:\n%s", stdout.err, stdout.String())
	}
	if got := declaredRunCancelExitCode(t, runArgs); got != 130 {
		t.Fatalf("declared you run cancel exit code = %d, want 130", got)
	}
	if err := externalWork.waitFinished(humanTextStreamScenarioTimeout); err != nil {
		t.Fatalf("external mock-worker work teardown not observed after interrupt: %v", err)
	}
	if !errors.Is(externalWork.runErr(), context.Canceled) {
		t.Fatalf("external work cancellation error = %v, want context.Canceled", externalWork.runErr())
	}

	output := stdout.String()
	if !strings.Contains(output, "--- invocation outcome ---") {
		t.Fatalf("stdout missing invocation outcome after interrupt:\n%s", output)
	}
	if !strings.Contains(output, "status: CANCELED") {
		t.Fatalf("stdout missing canceled status after interrupt:\n%s", output)
	}
	if strings.Contains(output, "--- primary result ---") {
		t.Fatalf("stdout contains primary-result separator after interrupt:\n%s", output)
	}
	for _, line := range nonEmptyStdoutLines(output) {
		if line == textStreamPrimaryResult {
			t.Fatalf("stdout line claims successful primary result after interrupt:\n%s", output)
		}
	}
}

var humanTextStreamForbiddenEnvelopeLiterals = []string{
	`"recordType":"factory_event"`, `"recordType":"invocation_result"`,
	`"recordType":"progress"`, `"recordType":"compaction"`, `"recordType":"primary_result"`,
	"recordType=factory_event", "recordType=invocation_result",
	"recordType=progress", "recordType=compaction", "recordType=primary_result",
	"FactoryResponseEvent", "response_event", "provider_session", "providerSession",
	"textDelta", "toolCallId", "toolCalls",
	`"primary_result":`, `"marking":`, `"placeId":`, `"Petri":`,
}

var humanTextStreamForbiddenOperatorChatter = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Recording saved to",
	"Factory:",
}

func assertHumanStdoutFreeOfStructuredEnvelopeNoise(t *testing.T, stdout string) {
	t.Helper()

	for _, forbidden := range humanTextStreamForbiddenEnvelopeLiterals {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains structured envelope noise %q:\n%s", forbidden, stdout)
		}
	}
	for _, forbidden := range humanTextStreamForbiddenOperatorChatter {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout contains operator lifecycle chatter %q:\n%s", forbidden, stdout)
		}
	}
	for _, line := range nonEmptyStdoutLines(stdout) {
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var record struct {
			RecordType string `json:"recordType"`
		}
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record.RecordType != "" {
			t.Fatalf("stdout line decodes as NDJSON recordType %q:\n%s", record.RecordType, line)
		}
	}
	if trimmed := strings.TrimSpace(stdout); strings.HasPrefix(trimmed, "{") {
		var response factoryapi.InvocationResponse
		if err := json.Unmarshal([]byte(trimmed), &response); err == nil && response.Status != "" {
			t.Fatalf("stdout decodes as single-JSON InvocationResponse wrapper:\n%s", stdout)
		}
	}
}

func runGoalHumanInvocation(t *testing.T, runArgs []string) string {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
	}
	args = append(args, runArgs...)
	args = append(args, "deterministic human text-stream envelope contract")
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}
	return inputs.Stdout()
}

type firstChunkGatedStdoutWriter struct {
	gate        chan struct{}
	releaseOnce sync.Once

	attempts       atomic.Int64
	firstChunkSeen atomic.Bool

	mu         sync.Mutex
	buffer     bytes.Buffer
	diagnostic bytes.Buffer

	done chan struct{}
	err  error
}

func newInterruptibleStdoutCapture() *interruptibleStdoutCapture {
	return &interruptibleStdoutCapture{done: make(chan struct{})}
}

func newFirstChunkGatedStdoutWriter() *firstChunkGatedStdoutWriter {
	return &firstChunkGatedStdoutWriter{
		gate: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (writer *firstChunkGatedStdoutWriter) Write(payload []byte) (int, error) {
	writer.attempts.Add(1)
	if !writer.firstChunkSeen.Swap(true) {
		writer.mu.Lock()
		defer writer.mu.Unlock()
		return writer.buffer.Write(payload)
	}
	<-writer.gate
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.Write(payload)
}

func (writer *firstChunkGatedStdoutWriter) String() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.buffer.String()
}

func (writer *firstChunkGatedStdoutWriter) diagnosticText() string {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.diagnostic.String()
}

func (writer *firstChunkGatedStdoutWriter) release() {
	writer.releaseOnce.Do(func() {
		close(writer.gate)
	})
}

func waitForFirstChunkStdoutContent(t *testing.T, writer *firstChunkGatedStdoutWriter) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(writer.String()) != "" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for first human response-stream stdout chunk")
}

func containsHumanLifecycleLine(stdout string) bool {
	for _, line := range nonEmptyStdoutLines(stdout) {
		if isHumanFactoryLifecycleLine(line) {
			return true
		}
	}
	return false
}

func nonEmptyStdoutLines(value string) []string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func isHumanFactoryLifecycleLine(line string) bool {
	closingBracket := strings.Index(line, "] ")
	if !strings.HasPrefix(line, "[") || closingBracket < 2 {
		return false
	}
	message := line[closingBracket+2:]
	for _, prefix := range []string{
		"work accepted", "work moved", "factory started", "factory completed",
		"workstation queued", "workstation started", "workstation completed", "workstation failed", "workstation interrupted",
		"inference started", "inference completed", "inference failed", "workflow phase", "workflow checkpoint written",
		"final output updated",
	} {
		if strings.HasPrefix(message, prefix) {
			return true
		}
	}
	return false
}

func declaredRunCancelExitCode(t *testing.T, args []string) int {
	t.Helper()

	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		t.Fatalf("RunSubmitFamilyManifest() error = %v", err)
	}
	commandName := ""
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "-") {
			continue
		}
		commandName = arg
		break
	}
	if commandName != "run" {
		t.Fatalf("args %v do not select you run", args)
	}
	for _, command := range manifest.Commands {
		if command.Name != commandName {
			continue
		}
		for _, exit := range command.Exits {
			if exit.Kind == "cancel" {
				return exit.Code
			}
		}
	}
	t.Fatalf("command %q missing cancel exit in run/submit manifest", commandName)
	return -1
}

func runGoalHumanInterruptibleResponseStream(
	t *testing.T,
	externalWork *cancellableExternalWorkRunner,
	stdout *interruptibleStdoutCapture,
) []string {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, writerFailureGoalMockWorkers())
	args := []string{
		"you", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic human text-stream interrupt contract",
	}
	ctx, cancel := context.WithCancel(t.Context())
	stdout.cancel = cancel
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = &stdout.diagnostic
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: externalWork,
	})

	go func() {
		stdout.err = process.Execute(inputs.Input)
		close(stdout.done)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-stdout.done:
		case <-time.After(humanTextStreamScenarioTimeout):
			t.Errorf("timed out waiting for interrupt invocation cleanup")
		}
	})
	return args
}

type interruptibleStdoutCapture struct {
	cancel context.CancelFunc

	mu         sync.Mutex
	buffer     bytes.Buffer
	diagnostic bytes.Buffer

	done chan struct{}
	err  error
}

func (capture *interruptibleStdoutCapture) Write(payload []byte) (int, error) {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.buffer.Write(payload)
}

func (capture *interruptibleStdoutCapture) String() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.buffer.String()
}

func (capture *interruptibleStdoutCapture) diagnosticText() string {
	capture.mu.Lock()
	defer capture.mu.Unlock()
	return capture.diagnostic.String()
}

func waitForInterruptibleStdoutLifecycle(t *testing.T, capture *interruptibleStdoutCapture) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if containsHumanLifecycleLine(capture.String()) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for human lifecycle stdout before interrupt\nstdout:\n%s", capture.String())
}

func runGoalHumanResponseStreamWithStdout(t *testing.T, stdout *firstChunkGatedStdoutWriter) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, goalFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "goal-executor",
			WorkstationName: "execute-goal",
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	})
	args := []string{
		"you", "run", "--named", goalFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record", "--output", "response-stream",
		"deterministic human text-stream incremental contract",
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Stdout = stdout
	inputs.Input.Stderr = &stdout.diagnostic
	process := support.BuildProcess(t, serviceedges.Edges{})

	go func() {
		stdout.err = process.Execute(inputs.Input)
		close(stdout.done)
	}()

	t.Cleanup(func() {
		stdout.release()
		select {
		case <-stdout.done:
		case <-time.After(humanTextStreamScenarioTimeout):
			t.Errorf("timed out waiting for invocation cleanup")
		}
	})
}

func textStreamPromptRunFactoryConfig() map[string]any {
	return map[string]any{
		"name": "text-stream-prompt-run",
		"workTypes": []map[string]any{
			{
				"name":             textStreamPromptRunWorkType,
				"handlingBehavior": []string{"DEFAULT"},
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "mock-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process-prompt",
				"worker":    "mock-worker",
				"inputs":    []map[string]string{{"workType": textStreamPromptRunWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": textStreamPromptRunWorkType, "state": "complete"}},
				"onFailure": []map[string]string{{"workType": textStreamPromptRunWorkType, "state": "failed"}},
			},
		},
	}
}

func reserveTextStreamLoopbackURL(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release loopback port: %v", err)
	}
	return "http://127.0.0.1:" + strconv.Itoa(port)
}

func writeTextStreamDefaultMockWorkersConfig(t *testing.T) string {
	t.Helper()

	data, err := json.MarshalIndent(workers.NewEmptyMockWorkersConfig(), "", "  ")
	if err != nil {
		t.Fatalf("marshal default mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func waitForTextStreamServerReady(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, support.DefaultSessionWorkURL(baseURL, "/work"), nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("server not ready")
	}
	return nil
}
