package isolation_and_recovery_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformlocking "github.com/portpowered/infinite-you/pkg/platform/locking"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession
// exercises the assembled replay/resume path through the customer process. The
// in-flight artifact is intentionally unfinalized, while the finished artifact
// retains its terminal Work state.
func TestSeededReplayResumeMaterializesRecordedWorkOnceThroughAssembledSession(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)

	for _, test := range []struct {
		name     string
		finished bool
	}{
		{
			name: "in-flight tail",
		}, {
			name:     "finished recording",
			finished: true,
		}} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			artifactPayload := seededReplayResumeArtifactPayload(t, test.finished)
			factoryDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			artifactPath := filepath.Join(factoryDir, "seeded-replay-resume.json")
			if err := os.WriteFile(artifactPath, artifactPayload, 0o644); err != nil {
				t.Fatalf("write replay artifact: %v", err)
			}
			running := reusable.run(t, factoryDir, artifactPath)

			stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
			waitForSeededReplayRuntimeStart(t, stream)
			status := support.GetJSON[factoryapi.StatusResponse](
				t,
				strings.TrimSuffix(running.url, "/")+"/factory-sessions/"+running.sessionID+"/status",
			)

			if status.TotalTokens != 1 {
				t.Fatalf("replayed status totalTokens = %d, want one Work token", status.TotalTokens)
			}
			if test.finished {
				if status.Categories.Terminal != 1 {
					t.Fatalf("finished replay terminal count = %d, want 1", status.Categories.Terminal)
				}
			} else if status.Categories.Initial != 1 {
				t.Fatalf("in-flight replay initial count = %d, want 1", status.Categories.Initial)
			}

			listed := support.GetJSON[factoryapi.ListWorkResponse](
				t,
				support.SessionWorkURL(running.url, running.sessionID, "/work"),
			)
			if len(listed.Results) != 1 {
				t.Fatalf("replayed Work listing length = %d, want one Work: %#v", len(listed.Results), listed.Results)
			}
			wantLocation := "task:init"
			if test.finished {
				wantLocation = "task:complete"
			}
			if !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", wantLocation) {
				t.Fatalf("replayed Work is not at %s: %#v", wantLocation, listed.Results)
			}

			for _, event := range support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID) {
				if event.Type == factoryapi.FactoryEventTypeDispatchRequest {
					t.Fatalf("replayed Work unexpectedly produced a dispatch request: %#v", event)
				}
			}
			running.daemon.Stop(t)
		})
	}
	t.Run("successor history", func(t *testing.T) {
		testSeededReplayResumePreservesSuccessorHistory(t, reusable)
	})
	t.Run("F01 read failure safety", func(t *testing.T) {
		testRecordStartupSafetyReadFailurePreservesTargetAndCause(t, reusable)
	})
	t.Run("JSON direct restore", func(t *testing.T) {
		testRecordStartupSafetyDirectRestore(t, reusable)
	})
	t.Run("F02 host startup failure", func(t *testing.T) {
		testRecordStartupSafetyHostFailure(t, reusable)
	})
	t.Run("F05 fresh recording boundaries", func(t *testing.T) {
		testRecordStartupSafetyFreshTargets(t, reusable)
	})
}

func testRecordStartupSafetyFreshTargets(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	t.Run("no recording", func(t *testing.T) {
		t.Parallel()
		dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
		running := reusable.runForSession(t, dir, "", uuid.NewString(), "--no-record")
		assertFreshRecordingBoard(t, running)
		restartProbeShutdown(t, running.url, running.daemon)
		if _, err := os.Stat(filepath.Join(running.home, ".you-agent-factory", "recordings")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("--no-record created a recording destination: %v", err)
		}
	})
	for _, empty := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty=%t", empty), func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selected := filepath.Join(dir, "fresh.__factory_session_id__.json")
			path := strings.ReplaceAll(selected, "__factory_session_id__", sessionID)
			if empty {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			running := reusable.runForSession(t, dir, "", sessionID, "--record", selected)
			assertFreshRecordingBoard(t, running)
			restartProbeShutdown(t, running.url, running.daemon)
			payload := mustReadSeededReplayArtifact(t, path)
			if len(payload) == 0 {
				t.Fatal("successful shutdown left an empty recording")
			}
			replayed := reusable.runForSession(t, dir, path, sessionID, "--replay", path, "--no-record")
			assertFreshRecordingBoard(t, replayed)
			restartProbeShutdown(t, replayed.url, replayed.daemon)
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, path)) {
				t.Fatal("read-only replay changed the fresh recording")
			}
		})
	}
}

func assertFreshRecordingBoard(t *testing.T, running seededReplayResumeRun) {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
	waitForSeededReplayRuntimeStart(t, stream)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(running.url, running.sessionID, "/work"))
	if len(listed.Results) != 0 {
		t.Fatalf("fresh board contains unexpected Work: %#v", listed.Results)
	}
}

// An explicitly selected UUID does not make a retained JSON board a fresh
// recording. Restore its Work and prefix before allowing any output flush.
func testRecordStartupSafetyDirectRestore(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	sessionID := uuid.NewString()
	selectedPath := filepath.Join(dir, "current-board.__factory_session_id__.json")
	path := strings.ReplaceAll(selectedPath, "__factory_session_id__", sessionID)
	var artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, true), &artifact); err != nil {
		t.Fatal(err)
	}
	for index := range artifact.Events {
		artifact.Events[index].Context.SessionID = &sessionID
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	// The same board is reopened only after its preceding host has joined.
	for opening := 0; opening < 3; opening++ {
		running := reusable.runForSession(t, dir, path, sessionID, "--record", selectedPath)
		assertSeededSuccessorWorkAndHistory(t, running, true)
		running.daemon.Stop(t)
	}
}

// A successor must remain recoverable after the live ledger has been released.
// Each format owns its files, session, profile and host on one shared process.
func testSeededReplayResumePreservesSuccessorHistory(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	for _, format := range []string{"json", "jsonl"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			source := filepath.Join(dir, "source.json")
			successor := filepath.Join(dir, "successor."+format)
			payload := seededReplayResumeArtifactPayload(t, true)
			sessionID := uuid.NewString()
			var artifact factorydefinitions.ReplayArtifact
			if err := json.Unmarshal(payload, &artifact); err != nil {
				t.Fatal(err)
			}
			for index := range artifact.Events {
				artifact.Events[index].Context.SessionID = &sessionID
			}
			payload, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, payload, 0o644); err != nil {
				t.Fatal(err)
			}
			resumed := reusable.runForSession(t, dir, source, sessionID, "--resume", source, "--record", successor)
			assertSeededSuccessorWorkAndHistory(t, resumed, true)
			resumed.daemon.Stop(t)
			if !bytes.Equal(payload, mustReadSeededReplayArtifact(t, source)) {
				t.Fatal("resume changed its source recording")
			}
			replayed := reusable.runForSession(t, dir, successor, sessionID, "--replay", successor, "--no-record")
			assertSeededSuccessorWorkAndHistory(t, replayed, false)
			replayed.daemon.Stop(t)
		})
	}
}

func assertSeededSuccessorWorkAndHistory(t *testing.T, running seededReplayResumeRun, retainedHistory bool) {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(t, support.SessionEventsURL(running.url, running.sessionID))
	waitForSeededReplayRuntimeStart(t, stream)
	listed := support.GetJSON[factoryapi.ListWorkResponse](t,
		support.SessionWorkURL(running.url, running.sessionID, "/work"))
	if len(listed.Results) != 1 || !support.HasWorkAtCustomerState(listed, "work-seeded-replay-resume", "task:complete") {
		t.Fatalf("successor lost the recorded terminal Work: %#v", listed.Results)
	}
	if !retainedHistory {
		return
	}
	events := support.GetFactoryEventsForSessionAt(t, running.url, running.sessionID)
	wantPrefix := []string{"run-request", "work-request", "work-state-change", "run-response"}
	if len(events) < len(wantPrefix) {
		t.Fatalf("successor history has %d events, want retained prefix %v", len(events), wantPrefix)
	}
	for index, id := range wantPrefix {
		if events[index].Id != id {
			t.Fatalf("successor event %d = %q, want retained %q", index, events[index].Id, id)
		}
	}
}

type seededReplayResumeProcess struct {
	process support.Process

	mu                 sync.RWMutex
	serversByPort      map[int]*support.ProcessAPIServer
	payloadsByPath     map[string][]byte
	readErrorsByPath   map[string]error
	replacementsByPath map[string][]byte
	serverErrorsByPort map[int]error
	nextPort           atomic.Int32
}

func TestRecordStartupSafetyResumeSourceConflict(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)
	for _, name := range []string{"same path", "normalized path", "relative path", "hard link", "symbolic link"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			factoryDir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			source := filepath.Join(factoryDir, "board.json")
			original := seededReplayResumeArtifactPayload(t, true)
			if err := os.WriteFile(source, original, 0o600); err != nil {
				t.Fatal(err)
			}
			target := seededResumeSourceAlias(t, name, source)
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "run", "--session", uuid.NewString(), "--dir", factoryDir,
				"--continuously", "--with-server", "--quiet", "--resume", source, "--record", target,
			})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = factoryDir
			err := reusable.process.Execute(inputs.Input)
			var coded interface{ CLIErrorCode() string }
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_SOURCE_CONFLICT" {
				t.Fatalf("source reuse result = %v; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
				t.Fatalf("decode source-conflict ErrorResponse: %v; stderr=%s", decodeErr, inputs.Stderr())
			}
			if response.Code != "RECORDING_SOURCE_CONFLICT" || response.Family != factoryapi.ErrorFamilyBadRequest || !strings.Contains(response.Message, strconv.Quote(target)) {
				t.Fatalf("source-conflict response = %#v", response)
			}
			if !bytes.Equal(mustReadSeededReplayArtifact(t, source), original) {
				t.Fatal("refused successor changed resume source bytes")
			}
			if strings.Contains(inputs.Stdout(), "API server:") || strings.Contains(inputs.Stdout(), "Dashboard:") {
				t.Fatalf("refused alias announced readiness: %s", inputs.Stdout())
			}
		})
	}
}

func TestRecordStartupSafetyRefusesAliasedDestination(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)
	for _, kind := range []string{"hard link", "symbolic link"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			source := filepath.Join(dir, "board.json")
			before := seededReplayResumeArtifactPayload(t, true)
			if err := os.WriteFile(source, before, 0o600); err != nil {
				t.Fatal(err)
			}
			sessionID := uuid.NewString()
			selected := filepath.Join(dir, "alias.__factory_session_id__.json")
			target := strings.ReplaceAll(selected, "__factory_session_id__", sessionID)
			if kind == "hard link" {
				if err := os.Link(source, target); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(source, target); err != nil {
				t.Skipf("OS does not permit scenario symlink: %v", err)
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selected})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err := reusable.process.Execute(inputs.Input)
			var coded interface{ CLIErrorCode() string }
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
				t.Fatalf("aliased destination startup = %v; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "RECORDING_TARGET_CONFLICT" || !strings.Contains(response.Message, strconv.Quote(target)) {
				t.Fatalf("missing typed path diagnostic: %#v", response)
			}
			for _, path := range []string{source, target} {
				if !bytes.Equal(before, mustReadSeededReplayArtifact(t, path)) {
					t.Fatalf("refused alias changed %q", path)
				}
			}
		})
	}
}

func seededResumeSourceAlias(t *testing.T, kind, source string) string {
	t.Helper()
	directory := filepath.Dir(source)
	switch kind {
	case "normalized path":
		return directory + string(filepath.Separator) + "." + string(filepath.Separator) + "board.json"
	case "relative path":
		return "board.json"
	case "hard link", "symbolic link":
		target := filepath.Join(directory, "alias.json")
		if kind == "hard link" {
			if err := os.Link(source, target); err != nil {
				t.Fatal(err)
			}
		} else if err := os.Symlink(source, target); err != nil {
			t.Skipf("OS does not permit scenario symlink: %v", err)
		}
		return target
	default:
		return source
	}
}

type seededReplayResumeRun struct {
	url       string
	sessionID string
	home      string
	daemon    *support.ProcessCommand
}

func newSeededReplayResumeProcess(t *testing.T, claims ...recordings.RecordingTargetClaim) *seededReplayResumeProcess {
	t.Helper()
	reusable := &seededReplayResumeProcess{
		serversByPort:      make(map[int]*support.ProcessAPIServer),
		payloadsByPath:     make(map[string][]byte),
		readErrorsByPath:   make(map[string]error),
		replacementsByPath: make(map[string][]byte),
		serverErrorsByPort: make(map[int]error),
	}
	var claim recordings.RecordingTargetClaim
	if len(claims) == 1 {
		claim = claims[0]
	}
	process := support.BuildProcess(t, serviceedges.Edges{
		RecordingTargetClaim:                claim,
		APIServerStarter:                    reusable.startAPIServer,
		FactorySessionReplayRecordingReader: reusable.readReplayRecording,
		RecordingReadFile:                   reusable.readRecording,
		ProviderCommandRunner: testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout("unexpected replay dispatch COMPLETE"),
		}),
	})
	support.CleanupProcess(t, process)
	reusable.process = process
	return reusable
}

type recordingTargetRelease func() error

func (release recordingTargetRelease) Close() error { return release() }

func TestRecordStartupSafetyDestinationChangesDuringRestore(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t)
	for _, name := range []string{"first board", "second board", "unchanged metadata"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selected := filepath.Join(dir, "board.__factory_session_id__.json")
			target := strings.ReplaceAll(selected, "__factory_session_id__", sessionID)
			var artifact factorydefinitions.ReplayArtifact
			if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, true), &artifact); err != nil {
				t.Fatal(err)
			}
			for index := range artifact.Events {
				artifact.Events[index].Context.SessionID = &sessionID
			}
			original, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, original, 0o600); err != nil {
				t.Fatal(err)
			}
			replacement := []byte("externally replaced retained history")
			if name == "unchanged metadata" {
				replacement = bytes.Clone(original)
				replacement[len(replacement)-1] = ' '
			}
			reusable.mu.Lock()
			reusable.replacementsByPath[filepath.Clean(target)] = replacement
			reusable.mu.Unlock()
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selected})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err = reusable.process.Execute(inputs.Input)
			var coded interface{ CLIErrorCode() string }
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
				t.Fatalf("changed restore input = %v; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "RECORDING_TARGET_CONFLICT" || !strings.Contains(response.Message, strconv.Quote(target)) {
				t.Fatalf("missing typed path diagnostic: %#v", response)
			}
			if !bytes.Equal(replacement, mustReadSeededReplayArtifact(t, target)) || strings.Contains(inputs.Stdout(), "Factory initiated:") {
				t.Fatal("failed startup published readiness or changed replacement history")
			}
		})
	}
}

func TestRecordStartupSafetyDestinationReplacement(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	reusable := newSeededReplayResumeProcess(t, func(ctx context.Context, target, marker string) (io.Closer, error) {
		files := &startupReplacementFiles{target: target, marker: marker}
		coordination, err := platformlocking.New(files)
		if err != nil {
			return nil, err
		}
		return coordination.TryLockTarget(ctx, target, marker)
	})
	for _, name := range []string{"first board", "second board"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selected := filepath.Join(dir, "board.__factory_session_id__.json")
			target := strings.ReplaceAll(selected, "__factory_session_id__", sessionID)
			original := seededReplayResumeArtifactPayload(t, true)
			replacement := seededReplayResumeArtifactPayload(t, false)
			for path, payload := range map[string][]byte{target: original, target + ".replacement": replacement} {
				if err := os.WriteFile(path, payload, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selected})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err := reusable.process.Execute(inputs.Input)
			var coded interface{ CLIErrorCode() string }
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" {
				t.Fatalf("replaced destination startup = %v; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != "RECORDING_TARGET_CONFLICT" || !strings.Contains(response.Message, strconv.Quote(target)) {
				t.Fatalf("missing typed path diagnostic: %#v", response)
			}
			if !bytes.Equal(replacement, mustReadSeededReplayArtifact(t, target)) || strings.Contains(inputs.Stdout(), "Factory initiated:") {
				t.Fatal("failed startup published readiness or changed replacement history")
			}
		})
	}
}

type startupReplacementFiles struct {
	platformlocking.LocalFileSystem
	target, marker string
}

func (files *startupReplacementFiles) OpenFile(path string, flags int, mode fs.FileMode) (platformlocking.File, error) {
	if path == files.marker {
		if err := os.Rename(files.target+".replacement", files.target); err != nil {
			return nil, err
		}
	}
	return files.LocalFileSystem.OpenFile(path, flags, mode)
}

func TestRecordStartupSafetyDestinationOwnership(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)
	var mu sync.Mutex
	denied := make(map[string]bool)
	released := make(map[string]int)
	busy := errors.New("recording destination already has an owner")
	reusable := newSeededReplayResumeProcess(t, func(_ context.Context, _ string, marker string) (io.Closer, error) {
		mu.Lock()
		defer mu.Unlock()
		if denied[marker] {
			return nil, busy
		}
		return recordingTargetRelease(func() error {
			mu.Lock()
			defer mu.Unlock()
			released[marker]++
			return nil
		}), nil
	})
	for _, name := range []string{"first owned board", "second owned board"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selected := filepath.Join(dir, "board.__factory_session_id__.json")
			path := strings.ReplaceAll(selected, "__factory_session_id__", sessionID)
			var artifact factorydefinitions.ReplayArtifact
			if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, true), &artifact); err != nil {
				t.Fatal(err)
			}
			for index := range artifact.Events {
				artifact.Events[index].Context.SessionID = &sessionID
			}
			original, err := json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			marker := path + ".recording.lock"
			if runtime.GOOS == "windows" {
				marker = strings.ToLower(marker)
			}
			mu.Lock()
			denied[marker] = true
			mu.Unlock()
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--record", selected})
			profile := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
			inputs.Input.WorkingDirectory = dir
			err = reusable.process.Execute(inputs.Input)
			var coded interface{ CLIErrorCode() string }
			if !errors.As(err, &coded) || coded.CLIErrorCode() != "RECORDING_TARGET_CONFLICT" ||
				!strings.Contains(err.Error(), strconv.Quote(path)) || strings.Contains(inputs.Stdout(), "Factory initiated:") {
				t.Fatalf("occupied destination startup = %v; stdout=%s", err, inputs.Stdout())
			}
			if !bytes.Equal(original, mustReadSeededReplayArtifact(t, path)) {
				t.Fatal("failed ownership acquisition changed retained history")
			}
			mu.Lock()
			delete(denied, marker)
			mu.Unlock()
			running := reusable.runForSession(t, dir, path, sessionID, "--record", selected)
			assertSeededSuccessorWorkAndHistory(t, running, true)
			running.daemon.Stop(t)
			mu.Lock()
			count := released[marker]
			mu.Unlock()
			if count != 1 {
				t.Fatalf("successful shutdown released destination %d times; want one", count)
			}
		})
	}
}

func (reusable *seededReplayResumeProcess) run(
	t *testing.T,
	factoryDir string,
	artifactPath string,
) seededReplayResumeRun {
	return reusable.runWithRecordingArgs(t, factoryDir, artifactPath, "--replay", artifactPath, "--no-record")
}

func (reusable *seededReplayResumeProcess) runWithRecordingArgs(
	t *testing.T,
	factoryDir string,
	artifactPath string,
	recordingArgs ...string,
) seededReplayResumeRun {
	return reusable.runForSession(t, factoryDir, artifactPath, uuid.NewString(), recordingArgs...)
}

func (reusable *seededReplayResumeProcess) runForSession(
	t *testing.T,
	factoryDir string,
	artifactPath string,
	sessionID string,
	recordingArgs ...string,
) seededReplayResumeRun {
	t.Helper()
	api := support.NewProcessAPIServer()
	port := 22000 + int(reusable.nextPort.Add(1))
	reusable.mu.Lock()
	reusable.serversByPort[port] = api
	if artifactPath != "" {
		reusable.payloadsByPath[filepath.Clean(artifactPath)] = append([]byte(nil), mustReadSeededReplayArtifact(t, artifactPath)...)
	}
	reusable.mu.Unlock()
	t.Cleanup(func() {
		reusable.mu.Lock()
		delete(reusable.serversByPort, port)
		delete(reusable.payloadsByPath, filepath.Clean(artifactPath))
		reusable.mu.Unlock()
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--session", sessionID,
		"--continuously", "--with-server", "--quiet",
		"--listen", fmt.Sprintf("127.0.0.1:%d", port),
		"--dir", factoryDir,
		"--provider", "CODEX", "--model", "gpt-5-codex",
	})
	inputs.Input.Args = append(inputs.Input.Args, recordingArgs...)
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		if stderr := strings.TrimSpace(inputs.Stderr()); stderr != "" {
			t.Logf("seeded replay daemon stderr: %s", stderr)
		}
	})
	daemon := support.StartProcessCommand(t, reusable.process, inputs.Input)
	return seededReplayResumeRun{url: api.WaitForURL(t), sessionID: sessionID, home: home, daemon: daemon}
}

func mustReadSeededReplayArtifact(t testing.TB, path string) []byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replay artifact: %v", err)
	}
	return payload
}

func (reusable *seededReplayResumeProcess) readReplayRecording(path string) ([]byte, error) {
	reusable.mu.RLock()
	payload := reusable.payloadsByPath[filepath.Clean(path)]
	err := reusable.readErrorsByPath[filepath.Clean(path)]
	reusable.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("seeded replay payload was not registered for this invocation")
	}
	return append([]byte(nil), payload...), nil
}

// A restore read failure must abort before any writes to the resolved target,
// including writes from startup cleanup. Each session owns its fault and file.
func testRecordStartupSafetyReadFailurePreservesTargetAndCause(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	for _, test := range []struct {
		name, format, payload, code string
		readFailure                 bool
	}{
		{"JSON read denied", "json", "retained recording bytes", "CURRENT_BOARD_RECORDING_UNREADABLE", true},
		{"JSONL read denied", "jsonl", "retained recording bytes", "CURRENT_BOARD_RECORDING_UNREADABLE", true},
		{"corrupt JSON", "json", `{"schemaVersion":"replay.v1","events":["PRIVATE_RECORDING_PAYLOAD"`, "CURRENT_BOARD_RECORDING_CORRUPT", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
			sessionID := uuid.NewString()
			selectedPath := filepath.Join(dir, "retained.__factory_session_id__."+test.format)
			path := strings.ReplaceAll(selectedPath, "__factory_session_id__", sessionID)
			support.WriteAgentConfig(t, dir, "worker-a", "---\ntype: MODEL_WORKER\n---\n")
			support.WriteWorkstationConfig(t, dir, "process", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
			before := []byte(test.payload)
			if err := os.WriteFile(path, before, 0o600); err != nil {
				t.Fatal(err)
			}
			cause := &fs.PathError{Op: "read recording", Path: path, Err: fs.ErrPermission}
			if test.readFailure {
				reusable.mu.Lock()
				reusable.readErrorsByPath[path] = cause
				reusable.mu.Unlock()
			}
			t.Cleanup(func() {
				reusable.mu.Lock()
				delete(reusable.readErrorsByPath, path)
				reusable.mu.Unlock()
			})
			inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
				"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selectedPath,
				"--provider", "CODEX", "--model", "gpt-5-codex"})
			home := t.TempDir()
			inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
			inputs.Input.WorkingDirectory = dir
			err := reusable.process.Execute(inputs.Input)
			if test.readFailure && !errors.Is(err, cause) {
				t.Fatalf("startup error = %v, want original read cause; stderr=%s", err, inputs.Stderr())
			}
			var response factoryapi.ErrorResponse
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); decodeErr != nil {
				t.Fatalf("decode ErrorResponse: %v; stderr=%s", decodeErr, inputs.Stderr())
			}
			if string(response.Code) != test.code || response.Family != factoryapi.ErrorFamilyInternalServerError ||
				!strings.Contains(response.Message, fmt.Sprintf("%q", path)) {
				t.Fatalf("startup response omits selected path or file cause: %#v", response)
			}
			if test.readFailure && !strings.Contains(response.Message, "permission denied") {
				t.Fatalf("startup response omits read cause: %#v", response)
			}
			if strings.Contains(inputs.Stdout()+inputs.Stderr(), "PRIVATE_RECORDING_PAYLOAD") {
				t.Fatal("startup diagnostic exposed recording payload")
			}
			if strings.Contains(inputs.Stdout()+inputs.Stderr(), "Factory initiated:") {
				t.Fatal("failed startup published readiness")
			}
			if !bytes.Equal(before, mustReadSeededReplayArtifact(t, path)) {
				t.Fatal("failed startup cleanup changed the retained recording")
			}
		})
	}
}

func (reusable *seededReplayResumeProcess) readRecording(path string) ([]byte, error) {
	reusable.mu.Lock()
	err := reusable.readErrorsByPath[filepath.Clean(path)]
	replacement := reusable.replacementsByPath[filepath.Clean(path)]
	delete(reusable.replacementsByPath, filepath.Clean(path))
	reusable.mu.Unlock()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err == nil && replacement != nil {
		if err := replaceReadRecordingInput(path, data, replacement); err != nil {
			return nil, err
		}
	}
	return data, err
}

// The read returns a valid prefix while a path-scoped external actor edits
// its source. Equal-sized edits also retain inode identity and modification time.
func replaceReadRecordingInput(path string, original, replacement []byte) error {
	if len(original) == len(replacement) {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, replacement, 0o600); err != nil {
			return err
		}
		return os.Chtimes(path, info.ModTime(), info.ModTime())
	}
	staging := path + ".replacement"
	if err := os.WriteFile(staging, replacement, 0o600); err != nil {
		return err
	}
	return os.Rename(staging, path)
}

func (reusable *seededReplayResumeProcess) startAPIServer(
	ctx context.Context,
	request platformhttpserver.StartRequest,
) error {
	reusable.mu.RLock()
	server := reusable.serversByPort[request.Port]
	err := reusable.serverErrorsByPort[request.Port]
	reusable.mu.RUnlock()
	if err != nil {
		return err
	}
	if server == nil {
		return fmt.Errorf("seeded replay API server is not registered for requested port %d", request.Port)
	}
	return server.Start(ctx, request)
}

func testRecordStartupSafetyHostFailure(t *testing.T, reusable *seededReplayResumeProcess) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, seededReplayResumeFactoryConfig())
	sessionID := uuid.NewString()
	selectedPath := filepath.Join(dir, "protected.__factory_session_id__.json")
	path := strings.ReplaceAll(selectedPath, "__factory_session_id__", sessionID)
	var artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(seededReplayResumeArtifactPayload(t, true), &artifact); err != nil {
		t.Fatal(err)
	}
	for index := range artifact.Events {
		artifact.Events[index].Context.SessionID = &sessionID
	}
	before, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	port := 22000 + int(reusable.nextPort.Add(1))
	cause := &fs.PathError{Op: "open listener configuration", Path: filepath.Join(dir, "listener"), Err: fs.ErrPermission}
	reusable.mu.Lock()
	reusable.payloadsByPath[path] = before
	// The external listener can fail while its cleanup reports cancellation.
	// Both identities must survive without classifying the primary as a stop.
	reusable.serverErrorsByPort[port] = errors.Join(cause, context.Canceled)
	reusable.mu.Unlock()
	t.Cleanup(func() {
		reusable.mu.Lock()
		delete(reusable.payloadsByPath, path)
		delete(reusable.serverErrorsByPort, port)
		reusable.mu.Unlock()
	})
	logRoot := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--session", sessionID,
		"--dir", dir, "--continuously", "--with-server", "--quiet", "--record", selectedPath,
		"--runtime-log-dir", logRoot,
		"--listen", fmt.Sprintf("127.0.0.1:%d", port), "--provider", "CODEX", "--model", "gpt-5-codex"})
	profile := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+profile, "USERPROFILE="+profile)
	inputs.Input.WorkingDirectory = dir
	err = reusable.process.Execute(inputs.Input)
	if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("startup lost listener cause: %v; stderr=%s", err, inputs.Stderr())
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stderr())), &response); err != nil {
		t.Fatalf("decode startup ErrorResponse: %v; stderr=%s", err, inputs.Stderr())
	}
	if response.Code != "SERVER_START_FAILED" || !strings.Contains(response.Message, "permission denied") {
		t.Fatalf("startup cause missing: %+v", response)
	}
	if !bytes.Equal(before, mustReadSeededReplayArtifact(t, path)) {
		t.Fatal("failed host startup replaced validated retained history")
	}
	if strings.Contains(inputs.Stdout(), "Factory initiated:") {
		t.Fatal("failed host startup published readiness")
	}
	assertRecordStartupSafetyLog(t, logRoot)
	// Same-board retry is deliberately serial: the failed invocation must
	// release its prepared writer before this invocation opens that target.
	retry := support.FakeInputs(t.Context(), inputs.Input.Args)
	retry.Input.Env = inputs.Input.Env
	retry.Input.WorkingDirectory = dir
	if retryErr := reusable.process.Execute(retry.Input); !errors.Is(retryErr, cause) {
		t.Fatalf("retry after failed opening = %v; stderr=%s", retryErr, retry.Stderr())
	}
	if !bytes.Equal(before, mustReadSeededReplayArtifact(t, path)) {
		t.Fatal("retry cleanup changed retained history")
	}
}

func assertRecordStartupSafetyLog(t *testing.T, logRoot string) {
	t.Helper()
	var logs strings.Builder
	if err := filepath.WalkDir(logRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		payload, err := os.ReadFile(path)
		logs.Write(payload)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "permission denied") || !strings.Contains(logs.String(), "open listener configuration") {
		t.Fatalf("runtime log lost startup cause: %s", logs.String())
	}
}

func waitForSeededReplayRuntimeStart(t *testing.T, stream *support.FactoryEventStream) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if event.Type == factoryapi.FactoryEventTypeFactoryStateResponse {
			return
		}
	}
}

func seededReplayResumeFactoryConfig() map[string]any {
	return map[string]any{
		"name": "seeded-replay-resume",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func seededReplayResumeArtifactPayload(t *testing.T, finished bool) []byte {
	t.Helper()
	const (
		workID    = "work-seeded-replay-resume"
		requestID = "request-seeded-replay-resume"
		traceID   = "trace-seeded-replay-resume"
	)
	snapshot, err := factorydefinitions.NewFactorySnapshot(seededReplayResumeFactoryConfig())
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	base := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	requestIDValue := requestID
	workIDValue := []string{workID}
	traceIDValue := []string{traceID}
	requestSourceValue := "external-submit"
	sourceValue := requestSourceValue
	events := []factorydefinitions.FactoryEvent{
		seededReplayResumeEvent(t, "run-request", 0, 0, base, factorydefinitions.FactoryEventTypeRunRequest, factorydefinitions.RunRequestEventPayload{
			Factory:    snapshot,
			RecordedAt: base,
		}),
		seededReplayResumeEventWithContext(t, "work-request", 1, 1, base.Add(time.Second), factorydefinitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Source: requestSourceValue,
			Type:   work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{
				Name:       "recorded-work",
				WorkID:     workID,
				RequestID:  requestID,
				WorkTypeID: "task",
				State:      &work.WorkEventState{Name: "init", Type: "INITIAL"},
				TraceID:    traceID,
			}},
		}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
	}
	if finished {
		finishedSource := work.WorkStateChangeSourceAPI
		events = append(events,
			seededReplayResumeEventWithContext(t, "work-state-change", 2, 2, base.Add(2*time.Second), factorydefinitions.FactoryEventTypeWorkStateChange, factorydefinitions.WorkStateChangeEventPayload{
				FromPlaceID:  "task:init",
				FromState:    "init",
				Source:       finishedSource,
				ToPlaceID:    "task:complete",
				ToState:      "complete",
				WorkID:       workID,
				WorkTypeName: "task",
			}, &sourceValue, &requestIDValue, &workIDValue, &traceIDValue),
			seededReplayResumeEvent(t, "run-response", 3, 3, base.Add(3*time.Second), factorydefinitions.FactoryEventTypeRunResponse, func() factorydefinitions.RunResponseEventPayload {
				state := factorydefinitions.FactoryStateCompleted
				return factorydefinitions.RunResponseEventPayload{State: &state}
			}()),
		)
	}

	artifact := factorydefinitions.ReplayArtifact{
		SchemaVersion: factorydefinitions.ReplayV1SourceFormat,
		RecordedAt:    base,
		Events:        events,
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal seeded replay artifact: %v", err)
	}
	return payload
}

func seededReplayResumeEvent(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
) factorydefinitions.FactoryEvent {
	return seededReplayResumeEventWithContext(t, id, sequence, tick, eventTime, eventType, payload, nil, nil, nil, nil)
}

func seededReplayResumeEventWithContext(
	t *testing.T,
	id string,
	sequence int,
	tick int,
	eventTime time.Time,
	eventType factorydefinitions.FactoryEventType,
	payload any,
	source *string,
	contextRequestID *string,
	workIDs *[]string,
	traceIDs *[]string,
) factorydefinitions.FactoryEvent {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal replay event %q payload: %v", id, err)
	}
	return factorydefinitions.FactoryEvent{
		Id:            id,
		Payload:       payloadBytes,
		SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
		Type:          eventType,
		Context: factorydefinitions.FactoryEventContext{
			EventTime: eventTime,
			RequestID: contextRequestID,
			Sequence:  sequence,
			Source:    source,
			Tick:      tick,
			TraceIDs:  traceIDs,
			WorkIDs:   workIDs,
		},
	}
}
