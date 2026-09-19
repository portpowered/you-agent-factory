package restart_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	restartBaselineEvidenceSchema = "factory.restart-baseline.v1"
	restartBaselineStoryID        = "factory-reliability-host-restart-single-instance-recovery-001-001"
	restartRecoveryStoryID        = "factory-reliability-host-restart-single-instance-recovery-001-003"
	restartLogTailLimit           = 8192
)

type restartBaselineEvidence struct {
	SchemaVersion              string                       `json:"schemaVersion"`
	StoryID                    string                       `json:"storyId"`
	TestName                   string                       `json:"testName"`
	Outcome                    string                       `json:"outcome"`
	ExitCode                   int                          `json:"testExitCode"`
	StartedAt                  time.Time                    `json:"startedAt"`
	CompletedAt                time.Time                    `json:"completedAt,omitempty"`
	EvidencePath               string                       `json:"evidencePath,omitempty"`
	Artifact                   restartCLIArtifactIdentity   `json:"artifact"`
	FixtureSHA256              map[string]string            `json:"fixtureSha256"`
	SourceRecordingSHA256      string                       `json:"sourceRecordingSha256,omitempty"`
	SourceRecordingAfterSHA256 string                       `json:"sourceRecordingAfterSha256,omitempty"`
	SuccessorRecordingSHA256   string                       `json:"successorRecordingSha256,omitempty"`
	ManualWorkMoves            []string                     `json:"baselineManualWorkMoves,omitempty"`
	Generations                []restartGenerationEvidence  `json:"processGenerations"`
	Observations               []restartPublicObservation   `json:"publicObservations"`
	TimingSamples              []restartTimingSample        `json:"timingSamples"`
	TimingRange                restartTimingRange           `json:"timingRange"`
	ScenarioID                 string                       `json:"scenarioId,omitempty"`
	FailureCases               []restartFailureCaseEvidence `json:"failureCases,omitempty"`
	RestartScenarios           []restartScenarioEvidence    `json:"restartScenarios,omitempty"`
}

type restartFailureCaseEvidence struct {
	ID                   string `json:"id"`
	SourceSHA256         string `json:"sourceSha256,omitempty"`
	SourceAfterSHA256    string `json:"sourceAfterSha256,omitempty"`
	ExitCode             *int   `json:"exitCode,omitempty"`
	ErrorCode            string `json:"errorCode,omitempty"`
	DiagnosticCount      int    `json:"diagnosticCount"`
	RecoveryRecordCount  int    `json:"recoveryRecordCount"`
	RecoverySuccessCount int    `json:"recoverySuccessCount"`
	DurationNanoseconds  int64  `json:"durationNanoseconds,omitempty"`
	Outcome              string `json:"outcome"`
}

type restartScenarioEvidence struct {
	StoryID                    string                       `json:"storyId"`
	ScenarioID                 string                       `json:"scenarioId"`
	TestName                   string                       `json:"testName"`
	Outcome                    string                       `json:"outcome"`
	StartedAt                  time.Time                    `json:"startedAt"`
	CompletedAt                time.Time                    `json:"completedAt"`
	Artifact                   restartCLIArtifactIdentity   `json:"artifact"`
	FixtureSHA256              map[string]string            `json:"fixtureSha256"`
	SourceRecordingSHA256      string                       `json:"sourceRecordingSha256,omitempty"`
	SourceRecordingAfterSHA256 string                       `json:"sourceRecordingAfterSha256,omitempty"`
	SuccessorRecordingSHA256   string                       `json:"successorRecordingSha256,omitempty"`
	ProcessGenerations         []restartGenerationEvidence  `json:"processGenerations"`
	PublicObservations         []restartPublicObservation   `json:"publicObservations"`
	FailureCases               []restartFailureCaseEvidence `json:"failureCases,omitempty"`
	TimingSamples              []restartTimingSample        `json:"timingSamples"`
	TimingRange                restartTimingRange           `json:"timingRange"`
}

type restartGenerationEvidence struct {
	Name                  string                `json:"name"`
	Command               []string              `json:"command,omitempty"`
	StartedAt             time.Time             `json:"startedAt,omitempty"`
	ReadyAt               time.Time             `json:"readyAt,omitempty"`
	ShutdownStartedAt     time.Time             `json:"shutdownStartedAt,omitempty"`
	StoppedAt             time.Time             `json:"stoppedAt,omitempty"`
	ExitCode              *int                  `json:"exitCode,omitempty"`
	WaitError             string                `json:"waitError,omitempty"`
	Stdout                restartCapturedOutput `json:"stdout"`
	Stderr                restartCapturedOutput `json:"stderr"`
	RuntimeLogs           []restartLogEvidence  `json:"runtimeLogs"`
	StartupDurationNanos  int64                 `json:"startupDurationNanoseconds,omitempty"`
	ShutdownDurationNanos int64                 `json:"shutdownDurationNanoseconds,omitempty"`
	Diagnostics           string                `json:"diagnostics,omitempty"`
	CapturedAt            time.Time             `json:"capturedAt"`
}

type restartCapturedOutput struct {
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Tail      string `json:"tail,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type restartLogEvidence struct {
	Name      string `json:"name"`
	Bytes     int64  `json:"bytes"`
	SHA256    string `json:"sha256"`
	Tail      string `json:"tail,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type restartPublicObservation struct {
	Name                     string                    `json:"name"`
	CapturedAt               time.Time                 `json:"capturedAt"`
	FactorySessionCount      int                       `json:"factorySessionCount"`
	FactorySessionIDs        []string                  `json:"factorySessionIds"`
	WorkCount                int                       `json:"workCount"`
	WorkIDs                  []string                  `json:"workIds"`
	EventCount               int                       `json:"eventCount"`
	DispatchCount            int                       `json:"dispatchCount"`
	ActiveDispatchCount      int                       `json:"activeDispatchCount"`
	WorkerSessionCount       int                       `json:"workerSessionCount"`
	ActiveWorkerSessionCount int                       `json:"activeWorkerSessionCount"`
	ActiveDispatchOwnerCount int                       `json:"activeDispatchOwnerCount"`
	Dispatches               []restartDispatchEvidence `json:"dispatches"`
}

type restartDispatchEvidence struct {
	ID                 string   `json:"id"`
	WorkIDs            []string `json:"workIds"`
	WorkerSessionIDs   []string `json:"workerSessionIds"`
	RequestEvents      int      `json:"requestEvents"`
	ResponseEvents     int      `json:"responseEvents"`
	InterruptedEvents  int      `json:"interruptedEvents"`
	ReconciledStatuses []string `json:"reconciledStatuses,omitempty"`
	Active             bool     `json:"active"`
}

type restartTimingSample struct {
	Name        string `json:"name"`
	Duration    string `json:"duration"`
	Nanoseconds int64  `json:"nanoseconds"`
}

type restartTimingRange struct {
	SampleCount        int    `json:"sampleCount"`
	MinimumNanoseconds int64  `json:"minimumNanoseconds,omitempty"`
	MaximumNanoseconds int64  `json:"maximumNanoseconds,omitempty"`
	Minimum            string `json:"minimum,omitempty"`
	Maximum            string `json:"maximum,omitempty"`
}

func beginRestartBaselineEvidence(artifact restartCLIArtifactIdentity) *restartBaselineEvidence {
	restartEvidenceMu.Lock()
	evidence := restartRunEvidence
	if evidence == nil {
		evidence = &restartBaselineEvidence{
			SchemaVersion:    restartBaselineEvidenceSchema,
			FixtureSHA256:    make(map[string]string),
			RestartScenarios: make([]restartScenarioEvidence, 0),
		}
		restartRunEvidence = evidence
	}
	evidence.StoryID = restartBaselineStoryID
	evidence.TestName = "TestRestoredReviewTransitionDispatchesEveryMigratedPair"
	evidence.Outcome = "RUNNING"
	evidence.StartedAt = time.Now().UTC()
	evidence.Artifact = artifact
	if evidence.FixtureSHA256 == nil {
		evidence.FixtureSHA256 = make(map[string]string)
	}
	restartEvidenceMu.Unlock()
	return evidence
}

func newRestartScenarioEvidence(artifact restartCLIArtifactIdentity, scenarioID, testName string) *restartBaselineEvidence {
	return &restartBaselineEvidence{
		SchemaVersion: restartBaselineEvidenceSchema,
		StoryID:       restartRecoveryStoryID,
		ScenarioID:    scenarioID,
		TestName:      testName,
		Outcome:       "RUNNING",
		StartedAt:     time.Now().UTC(),
		Artifact:      artifact,
		FixtureSHA256: make(map[string]string),
	}
}

func ensureRestartEvidence(artifact restartCLIArtifactIdentity) {
	restartEvidenceMu.Lock()
	defer restartEvidenceMu.Unlock()
	if restartRunEvidence != nil {
		return
	}
	restartRunEvidence = &restartBaselineEvidence{
		SchemaVersion:    restartBaselineEvidenceSchema,
		StoryID:          restartBaselineStoryID,
		TestName:         "TestRestoredReviewTransitionDispatchesEveryMigratedPair",
		Outcome:          "RUNNING",
		StartedAt:        time.Now().UTC(),
		Artifact:         artifact,
		FixtureSHA256:    make(map[string]string),
		RestartScenarios: make([]restartScenarioEvidence, 0),
	}
}

func (evidence *restartBaselineEvidence) publishRestartScenario(t *testing.T) {
	t.Helper()
	if evidence == nil || evidence.ScenarioID == "" {
		return
	}
	evidence.CompletedAt = time.Now().UTC()
	if t.Failed() {
		evidence.Outcome = "FAIL"
	} else if evidence.Outcome == "RUNNING" {
		evidence.Outcome = "PASS"
	}
	evidence.TimingRange = timingRange(evidence.TimingSamples)
	scenario := restartScenarioEvidence{
		StoryID:                    evidence.StoryID,
		ScenarioID:                 evidence.ScenarioID,
		TestName:                   evidence.TestName,
		Outcome:                    evidence.Outcome,
		StartedAt:                  evidence.StartedAt,
		CompletedAt:                evidence.CompletedAt,
		Artifact:                   evidence.Artifact,
		FixtureSHA256:              cloneStringStringMap(evidence.FixtureSHA256),
		SourceRecordingSHA256:      evidence.SourceRecordingSHA256,
		SourceRecordingAfterSHA256: evidence.SourceRecordingAfterSHA256,
		SuccessorRecordingSHA256:   evidence.SuccessorRecordingSHA256,
		ProcessGenerations:         append([]restartGenerationEvidence(nil), evidence.Generations...),
		PublicObservations:         append([]restartPublicObservation(nil), evidence.Observations...),
		FailureCases:               append([]restartFailureCaseEvidence(nil), evidence.FailureCases...),
		TimingSamples:              append([]restartTimingSample(nil), evidence.TimingSamples...),
		TimingRange:                evidence.TimingRange,
	}
	restartEvidenceMu.Lock()
	if restartRunEvidence != nil {
		restartRunEvidence.RestartScenarios = append(restartRunEvidence.RestartScenarios, scenario)
	}
	restartEvidenceMu.Unlock()
}

func cloneStringStringMap(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func writeRestartBaselineEvidence(path string, testExitCode int) error {
	restartEvidenceMu.Lock()
	evidence := restartRunEvidence
	restartEvidenceMu.Unlock()
	if evidence == nil {
		evidence = &restartBaselineEvidence{
			SchemaVersion: restartBaselineEvidenceSchema,
			StoryID:       restartBaselineStoryID,
			TestName:      "TestRestoredReviewTransitionDispatchesEveryMigratedPair",
			Outcome:       "NOT_RUN",
			FixtureSHA256: map[string]string{},
		}
	}
	evidence.ExitCode = testExitCode
	evidence.CompletedAt = time.Now().UTC()
	evidence.EvidencePath = path
	if testExitCode == 0 {
		if evidence.Outcome == "RUNNING" {
			evidence.Outcome = "PASS"
		}
	} else {
		evidence.Outcome = "FAIL"
	}
	evidence.TimingRange = timingRange(evidence.TimingSamples)

	encoded, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return fmt.Errorf("encode evidence bundle: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func (evidence *restartBaselineEvidence) hashFixtureFiles(root string, relativePaths ...string) error {
	for _, relative := range relativePaths {
		path := filepath.Join(root, filepath.FromSlash(relative))
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture %q: %w", relative, err)
		}
		evidence.FixtureSHA256[filepath.ToSlash(relative)] = sha256Hex(contents)
	}
	return nil
}

func (evidence *restartBaselineEvidence) verifyFixtureFilesUnchanged(root string) error {
	for relative, want := range evidence.FixtureSHA256 {
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("read fixture %q after recovery: %w", relative, err)
		}
		if got := sha256Hex(contents); got != want {
			return fmt.Errorf("fixture %q changed from SHA-256 %s to %s", relative, want, got)
		}
	}
	return nil
}

func (evidence *restartBaselineEvidence) recordSourceRecording(path string) error {
	sha, _, err := restartFileSHA256(path)
	if err != nil {
		return err
	}
	evidence.SourceRecordingSHA256 = sha
	return nil
}

func (evidence *restartBaselineEvidence) recordSuccessorRecordings(sourcePath, successorPath string) error {
	sourceHash, _, err := restartFileSHA256(sourcePath)
	if err != nil {
		return fmt.Errorf("hash source recording after resume: %w", err)
	}
	successorHash, _, err := restartFileSHA256(successorPath)
	if err != nil {
		return fmt.Errorf("hash successor recording: %w", err)
	}
	evidence.SourceRecordingAfterSHA256 = sourceHash
	evidence.SuccessorRecordingSHA256 = successorHash
	if evidence.SourceRecordingSHA256 != sourceHash {
		return errors.New("resume mutated the source recording")
	}
	if filepath.Clean(sourcePath) == filepath.Clean(successorPath) {
		return errors.New("source and successor recording paths alias")
	}
	return nil
}

func restartFileSHA256(path string) (string, int64, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	return sha256Hex(contents), int64(len(contents)), nil
}

func sha256Hex(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:])
}

func (evidence *restartBaselineEvidence) trackDaemon(t *testing.T, name string, daemon *boardPersistenceDaemon) {
	t.Helper()
	entry := restartGenerationEvidence{Name: name}
	evidence.Generations = append(evidence.Generations, entry)
	index := len(evidence.Generations) - 1
	t.Cleanup(func() {
		daemon.cleanup()
		evidence.captureDaemon(index, daemon)
	})
}

func (evidence *restartBaselineEvidence) captureDaemon(index int, daemon *boardPersistenceDaemon) {
	if index < 0 || index >= len(evidence.Generations) || daemon == nil {
		return
	}
	entry := &evidence.Generations[index]
	if !entry.CapturedAt.IsZero() {
		return
	}
	entry.Command = append([]string{restartCLIArtifact.Path}, daemon.cmd.Args[1:]...)
	entry.StartedAt = daemon.startedAt.UTC()
	entry.ReadyAt = daemon.readyAt.UTC()
	entry.ShutdownStartedAt = daemon.shutdownStartedAt.UTC()
	entry.StoppedAt = daemon.stoppedAt.UTC()
	entry.CapturedAt = time.Now().UTC()
	if daemon.readyAt.After(daemon.startedAt) {
		duration := daemon.readyAt.Sub(daemon.startedAt)
		entry.StartupDurationNanos = int64(duration)
		evidence.addTiming(entry.Name+".startup-to-ready", duration)
	}
	if daemon.stoppedAt.After(daemon.shutdownStartedAt) && !daemon.shutdownStartedAt.IsZero() {
		duration := daemon.stoppedAt.Sub(daemon.shutdownStartedAt)
		entry.ShutdownDurationNanos = int64(duration)
		evidence.addTiming(entry.Name+".shutdown", duration)
	}
	entry.Stdout = capturedOutput(daemon.stdout.Bytes())
	entry.Stderr = capturedOutput(daemon.stderr.Bytes())
	if waitErr := daemon.waitError(); waitErr != nil {
		entry.WaitError = waitErr.Error()
	}
	if daemon.cmd.ProcessState != nil {
		code := daemon.cmd.ProcessState.ExitCode()
		entry.ExitCode = &code
	}
	entry.RuntimeLogs = collectRestartRuntimeLogs(daemon.logDir)
}

func (evidence *restartBaselineEvidence) addTiming(name string, duration time.Duration) {
	evidence.TimingSamples = append(evidence.TimingSamples, restartTimingSample{
		Name:        name,
		Duration:    duration.String(),
		Nanoseconds: int64(duration),
	})
}

func (evidence *restartBaselineEvidence) addPublicObservation(observation restartPublicObservation) {
	evidence.Observations = append(evidence.Observations, observation)
}

func (evidence *restartBaselineEvidence) capturePublicObservation(t *testing.T, name, baseURL string) restartPublicObservation {
	t.Helper()
	ctx := t.Context()
	sessions, err := readBoardSessions(ctx, baseURL)
	if err != nil {
		t.Fatalf("read public Factory Session counts for %s: %v", name, err)
	}
	works, err := readBoardWorkList(ctx, baseURL)
	if err != nil {
		t.Fatalf("read public Work counts for %s: %v", name, err)
	}
	events, err := readBoardEvents(ctx, baseURL)
	if err != nil {
		t.Fatalf("read public Factory Event counts for %s: %v", name, err)
	}
	dispatches, err := readBoardDispatchStates(ctx, baseURL)
	if err != nil {
		t.Fatalf("read public dispatch and owner associations for %s: %v", name, err)
	}

	observation := restartPublicObservation{
		Name:                name,
		CapturedAt:          time.Now().UTC(),
		FactorySessionCount: len(sessions.Sessions),
		WorkCount:           len(works.Results),
		EventCount:          len(events),
		DispatchCount:       len(dispatches),
		FactorySessionIDs:   make([]string, 0, len(sessions.Sessions)),
		WorkIDs:             make([]string, 0, len(works.Results)),
		Dispatches:          make([]restartDispatchEvidence, 0, len(dispatches)),
	}
	for _, session := range sessions.Sessions {
		observation.FactorySessionIDs = append(observation.FactorySessionIDs, session.Id)
	}
	sort.Strings(observation.FactorySessionIDs)

	activeDispatchIDs := make(map[string]struct{})
	activeOwnerIDs := make(map[string]struct{})
	workerSessions := make(map[string]factoryapi.WorkerSessionObservation)
	for _, work := range works.Results {
		workID := boardPersistenceStringPointerValue(work.WorkId)
		if workID == "" {
			continue
		}
		observation.WorkIDs = append(observation.WorkIDs, workID)
		listed, err := readBoardWorkerSessions(ctx, baseURL, "", workID)
		if err != nil {
			t.Fatalf("read public Worker Session observations for %s Work %q: %v", name, workID, err)
		}
		for _, worker := range listed.Sessions {
			if worker.WorkerSessionId != "" {
				workerSessions[worker.WorkerSessionId] = worker
			}
		}
		for _, dispatch := range activeBoardDispatches(dispatches, workID) {
			activeDispatchIDs[dispatch.ID] = struct{}{}
			for _, ownerID := range dispatch.WorkerSessionIDs {
				if ownerID != "" {
					activeOwnerIDs[ownerID] = struct{}{}
				}
			}
		}
	}
	sort.Strings(observation.WorkIDs)
	observation.ActiveDispatchCount = len(activeDispatchIDs)
	observation.WorkerSessionCount = len(workerSessions)
	for _, worker := range workerSessions {
		if worker.State == factoryapi.WorkerSessionObservationStateRunning || worker.State == factoryapi.WorkerSessionObservationStateStarting {
			observation.ActiveWorkerSessionCount++
		}
	}
	observation.ActiveDispatchOwnerCount = len(activeOwnerIDs)

	dispatchIDs := make([]string, 0, len(dispatches))
	for id := range dispatches {
		dispatchIDs = append(dispatchIDs, id)
	}
	sort.Strings(dispatchIDs)
	for _, id := range dispatchIDs {
		dispatch := dispatches[id]
		workIDs := make([]string, 0, len(dispatch.WorkIDs))
		for workID := range dispatch.WorkIDs {
			workIDs = append(workIDs, workID)
		}
		sort.Strings(workIDs)
		workerSessionIDs := append([]string(nil), dispatch.WorkerSessionIDs...)
		sort.Strings(workerSessionIDs)
		reconciledStatuses := make([]string, 0, len(dispatch.ReconciledStatuses))
		for _, status := range dispatch.ReconciledStatuses {
			reconciledStatuses = append(reconciledStatuses, string(status))
		}
		isActive := false
		if _, ok := activeDispatchIDs[id]; ok {
			isActive = true
		}
		observation.Dispatches = append(observation.Dispatches, restartDispatchEvidence{
			ID: id, WorkIDs: workIDs, WorkerSessionIDs: workerSessionIDs,
			RequestEvents: dispatch.RequestEvents, ResponseEvents: dispatch.ResponseEvents,
			InterruptedEvents: dispatch.InterruptedEvents, ReconciledStatuses: reconciledStatuses,
			Active: isActive,
		})
	}
	evidence.addPublicObservation(observation)
	return observation
}

func capturedOutput(contents []byte) restartCapturedOutput {
	evidence := restartCapturedOutput{Bytes: int64(len(contents)), SHA256: sha256Hex(contents)}
	if len(contents) > restartLogTailLimit {
		contents = contents[len(contents)-restartLogTailLimit:]
		evidence.Truncated = true
	}
	evidence.Tail = string(contents)
	return evidence
}

func collectRestartRuntimeLogs(root string) []restartLogEvidence {
	var logs []restartLogEvidence
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		captured := capturedOutput(contents)
		logs = append(logs, restartLogEvidence{
			Name: filepath.ToSlash(relative), Bytes: captured.Bytes, SHA256: captured.SHA256,
			Tail: captured.Tail, Truncated: captured.Truncated,
		})
		return nil
	})
	sort.Slice(logs, func(i, j int) bool { return logs[i].Name < logs[j].Name })
	return logs
}

func timingRange(samples []restartTimingSample) restartTimingRange {
	rangeResult := restartTimingRange{SampleCount: len(samples)}
	if len(samples) == 0 {
		return rangeResult
	}
	minimum, maximum := samples[0].Nanoseconds, samples[0].Nanoseconds
	for _, sample := range samples[1:] {
		if sample.Nanoseconds < minimum {
			minimum = sample.Nanoseconds
		}
		if sample.Nanoseconds > maximum {
			maximum = sample.Nanoseconds
		}
	}
	rangeResult.MinimumNanoseconds = minimum
	rangeResult.MaximumNanoseconds = maximum
	rangeResult.Minimum = time.Duration(minimum).String()
	rangeResult.Maximum = time.Duration(maximum).String()
	return rangeResult
}

func assertRestartPublicCounts(
	t *testing.T,
	observation restartPublicObservation,
	wantSessions, wantWorks, wantActiveDispatches, wantActiveOwners int,
) {
	t.Helper()
	if observation.FactorySessionCount != wantSessions || observation.WorkCount != wantWorks ||
		observation.ActiveDispatchCount != wantActiveDispatches || observation.ActiveDispatchOwnerCount != wantActiveOwners {
		t.Fatalf("public restart counts at %q = sessions:%d works:%d activeDispatches:%d activeOwners:%d; want %d/%d/%d/%d",
			observation.Name, observation.FactorySessionCount, observation.WorkCount,
			observation.ActiveDispatchCount, observation.ActiveDispatchOwnerCount,
			wantSessions, wantWorks, wantActiveDispatches, wantActiveOwners)
	}
}

func (evidence *restartBaselineEvidence) recordManualWorkMove(workID, state string) {
	evidence.ManualWorkMoves = append(evidence.ManualWorkMoves, fmt.Sprintf("%s -> %s", workID, state))
}

func evidenceLogTailForError(logs []restartLogEvidence) string {
	parts := make([]string, 0, len(logs))
	for _, log := range logs {
		if strings.TrimSpace(log.Tail) != "" {
			parts = append(parts, log.Name+": "+log.Tail)
		}
	}
	return strings.Join(parts, "\n")
}
