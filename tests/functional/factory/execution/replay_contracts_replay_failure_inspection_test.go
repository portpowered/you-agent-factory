package execution_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	// replayRecordingFixture is a trimmed slice of a real operator recording:
	// the opening events, a few dispatches, and the session-complete tail. It is
	// deliberately small; replay behavior does not depend on ledger length.
	replayRecordingFixture = "tests/functional/factory/replay_contracts/testdata/recording/slice.jsonl"
	// replayFactoryFixture is the "current" Factory the recording is replayed
	// against. It differs from the recorded Factory, which drives the drift warning.
	replayFactoryFixture = "tests/functional/factory/replay_contracts/testdata/recording/factory"

	replayFixtureEvents    = 118
	replayFixtureWork      = 10
	replayFixtureLineage   = 10
	replayFixtureRelations = 4
	replayFixtureFailures  = 1
)

// newReplayProcess builds a root process whose every execution and
// recording-write edge fails closed, and registers the audit that proves a
// replay never reached one of them. Each subtest owns its own process and
// audit, so the subtests can run in parallel.
func newReplayProcess(t *testing.T) support.Process {
	t.Helper()
	effects := &replayEffects{}
	process := support.BuildProcess(t, effects.edges())
	t.Cleanup(func() {
		if got := effects.forbiddenCalls(); got != 0 {
			t.Errorf("forbidden provider/model/process/recording effects during replay = %d, want 0", got)
		}
	})
	return process
}

// TestReplayInspectsRecordedRunsReadOnly covers what a user sees when running
// "you run --replay" against a recording: a deterministic, read-only
// inspection report, a drift warning when the Factory changed, no fabricated
// facts for recordings that lack them, and a typed refusal for a corrupt
// recording.
func TestReplayInspectsRecordedRunsReadOnly(t *testing.T) {
	t.Parallel()
	t.Run("recorded-run-reports-history-deterministically", func(t *testing.T) {
		t.Parallel()
		process := newReplayProcess(t)
		recording := mustReadFile(t, testutil.MustRepoPath(t, replayRecordingFixture))
		replayPath := filepath.Join(t.TempDir(), "recording.jsonl")
		if err := os.WriteFile(replayPath, recording, 0o600); err != nil {
			t.Fatalf("write isolated replay copy: %v", err)
		}
		factoryDir := copyDirectory(t, testutil.MustRepoPath(t, replayFactoryFixture))
		factoryBefore := directoryDigest(t, factoryDir)
		env := isolatedReplayEnvironmentFor(t)

		first := executeReplay(t, process, factoryDir, replayPath, env)
		second := executeReplay(t, process, factoryDir, replayPath, env)
		for name, run := range map[string]replayRun{"first": first, "second": second} {
			if run.err != nil {
				t.Fatalf("%s replay: %v\nerror-chain=%s\nstdout=%s\nstderr=%s", name, run.err, replayErrorChain(run.err), run.stdout, run.stderr)
			}
		}
		if first.stdout != second.stdout {
			t.Fatal("replay output is not deterministic across repeated read-only invocations")
		}
		assertRecordedRunReport(t, recording, first.stdout)
		if got := sha256Hex(mustReadFile(t, replayPath)); got != sha256Hex(recording) {
			t.Fatalf("recording changed during replay: got %s", got)
		}
		if got := directoryDigest(t, factoryDir); !equalStringMap(got, factoryBefore) {
			t.Fatalf("Factory changed during historical replay: before=%v after=%v", factoryBefore, got)
		}
	})
	t.Run("recording-without-factory-work-facts-reports-unavailable", func(t *testing.T) {
		t.Parallel()
		exercisePortableReplay(t, newReplayProcess(t), testutil.MustRepoPath(t, replayFactoryFixture))
	})
	t.Run("first-corrupt-event-is-typed-and-atomic", func(t *testing.T) {
		t.Parallel()
		exerciseCorruptReplay(t, newReplayProcess(t), mustReadFile(t, testutil.MustRepoPath(t, replayRecordingFixture)), testutil.MustRepoPath(t, replayFactoryFixture))
	})
}

func replayErrorChain(err error) string {
	var parts []string
	for current := err; current != nil; current = errors.Unwrap(current) {
		parts = append(parts, fmt.Sprintf("%T: %v", current, current))
	}
	return strings.Join(parts, " <- ")
}

type recordedEvent struct{ ID, Type string }

func recordedEvents(t *testing.T, data []byte) []recordedEvent {
	t.Helper()
	var events []recordedEvent
	for lineIndex, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record struct {
			RecordType string `json:"recordType"`
			Event      struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"event"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode recording line %d: %v", lineIndex, err)
		}
		if record.RecordType == "event" {
			events = append(events, recordedEvent{ID: record.Event.ID, Type: record.Event.Type})
		}
	}
	return events
}

func assertRecordedRunReport(t *testing.T, recording []byte, output string) {
	t.Helper()
	for _, want := range []string{
		"Status: SUCCEEDED",
		"Result: FINAL",
		"Worker history: UNAVAILABLE (reason=SCHEMA_DID_NOT_RECORD_CANONICAL_WORKER_HISTORY)",
		"Factory projection: AVAILABLE (reason=)",
		"Replay warning: current Factory Definition differs from the recording; affected components:",
		"Session lifecycle: control=\"PAUSED\" terminal=true final=\"SUCCEEDED\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("replay output missing %q:\n%s", want, output)
		}
	}
	events := recordedEvents(t, recording)
	if len(events) != replayFixtureEvents {
		t.Fatalf("recording fixture has %d events, want %d", len(events), replayFixtureEvents)
	}
	for prefix, want := range map[string]int{
		"Work: ":     replayFixtureWork,
		"Lineage: ":  replayFixtureLineage,
		"Relation: ": replayFixtureRelations,
		"Failure: ":  replayFixtureFailures,
		"Event ":     replayFixtureEvents,
	} {
		if got := countOutputLines(output, prefix); got != want {
			t.Fatalf("output lines with prefix %q = %d, want %d", prefix, got, want)
		}
	}
	for _, prefix := range []string{"Work: ", "Lineage: ", "Relation: ", "Failure: "} {
		var rows []string
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, prefix) {
				rows = append(rows, line)
			}
		}
		if !sort.StringsAreSorted(rows) {
			t.Fatalf("%s rows are not sorted", prefix)
		}
	}
	lastIndex := -1
	for index, event := range events {
		want := fmt.Sprintf("Event %d: %s (%s)", index, event.Type, event.ID)
		position := strings.Index(output, want)
		if position < 0 {
			t.Fatalf("replay output missing ordered event identity %q", want)
		}
		if position <= lastIndex {
			t.Fatalf("replay event identity %q is out of source order", want)
		}
		lastIndex = position
	}
}

func exercisePortableReplay(t *testing.T, process support.Process, factorySource string) {
	t.Helper()
	portableSource := testutil.MustRepoPath(t, "pkg/services/recordings/internal/artifacts/testdata/valid-v3-worker-history.json")
	portableData := mustReadFile(t, portableSource)
	factoryDir := copyDirectory(t, factorySource)
	replayPath := filepath.Join(t.TempDir(), "portable.json")
	if err := os.WriteFile(replayPath, portableData, 0o600); err != nil {
		t.Fatalf("write isolated portable replay copy: %v", err)
	}
	run := executeReplay(t, process, factoryDir, replayPath, isolatedReplayEnvironmentFor(t))
	if run.err != nil {
		t.Fatalf("portable replay: %v\nstdout=%s\nstderr=%s", run.err, run.stdout, run.stderr)
	}
	for _, want := range []string{
		"Factory projection: UNAVAILABLE (reason=RECORDING_DID_NOT_CAPTURE_CANONICAL_FACTORY_WORK_FACTS)",
		"Worker history: AVAILABLE (reason=)",
		"Events: 2",
	} {
		if !strings.Contains(run.stdout, want) {
			t.Fatalf("portable replay output missing %q", want)
		}
	}
	for _, prefix := range []string{"Work: ", "Lineage: ", "Relation: ", "Failure: "} {
		if got := countOutputLines(run.stdout, prefix); got != 0 {
			t.Fatalf("portable replay fabricated %d %s rows", got, prefix)
		}
	}
	if got := sha256Hex(mustReadFile(t, replayPath)); got != sha256Hex(portableData) {
		t.Fatalf("portable replay copy changed: got %s want %s", got, sha256Hex(portableData))
	}
}

func exerciseCorruptReplay(t *testing.T, process support.Process, recording []byte, factorySource string) {
	t.Helper()
	corruptData := duplicateEventID(t, recording, 3, 2)
	replayPath := filepath.Join(t.TempDir(), "corrupt.jsonl")
	if err := os.WriteFile(replayPath, corruptData, 0o600); err != nil {
		t.Fatalf("write corrupt replay copy: %v", err)
	}
	factoryDir := copyDirectory(t, factorySource)
	run := executeReplay(t, process, factoryDir, replayPath, isolatedReplayEnvironmentFor(t))
	if run.err == nil {
		t.Fatalf("corrupt replay succeeded; stdout=%s", run.stdout)
	}
	diagnostic := run.err.Error() + "\n" + run.stderr
	for _, want := range []string{"INVALID_RECORDING_IDENTITY", "events[3]", "REPLACE_OR_REGENERATE_RECORDING"} {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("corrupt replay diagnostic missing %q: %s", want, diagnostic)
		}
	}
	for _, prefix := range []string{"Replayed Factory Session:", "Work: ", "Lineage: ", "Relation: ", "Failure: ", "Event "} {
		if strings.Contains(run.stdout, prefix) {
			t.Fatalf("corrupt replay emitted %s before rejecting the first corrupt event: %s", prefix, run.stdout)
		}
	}
	if got := sha256Hex(mustReadFile(t, replayPath)); got != sha256Hex(corruptData) {
		t.Fatalf("corrupt replay copy changed after rejection: got %s want %s", got, sha256Hex(corruptData))
	}
}

type replayRun struct {
	err    error
	stdout string
	stderr string
}

func executeReplay(t *testing.T, process support.Process, factoryDir, replayPath string, env []string) replayRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), support.ScaledTimeout(90*time.Second))
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{"you", "run", "--dir", factoryDir, "--replay", replayPath, "--no-record"})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = factoryDir
	err := process.Execute(inputs.Input)
	return replayRun{err: err, stdout: inputs.Stdout(), stderr: inputs.Stderr()}
}

func isolatedReplayEnvironmentFor(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	cache := filepath.Join(home, "cache")
	config := filepath.Join(home, "config")
	appData := filepath.Join(home, "appdata")
	localAppData := filepath.Join(home, "localappdata")
	values := map[string]string{
		"HOME":            home,
		"USERPROFILE":     home,
		"XDG_CACHE_HOME":  cache,
		"XDG_CONFIG_HOME": config,
		"APPDATA":         appData,
		"LOCALAPPDATA":    localAppData,
		"FACTORY_HOME":    home,
		"YOU_HOME":        home,
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var env []string
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replace := values[key]; replace {
				continue
			}
		}
		env = append(env, item)
	}
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func duplicateEventID(t *testing.T, data []byte, target, source int) []byte {
	t.Helper()
	lines := bytes.Split(data, []byte("\n"))
	eventIndex := -1
	var sourceID string
	for lineIndex, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var record struct {
			RecordType string `json:"recordType"`
			Event      struct {
				ID string `json:"id"`
			} `json:"event"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("decode line %d while corrupting replay: %v", lineIndex, err)
		}
		if record.RecordType != "event" {
			continue
		}
		eventIndex++
		if eventIndex == source {
			sourceID = record.Event.ID
		}
		if eventIndex != target {
			continue
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(line, &envelope); err != nil {
			t.Fatalf("decode target envelope %d while corrupting replay: %v", target, err)
		}
		var event map[string]json.RawMessage
		if err := json.Unmarshal(envelope["event"], &event); err != nil {
			t.Fatalf("decode target event %d while corrupting replay: %v", target, err)
		}
		id, err := json.Marshal(sourceID)
		if err != nil {
			t.Fatalf("marshal duplicate event ID: %v", err)
		}
		event["id"] = id
		envelope["event"], err = json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal corrupt target event %d: %v", target, err)
		}
		lines[lineIndex], err = json.Marshal(envelope)
		if err != nil {
			t.Fatalf("marshal corrupt target envelope %d: %v", target, err)
		}
	}
	if sourceID == "" {
		t.Fatalf("source event %d was not found while corrupting replay", source)
	}
	if eventIndex < target {
		t.Fatalf("target event %d was not found while corrupting replay", target)
	}
	return bytes.Join(lines, []byte("\n"))
}

func copyDirectory(t *testing.T, source string) string {
	t.Helper()
	target := filepath.Join(t.TempDir(), "factory")
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0o755)
		}
		targetPath := filepath.Join(target, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copy Factory fixture %q: %v", source, err)
	}
	return target
}

func directoryDigest(t *testing.T, root string) map[string]string {
	t.Helper()
	digest := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest[relative] = sha256Hex(mustReadFile(t, path))
		return nil
	})
	if err != nil {
		t.Fatalf("digest Factory directory %q: %v", root, err)
	}
	return digest
}

func equalStringMap(left, right map[string]string) bool {
	return len(left) == len(right) && func() bool {
		for key, value := range left {
			if right[key] != value {
				return false
			}
		}
		return true
	}()
}

func countOutputLines(output, prefix string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			count++
		}
	}
	return count
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return data
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

type replayEffects struct {
	providerCalls  atomic.Int64
	scriptCalls    atomic.Int64
	modelCalls     atomic.Int64
	grpcCalls      atomic.Int64
	protocolCalls  atomic.Int64
	writeCalls     atomic.Int64
	appendCalls    atomic.Int64
	directoryCalls atomic.Int64
	temporaryCalls atomic.Int64
	removeCalls    atomic.Int64
	renameCalls    atomic.Int64
	reads          atomic.Int64
	opens          atomic.Int64
}

func (effects *replayEffects) forbiddenCalls() int64 {
	return effects.providerCalls.Load() +
		effects.scriptCalls.Load() +
		effects.modelCalls.Load() +
		effects.grpcCalls.Load() +
		effects.protocolCalls.Load() +
		effects.writeCalls.Load() +
		effects.appendCalls.Load() +
		effects.directoryCalls.Load() +
		effects.temporaryCalls.Load() +
		effects.removeCalls.Load() +
		effects.renameCalls.Load()
}

func (effects *replayEffects) edges() serviceedges.Edges {
	read := func(path string) ([]byte, error) {
		effects.reads.Add(1)
		return os.ReadFile(path)
	}
	open := func(path string) (io.ReadCloser, error) {
		effects.opens.Add(1)
		return os.Open(path)
	}
	failWrite := func(label string, counter *atomic.Int64) func(string, []byte) error {
		return func(string, []byte) error {
			counter.Add(1)
			return fmt.Errorf("historical replay called forbidden %s", label)
		}
	}
	failPath := func(label string, counter *atomic.Int64) func(string) error {
		return func(string) error {
			counter.Add(1)
			return fmt.Errorf("historical replay called forbidden %s", label)
		}
	}
	return serviceedges.Edges{
		ProviderCommandRunner: replayForbiddenCommandRunner{label: "provider", calls: &effects.providerCalls},
		ScriptCommandRunner:   replayForbiddenCommandRunner{label: "script", calls: &effects.scriptCalls},
		ModelRuntimeCommandRunner: replayForbiddenCommandRunner{
			label: "model runtime", calls: &effects.modelCalls,
		},
		ModelInvocationBackend: func(context.Context, models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
			effects.modelCalls.Add(1)
			return nil, nil, errors.New("historical replay called forbidden model backend")
		},
		ModelInvocationProtocolClient: replayForbiddenProtocolClient{calls: &effects.protocolCalls},
		ModelInvocationGRPCDialer:     replayForbiddenGRPCDialer{calls: &effects.grpcCalls},
		RecordingWriteFile:            failWrite("RecordingWriteFile", &effects.writeCalls),
		RecordingAppendFile:           failWrite("RecordingAppendFile", &effects.appendCalls),
		RecordingMakeDirectories: func(string, fs.FileMode) error {
			effects.directoryCalls.Add(1)
			return errors.New("historical replay called forbidden RecordingMakeDirectories")
		},
		RecordingCreateTempFile: func(string, string) (recordings.RecordingTemporaryFile, error) {
			effects.temporaryCalls.Add(1)
			return nil, errors.New("historical replay called forbidden RecordingCreateTempFile")
		},
		RecordingRemovePath: failPath("RecordingRemovePath", &effects.removeCalls),
		RecordingRenamePath: func(string, string) error {
			effects.renameCalls.Add(1)
			return errors.New("historical replay called forbidden RecordingRenamePath")
		},
		RecordingReadFile: read,
		RecordingOpenFile: open,
		FactorySessionReplayRecordingReader: func(path string) ([]byte, error) {
			effects.reads.Add(1)
			return os.ReadFile(path)
		},
	}
}

type replayForbiddenCommandRunner struct {
	label string
	calls *atomic.Int64
}

func (runner replayForbiddenCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.calls.Add(1)
	return platformprocess.CommandResult{}, fmt.Errorf("historical replay called forbidden %s command runner", runner.label)
}

type replayForbiddenGRPCDialer struct{ calls *atomic.Int64 }

func (dialer replayForbiddenGRPCDialer) Dial(context.Context, string) (platformgrpc.Connection, error) {
	dialer.calls.Add(1)
	return nil, errors.New("historical replay called forbidden model gRPC dialer")
}

type replayForbiddenProtocolClient struct{ calls *atomic.Int64 }

func (client replayForbiddenProtocolClient) Predict(context.Context, models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	client.calls.Add(1)
	return models.InvocationProtocolResponse{}, errors.New("historical replay called forbidden model protocol client")
}
