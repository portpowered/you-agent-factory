package execution_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var fscp01ReplayTransientFields = map[string]string{
	"confirmationState": "read-boundary confirmation is a process-local durability watermark, not a dispatch fact",
}

// TestFSCP01DispatchReplayParityAndSourceWitness closes the dispatch gap with
// a two-process root-built record/replay handoff. The live Recordings ledger
// supplies the executing association fact (including its private model
// metadata), while the public dispatch list/detail and canonical event reads
// are compared before and after the handoff. Fields that the current
// JavaScript fixture does not emit as canonical dispatch events remain
// INCONCLUSIVE with their stable blockers in the matrix.
func TestFSCP01DispatchReplayParityAndSourceWitness(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)
	dir := support.ScaffoldFactory(t, map[string]any{"name": "fscp01-dispatch-replay"})
	recordPath := filepath.Join(t.TempDir(), "fscp01-dispatch-replay.json")
	logFSCP01RecordingDeclaration(t, dir, recordPath)

	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("fscp01 replay provider output"),
	})
	liveWitness := &fscp01DispatchRecordingWitness{}
	first := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", recordPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner:  runner,
			RecordingsRootObserver: liveWitness.observeRoot,
			DispatchRecorder:       liveWitness.observeRuntimeDispatch,
		},
	})
	firstClosed := false
	t.Cleanup(func() {
		if !firstClosed {
			first.Stop(t)
			first.Close(t)
		}
	})

	started := startFSCP01DispatchWorkflowSync(t, first.URL(), fscp01LiveDispatchCorrelationWorkflow)
	if started.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("recorded session status = %q, want SUCCEEDED", started.Status)
	}
	if strings.TrimSpace(started.SessionId) == "" {
		t.Fatal("recorded session id is empty")
	}
	listed := listFactorySessionDispatches(t, first.URL(), started.SessionId)
	summary := requireFSCP01DispatchSummaryByLabel(t, listed, dispatchCorrelationChildLabel)
	detail := getFactorySessionDispatch(t, first.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchListDetail(t, started.SessionId, summary, detail)
	canonicalBefore := support.GetFactoryEventsForSessionAt(t, first.URL(), started.SessionId)
	if len(canonicalBefore) == 0 {
		t.Fatal("recorded session returned no canonical events")
	}
	publicFacts := observeFSCP01CanonicalDispatch(t, first.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchAttemptAndWorkerIdentity(t, detail, publicFacts)
	sourceFacts := observeFSCP01RecordingDispatch(t, liveWitness, started.SessionId, summary.Id, detail)
	recordFSCP01DispatchFieldSources(t, "terminal", summary, detail, sourceFacts)

	// Closing the first root is the recording finalization boundary. The replay
	// root is not started until this process has been stopped and joined.
	first.Stop(t)
	first.Close(t)
	firstClosed = true
	artifact := testutil.LoadReplayArtifact(t, recordPath)
	assertFSCP01ReplayArtifactAssociation(t, artifact.Events, sourceFacts)

	second := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--replay", recordPath, "--no-record"},
	})
	t.Cleanup(func() {
		second.Stop(t)
		second.Close(t)
	})

	replayed := listFactorySessionDispatches(t, second.URL(), started.SessionId)
	replayedSummary := requireFSCP01DispatchSummary(t, replayed, summary.Id)
	replayedDetail := getFactorySessionDispatch(t, second.URL(), started.SessionId, summary.Id)
	assertFSCP01DispatchListDetail(t, started.SessionId, replayedSummary, replayedDetail)
	assertFSCP01ReplayFieldParity(t, "summary", summary, replayedSummary)
	assertFSCP01ReplayFieldParity(t, "detail", detail, replayedDetail)

	canonicalAfter := support.GetFactoryEventsForSessionAt(t, second.URL(), started.SessionId)
	assertFSCP01CanonicalReplayParity(t, canonicalBefore, canonicalAfter)
	t.Logf("FSCP-01 replay evidence: session=%s dispatch=%s canonicalEvents=%d artifactEvents=%d workerSession=%s status=%s", started.SessionId, summary.Id, len(canonicalBefore), len(artifact.Events), sourceFacts.WorkerSessionID, detail.Status)
}

func logFSCP01RecordingDeclaration(t *testing.T, factoryDir, recordPath string) {
	t.Helper()
	commit := fscp01CurrentCommit()
	t.Logf("FSCP-01 declaration: platform=%s commit=%s sourcePlanSHA256=%s isolatedFactoryDir=%s isolatedRecordingPath=%s timeout=15m firstProcess=stop-and-close-before-replay secondProcess=replay-only network=none retries=0 providerCallBudget=1", runtime.GOOS, commit, fscp01SourcePlanSHA256, factoryDir, recordPath)
}

func fscp01CurrentCommit() string {
	for _, key := range []string{"UNIT_TIMING_COMMIT", "GITHUB_SHA"} {
		if commit := strings.TrimSpace(os.Getenv(key)); commit != "" {
			return commit
		}
	}
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range buildInfo.Settings {
			if setting.Key == "vcs.revision" {
				if commit := strings.TrimSpace(setting.Value); commit != "" {
					return commit
				}
			}
		}
	}
	if workingDir, err := os.Getwd(); err == nil {
		if gitDir := fscp01FindGitDir(workingDir); gitDir != "" {
			if head, err := os.ReadFile(filepath.Join(gitDir, "HEAD")); err == nil {
				ref := strings.TrimSpace(string(head))
				if strings.HasPrefix(ref, "ref: ") {
					refName := strings.TrimSpace(strings.TrimPrefix(ref, "ref: "))
					if commit := fscp01ReadGitRef(gitDir, refName); commit != "" {
						return commit
					}
				} else if ref != "" {
					return ref
				}
			}
		}
	}
	return "UNAVAILABLE"
}

func fscp01FindGitDir(start string) string {
	for current := filepath.Clean(start); ; current = filepath.Dir(current) {
		gitPath := filepath.Join(current, ".git")
		info, err := os.Stat(gitPath)
		if err == nil {
			if info.IsDir() {
				return gitPath
			}
			data, readErr := os.ReadFile(gitPath)
			line := strings.TrimSpace(string(data))
			if strings.HasPrefix(line, "gitdir: ") {
				resolved := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
				if !filepath.IsAbs(resolved) {
					resolved = filepath.Join(current, resolved)
				}
				return filepath.Clean(resolved)
			}
			if readErr != nil {
				return ""
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
	}
}

func fscp01ReadGitRef(gitDir, refName string) string {
	refPath := filepath.Join(gitDir, filepath.FromSlash(refName))
	if resolved, err := os.ReadFile(refPath); err == nil {
		return strings.TrimSpace(string(resolved))
	}
	commonDirData, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(commonDirData))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	resolved, err := os.ReadFile(filepath.Join(commonDir, filepath.FromSlash(refName)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(resolved))
}

func assertFSCP01ReplayArtifactAssociation(
	t *testing.T,
	events []recordings.FactoryEvent,
	want fscp01RecordingDispatchFacts,
) {
	t.Helper()
	associationCount := 0
	terminalCount := 0
	for _, event := range events {
		if event.Type == recordings.FactoryEventTypeSessionCompleted {
			terminalCount++
		}
		if event.Type != recordings.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil ||
			!fscp01CanonicalDispatchIDMatches(*event.Context.DispatchID, want.SessionID, want.DispatchID) {
			continue
		}
		associationCount++
		if event.Context.SessionID == nil ||
			(*event.Context.SessionID != want.SessionID && *event.Context.SessionID != "~default") {
			t.Fatalf("replay artifact association sessionId = %#v, want %q or ~default", event.Context.SessionID, want.SessionID)
		}
		var privatePayload struct {
			Model           string `json:"model"`
			WorkerSessionID string `json:"workerSessionId"`
		}
		if err := json.Unmarshal(event.Payload, &privatePayload); err != nil {
			t.Fatalf("decode replay artifact private association %q: %v", event.Id, err)
		}
		if privatePayload.WorkerSessionID != want.WorkerSessionID {
			t.Fatalf("replay artifact Worker Session identity = %q, want %q", privatePayload.WorkerSessionID, want.WorkerSessionID)
		}
		if privatePayload.Model != want.Model {
			t.Fatalf("replay artifact private model = %q, want %q", privatePayload.Model, want.Model)
		}
	}
	if associationCount != 1 {
		t.Fatalf("replay artifact association count = %d, want exactly one", associationCount)
	}
	if terminalCount != 1 {
		t.Fatalf("replay artifact SESSION_COMPLETED count = %d, want exactly one", terminalCount)
	}
}

func assertFSCP01ReplayFieldParity(
	t *testing.T,
	shape string,
	before, after any,
) {
	t.Helper()
	beforeFields := marshalFSCP01JSONFields(t, before)
	afterFields := marshalFSCP01JSONFields(t, after)
	if len(beforeFields) == 0 || len(afterFields) == 0 {
		t.Fatalf("replay %s field maps = %d/%d, want non-empty", shape, len(beforeFields), len(afterFields))
	}
	for field, beforeValue := range beforeFields {
		if reason, transient := fscp01ReplayTransientFields[field]; transient {
			t.Logf("FSCP-01 replay parity shape=%s field=%s disposition=INCONCLUSIVE exclusion=%s before=%s after=%s", shape, field, reason, beforeValue, afterFields[field])
			continue
		}
		afterValue, ok := afterFields[field]
		if !ok {
			t.Fatalf("replay %s canonical field %q missing after handoff", shape, field)
		}
		if !bytes.Equal(beforeValue, afterValue) {
			t.Fatalf("replay %s canonical field %q changed: before=%s after=%s", shape, field, beforeValue, afterValue)
		}
	}
	for field := range afterFields {
		if _, transient := fscp01ReplayTransientFields[field]; transient {
			continue
		}
		if _, ok := beforeFields[field]; !ok {
			t.Fatalf("replay %s canonical field %q appeared only after handoff", shape, field)
		}
	}
	t.Logf("FSCP-01 replay parity shape=%s canonicalFields=%d transientExclusions=%d", shape, len(beforeFields)-len(fscp01ReplayTransientFields), len(fscp01ReplayTransientFields))
}

func assertFSCP01CanonicalReplayParity(
	t *testing.T,
	before, after []factoryapi.FactoryEvent,
) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("canonical replay event count = %d, want %d", len(after), len(before))
	}
	if len(before) == 0 {
		t.Fatal("canonical replay parity has no events")
	}
	for index := range before {
		beforePayload, err := json.Marshal(before[index])
		if err != nil {
			t.Fatalf("marshal canonical event before replay at index %d: %v", index, err)
		}
		afterPayload, err := json.Marshal(after[index])
		if err != nil {
			t.Fatalf("marshal canonical event after replay at index %d: %v", index, err)
		}
		if !bytes.Equal(beforePayload, afterPayload) {
			t.Fatalf("canonical replay event at index %d changed: before=%s after=%s", index, beforePayload, afterPayload)
		}
	}
	t.Logf("FSCP-01 canonical replay parity: events=%d ordering=exact association=retained", len(before))
}
