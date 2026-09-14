package root_composition_test

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	resumedResponseScopeRecordingEnv = "INFINITE_YOU_RESUME_RESPONSE_SCOPE_RECORDING"
	resumedResponseScopeArchiveEnv   = "INFINITE_YOU_RESUME_RESPONSE_SCOPE_FACTORY_ARCHIVE"

	resumedResponseScopeRecordingSHA256 = "A4CE2FD1F587573224DB5283F797B12FA549315CEBB1E152AA3B6CAC60873EE9"
	resumedResponseScopeRecordingBytes  = int64(23606575)
	resumedResponseScopeArchiveSHA256   = "CAF067534E9FCD1F536298F3273F0049FC925BE251D44C5AB9179082BBF968AD"
	resumedResponseScopeArchiveBytes    = int64(145960)

	resumedResponseScopeReservedWorkID   = "work-thoughts-213"
	resumedResponseScopeWorkType         = "thoughts"
	resumedResponseScopeSecretMarker     = "story003-untrusted-payload-secret"
	resumedResponseScopeGeneratedName    = "generated successor thought"
	resumedResponseScopeGeneratedMarker  = "generated-success-payload"
	resumedResponseScopeControlledOutput = `{"decision":"ACCEPTED","feedback":"controlled replay result","output":"done"}`
	resumedResponseScopeSuccessorID      = "00000000-0000-4000-8000-000000000003"
	resumedResponseScopeRuntimeID        = "00000000-0000-4000-8000-000000000004"

	resumedResponseScopeObservationTimeout = 90 * time.Second
	resumedResponseScopeStreamTimeout      = 90 * time.Second
)

var resumedResponseScopeWorkIDPattern = regexp.MustCompile(
	`\bwork-([A-Za-z0-9][A-Za-z0-9_-]*)-([0-9]+)\b`,
)

// TestResumedSuccessorResponseScopeAndWorkAdmission exercises one copied
// legacy recording through the production root composition. It keeps the
// provider edge gated until the public HTTP and CLI conflict snapshots have
// been taken, then admits one generated Work and observes its live streams.
// The large artifacts are intentionally operator-supplied rather than checked
// into the repository; ordinary package runs skip when they are unavailable.
func TestResumedSuccessorResponseScopeAndWorkAdmission(t *testing.T) {
	t.Parallel()
	acquireRootCompositionFixtureSlot(t)

	artifacts := stageResumedResponseScopeArtifacts(t)
	gate := make(chan struct{})
	providerRunner := newResumedResponseScopeProviderRunner(gate)
	responseEventIDs := &atomic.Uint64{}
	successorPath := filepath.Join(t.TempDir(), "successor.jsonl")

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                artifacts.factoryDir,
		WorkingDirectory:          artifacts.extractedRoot,
		FactoryConfigPath:         filepath.Join(artifacts.factoryDir, "factory.json"),
		ServerReadyTimeout:        resumedResponseScopeObservationTimeout,
		WaitForServiceModeRuntime: true,
		Args: []string{
			"--resume", artifacts.recordingCopy,
			"--record", successorPath,
		},
		Edges: serviceedges.Edges{
			FactorySessionIDGenerator: func() string {
				return resumedResponseScopeSuccessorID
			},
			FactorySessionRuntimeInstanceIDGenerator: func() string {
				return resumedResponseScopeRuntimeID
			},
			FactorySessionResponseEventIDGenerator: func() string {
				return fmt.Sprintf("story003-response-event-%d", responseEventIDs.Add(1))
			},
			ProviderCommandRunner: providerRunner,
			ScriptCommandRunner:   support.NewStaticSuccessCommandRunner("blocked"),
		},
	})

	baseURL := server.URL()
	assertResumedResponseScopeListener(t, baseURL)
	if err := providerRunner.WaitForCall(t.Context()); err != nil {
		t.Fatalf("wait for resumed provider dispatch: %v", err)
	}
	t.Log("resumed provider dispatch observed")

	session := support.GetDefaultSession(t, baseURL)
	assertResumedSuccessorIdentity(t, session)
	repeatedSession := support.GetDefaultSession(t, baseURL)
	assertResumedSuccessorIdentityStable(t, session, repeatedSession)
	sessionID := session.Id
	t.Logf(
		"resumed successor session identity stable: public=%s canonical=%s",
		sessionID,
		session.Runtime.StreamIdentity.FactorySessionID,
	)

	initialEvents := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	assertResumedHistoricalPrefix(t, initialEvents, artifacts.ledger)
	t.Logf("resumed historical prefix observed: %d events", len(initialEvents))
	before, beforeEvents, beforeResponseEvents := captureOpenResumedScopeSnapshot(t, baseURL, sessionID)
	if len(beforeEvents) == 0 {
		t.Fatal("resumed canonical event snapshot is empty")
	}
	t.Logf("resumed open scope snapshot captured: canonical=%d response=%d", len(beforeEvents), len(beforeResponseEvents))

	t.Log("opening future canonical and response streams")
	if len(beforeEvents) < 2 {
		t.Fatalf("resumed canonical event snapshot has %d events, want at least two for a replay cursor", len(beforeEvents))
	}
	// A one-event replay prefix makes the legacy-compatible SSE handler flush
	// its headers before the generated Work is admitted. The cursor itself is
	// still the acknowledged canonical event immediately before the baseline
	// tail, so no event is lost from the live subscription.
	canonicalCursor := resumedResponseScopeCursor(beforeEvents[len(beforeEvents)-2])
	canonicalStream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURLWithCursor(baseURL, sessionID, canonicalCursor),
	)
	defer canonicalStream.Close()
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURLWithAfterSequence(
			baseURL,
			sessionID,
			lastResumedResponseSequence(beforeResponseEvents),
		),
	)
	t.Log("future canonical and response streams open")

	reserved := resumedResponseScopeWork(
		resumedResponseScopeReservedWorkID,
		"reserved historical work",
		resumedResponseScopeSecretMarker,
	)
	assertReservedWorkReadable(t, baseURL, sessionID, reserved)

	httpConflictRequest := factoryapi.WorkRequest{
		RequestId: "story003-http-reserved-conflict",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &[]factoryapi.Work{reserved},
	}
	httpStatus, httpBody := putResumedWorkRequest(t, baseURL, sessionID, httpConflictRequest)
	assertWorkRequestConflict(t, httpStatus, httpBody, "HTTP")
	t.Log("HTTP reserved Work conflict observed")
	afterHTTP := captureOpenResumedScopeSnapshotOnly(t, baseURL, sessionID)
	assertResumedScopeSnapshotUnchanged(t, before, afterHTTP, "HTTP reserved Work conflict")
	t.Log("HTTP reserved Work conflict snapshot unchanged")

	cliConflictRequest := factoryapi.WorkRequest{
		RequestId: "story003-cli-reserved-conflict",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			resumedResponseScopeWork(
				resumedResponseScopeReservedWorkID,
				"reserved historical work from CLI",
				resumedResponseScopeSecretMarker,
			),
		},
	}
	cliStdout, cliStderr, cliErr := executeResumedResponseScopeCLI(
		t,
		server,
		[]string{
			"you", "--server", baseURL, "--json", "submit", "batch",
			"--session", sessionID, resumedInlineJSON(t, cliConflictRequest),
		},
	)
	if cliErr == nil {
		t.Fatal("CLI reserved Work conflict succeeded")
	}
	cliDiagnostic := cliErr.Error() + "\n" + cliStderr
	for _, marker := range []string{
		"batch submission failed (409)",
		"code=CONFLICT",
		"family=CONFLICT",
	} {
		if !strings.Contains(cliDiagnostic, marker) {
			t.Fatalf("CLI reserved Work conflict missing %q: %s", marker, cliDiagnostic)
		}
	}
	if cliStdout != "" {
		t.Fatalf("CLI reserved Work conflict emitted success stdout: %q", cliStdout)
	}
	if strings.Contains(cliDiagnostic, resumedResponseScopeSecretMarker) {
		t.Fatalf("CLI reserved Work conflict leaked payload marker: %s", cliDiagnostic)
	}
	t.Log("CLI reserved Work conflict observed")
	afterCLI := captureOpenResumedScopeSnapshotOnly(t, baseURL, sessionID)
	assertResumedScopeSnapshotUnchanged(t, before, afterCLI, "CLI reserved Work conflict")
	t.Log("CLI reserved Work conflict snapshot unchanged")

	generatedWorkType := "validation"
	generatedWorkID := resumedResponseScopeGeneratedWorkID(artifacts.ledger, generatedWorkType)
	providerCallsBeforeGenerated := providerRunner.CallCount()
	generatedRequest := factoryapi.WorkRequest{
		RequestId: "story003-generated-work",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			resumedResponseScopeWorkOfType(
				generatedWorkType,
				generatedWorkID,
				resumedResponseScopeGeneratedName,
				resumedResponseScopeGeneratedMarker,
			),
		},
	}
	t.Log("submitting generated Work")
	var submitted factoryapi.UpsertWorkRequestResponse
	generatedStatus, generatedBody := putResumedWorkRequest(t, baseURL, sessionID, generatedRequest)
	if generatedStatus < http.StatusOK || generatedStatus >= http.StatusMultipleChoices {
		t.Fatalf("generated Work admission status = %d: %s", generatedStatus, strings.TrimSpace(string(generatedBody)))
	}
	if err := json.Unmarshal(generatedBody, &submitted); err != nil {
		t.Fatalf("decode generated Work admission: %v", err)
	}
	if len(submitted.Works) != 1 || submitted.Works[0].WorkId != generatedWorkID {
		t.Fatalf("generated Work admission = %#v, want generated Work ID %q", submitted, generatedWorkID)
	}
	if generatedWorkID == resumedResponseScopeReservedWorkID {
		t.Fatalf("generated Work reused reserved historical ID %q", generatedWorkID)
	}
	t.Logf("generated Work admitted: %s", generatedWorkID)
	generatedProviderContext, cancelGeneratedProviderWait := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelGeneratedProviderWait()
	if err := providerRunner.WaitForCallCount(generatedProviderContext, providerCallsBeforeGenerated+1); err != nil {
		t.Fatalf("wait for generated Work provider dispatch: %v", err)
	}
	t.Logf("generated Work provider dispatch observed: calls=%d workDirs=%q", providerRunner.CallCount(), providerRunner.WorkDirs())
	admittedWork := support.GetJSON[factoryapi.Work](
		t,
		support.SessionWorkURL(baseURL, sessionID, "/work/"+url.PathEscape(generatedWorkID)),
	)
	statusAfterAdmission := support.GetJSON[factoryapi.StatusResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/status",
	)
	admittedEvents := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	t.Logf(
		"generated Work public state after admission: id=%s state=%#v runtime=%s factory=%s categories=%#v resources=%#v events=%d",
		generatedWorkID,
		admittedWork.State,
		statusAfterAdmission.RuntimeStatus,
		statusAfterAdmission.FactoryState,
		statusAfterAdmission.Categories,
		statusAfterAdmission.Resources,
		len(admittedEvents),
	)

	providerRunner.Release()
	providerReturnContext, cancelProviderReturnWait := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelProviderReturnWait()
	if err := providerRunner.WaitForReturnCount(providerReturnContext, providerCallsBeforeGenerated+1); err != nil {
		t.Fatalf("wait for generated ProviderCommandRunner return: %v", err)
	}
	t.Logf("generated ProviderCommandRunner returned: returns=%d", providerRunner.ReturnCount())
	futureEvents := readResumedFutureEventsUntilWorkTerminal(t, canonicalStream, generatedWorkID)
	dispatchID := assertGeneratedWorkCanonicalProgress(t, futureEvents, generatedWorkID)
	responseEvents := readResumedResponseEventsUntilDispatchTerminal(t, responseStream, dispatchID)
	assertGeneratedWorkResponseProgress(t, responseEvents, dispatchID, sessionID)

	generatedWork := waitForResumedWorkTerminal(t, baseURL, sessionID, generatedWorkID)
	assertGeneratedWorkTerminal(t, generatedWork, generatedWorkID)
	assertGeneratedSuffixesAboveHistory(t, baseURL, sessionID, artifacts.ledger)
	assertResumedResponseScopeNoWarnings(t, futureEvents, responseEvents)
	providerRunner.AssertSafeRequests(t)
	t.Logf("generated Work terminal and response lifecycle observed: canonical=%d response=%d", len(futureEvents), len(responseEvents))

	terminationStatus, terminationBody := postResumedLifecycleControl(
		t, baseURL, sessionID, "terminate",
	)
	if terminationStatus == http.StatusConflict {
		var terminalControl factoryapi.FactorySessionLifecycleControlResponse
		if err := json.Unmarshal(terminationBody, &terminalControl); err != nil {
			t.Fatalf("decode automatic terminal lifecycle response: %v", err)
		}
		if terminalControl.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
			t.Fatalf("automatic terminal lifecycle conflict = %#v, want TERMINAL_SESSION", terminalControl)
		}
	} else if terminationStatus != http.StatusOK && terminationStatus != http.StatusAccepted {
		t.Fatalf("terminate successor Factory Session status = %d: %s", terminationStatus, strings.TrimSpace(string(terminationBody)))
	}
	// The live Response Event stream closes on the terminal lifecycle event;
	// use that customer-visible signal instead of polling the status endpoint.
	responseStream.WaitClosed(resumedResponseScopeStreamTimeout)
	terminalSession := support.GetDefaultSession(t, baseURL)
	assertTerminalResumedSession(t, terminalSession)
	t.Log("resumed successor terminal lifecycle observed")
	responseStream.Close()

	terminalResponseEvents := readClosedResumedResponseEvents(t, baseURL, sessionID)
	assertResumedResponseEventOrder(t, terminalResponseEvents, sessionID, dispatchID)
	terminalCanonicalEvents := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	assertResumedSuccessorLifecycle(t, terminalCanonicalEvents, session.Runtime.StreamIdentity.FactorySessionID)
	terminalSnapshot := captureClosedResumedScopeSnapshot(t, baseURL, sessionID, terminalResponseEvents)

	httpTerminalStatus, httpTerminalBody := postResumedLifecycleControl(t, baseURL, sessionID, "cancel")
	assertTerminalLifecycleConflict(t, httpTerminalStatus, httpTerminalBody, "HTTP")
	afterHTTPTerminal := captureClosedResumedScopeSnapshot(t, baseURL, sessionID, nil)
	assertResumedScopeSnapshotUnchanged(t, terminalSnapshot, afterHTTPTerminal, "HTTP terminal lifecycle conflict")

	cliTerminalStdout, cliTerminalStderr, cliTerminalErr := executeResumedResponseScopeCLI(
		t,
		server,
		[]string{"you", "--server", baseURL, "--json", "session", "cancel", sessionID},
	)
	if cliTerminalErr == nil {
		t.Fatal("CLI terminal lifecycle mutation succeeded")
	}
	if !strings.Contains(cliTerminalErr.Error(), string(factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession)) {
		t.Fatalf("CLI terminal lifecycle error = %v, want TERMINAL_SESSION", cliTerminalErr)
	}
	if strings.Contains(cliTerminalErr.Error()+"\n"+cliTerminalStderr, resumedResponseScopeSecretMarker) {
		t.Fatalf("CLI terminal lifecycle conflict leaked payload marker")
	}
	if strings.TrimSpace(cliTerminalStdout) == "" {
		t.Fatal("CLI terminal lifecycle conflict omitted typed response")
	}
	var cliTerminalControl factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal([]byte(cliTerminalStdout), &cliTerminalControl); err != nil {
		t.Fatalf("decode CLI terminal lifecycle conflict: %v; stdout=%q stderr=%q", err, cliTerminalStdout, cliTerminalStderr)
	}
	if cliTerminalControl.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("CLI terminal lifecycle response = %#v, want TERMINAL_SESSION", cliTerminalControl)
	}
	afterCLITerminal := captureClosedResumedScopeSnapshot(t, baseURL, sessionID, nil)
	assertResumedScopeSnapshotUnchanged(t, terminalSnapshot, afterCLITerminal, "CLI terminal lifecycle conflict")

	server.Stop(t)
	assertRecordedSuccessorExists(t, successorPath)
}

type resumedResponseScopeArtifacts struct {
	recordingSource string
	recordingCopy   string
	archiveSource   string
	archiveCopy     string
	extractedRoot   string
	factoryDir      string
	ledger          resumedResponseScopeLedger
}

type resumedResponseScopeLedger struct {
	eventIDs             map[string]struct{}
	historicalWorkIDs    map[string]struct{}
	eventCount           int
	sessionCompleted     int
	historicalWorkMaxima map[string]int
}

type resumedReplayRecord struct {
	RecordType string          `json:"recordType"`
	Event      json.RawMessage `json:"event"`
}

func stageResumedResponseScopeArtifacts(t *testing.T) resumedResponseScopeArtifacts {
	t.Helper()
	recordingSource := strings.TrimSpace(os.Getenv(resumedResponseScopeRecordingEnv))
	archiveSource := strings.TrimSpace(os.Getenv(resumedResponseScopeArchiveEnv))
	if recordingSource == "" || archiveSource == "" {
		t.Skipf(
			"immutable resumed-successor artifacts unavailable: set %s and %s",
			resumedResponseScopeRecordingEnv,
			resumedResponseScopeArchiveEnv,
		)
	}
	recordingIdentity := requireResumedArtifactIdentity(
		t,
		recordingSource,
		resumedResponseScopeRecordingBytes,
		resumedResponseScopeRecordingSHA256,
		"recording source",
	)
	archiveIdentity := requireResumedArtifactIdentity(
		t,
		archiveSource,
		resumedResponseScopeArchiveBytes,
		resumedResponseScopeArchiveSHA256,
		"Factory archive source",
	)

	stageRoot := t.TempDir()
	recordingCopy := filepath.Join(stageRoot, "preserved-before-restart.jsonl")
	archiveCopy := filepath.Join(stageRoot, "factory-8f1e2e5.zip")
	copyResumedArtifact(t, recordingSource, recordingCopy)
	copyResumedArtifact(t, archiveSource, archiveCopy)
	if got := requireResumedArtifactIdentity(t, recordingCopy, recordingIdentity.bytes, recordingIdentity.hash, "recording copy"); got != recordingIdentity {
		t.Fatalf("recording copy identity = %#v, want %#v", got, recordingIdentity)
	}
	if got := requireResumedArtifactIdentity(t, archiveCopy, archiveIdentity.bytes, archiveIdentity.hash, "Factory archive copy"); got != archiveIdentity {
		t.Fatalf("Factory archive copy identity = %#v, want %#v", got, archiveIdentity)
	}
	t.Cleanup(func() {
		verifyResumedArtifactIdentity(t, recordingSource, recordingIdentity, "recording source after scenario")
		verifyResumedArtifactIdentity(t, recordingCopy, recordingIdentity, "recording copy after scenario")
		verifyResumedArtifactIdentity(t, archiveSource, archiveIdentity, "Factory archive source after scenario")
		verifyResumedArtifactIdentity(t, archiveCopy, archiveIdentity, "Factory archive copy after scenario")
	})

	extractedRoot, factoryDir := extractResumedFactoryArchive(t, archiveCopy)
	return resumedResponseScopeArtifacts{
		recordingSource: recordingSource,
		recordingCopy:   recordingCopy,
		archiveSource:   archiveSource,
		archiveCopy:     archiveCopy,
		extractedRoot:   extractedRoot,
		factoryDir:      factoryDir,
		ledger:          readResumedResponseScopeLedger(t, recordingCopy),
	}
}

type resumedArtifactIdentity struct {
	bytes int64
	hash  string
}

func requireResumedArtifactIdentity(
	t testing.TB,
	path string,
	wantBytes int64,
	wantHash string,
	label string,
) resumedArtifactIdentity {
	t.Helper()
	got, err := hashResumedArtifact(path)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	if got.bytes != wantBytes || got.hash != strings.ToUpper(wantHash) {
		t.Fatalf(
			"%s identity = (%d bytes, %s), want (%d bytes, %s)",
			label, got.bytes, got.hash, wantBytes, strings.ToUpper(wantHash),
		)
	}
	return got
}

func verifyResumedArtifactIdentity(
	t testing.TB,
	path string,
	want resumedArtifactIdentity,
	label string,
) {
	t.Helper()
	got, err := hashResumedArtifact(path)
	if err != nil {
		t.Errorf("%s: %v", label, err)
		return
	}
	if got != want {
		t.Errorf("%s identity = %#v, want %#v", label, got, want)
	}
}

func hashResumedArtifact(path string) (resumedArtifactIdentity, error) {
	file, err := os.Open(path)
	if err != nil {
		return resumedArtifactIdentity{}, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return resumedArtifactIdentity{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return resumedArtifactIdentity{}, err
	}
	return resumedArtifactIdentity{
		bytes: stat.Size(),
		hash:  strings.ToUpper(hex.EncodeToString(hash.Sum(nil))),
	}, nil
}

func copyResumedArtifact(t testing.TB, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatalf("open immutable artifact %q: %v", source, err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create immutable artifact copy %q: %v", destination, err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatalf("copy immutable artifact %q: %v", source, err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close immutable artifact copy %q: %v", destination, err)
	}
}

func extractResumedFactoryArchive(t testing.TB, archivePath string) (string, string) {
	t.Helper()
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open Factory archive: %v", err)
	}
	defer reader.Close()
	extractedRoot := t.TempDir()
	rootAbs, err := filepath.Abs(extractedRoot)
	if err != nil {
		t.Fatalf("resolve Factory archive extraction root: %v", err)
	}
	for _, entry := range reader.File {
		name := filepath.Clean(filepath.FromSlash(entry.Name))
		if name == "." || name == ".." || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) {
			t.Fatalf("unsafe Factory archive entry %q", entry.Name)
		}
		target := filepath.Join(extractedRoot, name)
		relative, err := filepath.Rel(rootAbs, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
			t.Fatalf("Factory archive entry escapes extraction root: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				t.Fatalf("create Factory archive directory %q: %v", entry.Name, err)
			}
			continue
		}
		if entry.FileInfo().Mode()&os.ModeSymlink != 0 {
			t.Fatalf("Factory archive contains unsupported symlink %q", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatalf("create Factory archive parent for %q: %v", entry.Name, err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatalf("open Factory archive entry %q: %v", entry.Name, err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			_ = input.Close()
			t.Fatalf("create extracted Factory archive entry %q: %v", entry.Name, err)
		}
		_, copyErr := io.Copy(output, input)
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil || closeInputErr != nil || closeOutputErr != nil {
			t.Fatalf("extract Factory archive entry %q: copy=%v inputClose=%v outputClose=%v", entry.Name, copyErr, closeInputErr, closeOutputErr)
		}
	}
	factoryDir := filepath.Join(extractedRoot, "factory")
	factoryConfigPath := filepath.Join(factoryDir, "factory.json")
	if _, err := os.Stat(factoryConfigPath); err != nil {
		t.Fatalf("extracted Factory archive missing factory/factory.json: %v", err)
	}
	// The archive is a direct Factory project, while session-scoped definition
	// version lookup resolves named factories beneath the selected Factory
	// root. Preserve the archive bytes and add the minimal derived catalog entry
	// needed by that public definition boundary.
	namedFactoryDir := filepath.Join(factoryDir, "you-agent-factory")
	if err := os.MkdirAll(namedFactoryDir, 0o700); err != nil {
		t.Fatalf("create extracted named Factory catalog entry: %v", err)
	}
	copyResumedArtifact(t, factoryConfigPath, filepath.Join(namedFactoryDir, "factory.json"))
	return extractedRoot, factoryDir
}

func readResumedResponseScopeLedger(t testing.TB, path string) resumedResponseScopeLedger {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open copied replay recording: %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 64<<20)
	ledger := resumedResponseScopeLedger{
		eventIDs:             make(map[string]struct{}),
		historicalWorkIDs:    make(map[string]struct{}),
		historicalWorkMaxima: make(map[string]int),
	}
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		var record resumedReplayRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode replay recording line %d: %v", lineNumber, err)
		}
		switch record.RecordType {
		case "header":
			continue
		case "event":
		default:
			t.Fatalf("replay recording line %d recordType = %q, want header or event", lineNumber, record.RecordType)
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal(record.Event, &event); err != nil {
			t.Fatalf("decode replay recording event line %d: %v", lineNumber, err)
		}
		if event.Id == "" {
			t.Fatalf("replay recording line %d has empty event ID", lineNumber)
		}
		if _, exists := ledger.eventIDs[event.Id]; exists {
			t.Fatalf("replay recording duplicates event ID %q", event.Id)
		}
		ledger.eventIDs[event.Id] = struct{}{}
		ledger.eventCount++
		if event.Type == factoryapi.FactoryEventTypeSessionCompleted {
			ledger.sessionCompleted++
		}
		for _, match := range resumedResponseScopeWorkIDPattern.FindAllStringSubmatch(string(scanner.Bytes()), -1) {
			value, err := strconv.Atoi(match[2])
			if err != nil {
				t.Fatalf("parse historical Work suffix %q: %v", match[0], err)
			}
			if value > ledger.historicalWorkMaxima[match[1]] {
				ledger.historicalWorkMaxima[match[1]] = value
			}
			ledger.historicalWorkIDs[match[0]] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan replay recording: %v", err)
	}
	if ledger.eventCount != 5801 {
		t.Fatalf("replay recording event count = %d, want 5801", ledger.eventCount)
	}
	if ledger.sessionCompleted == 0 {
		t.Fatal("replay recording has no historical SESSION_COMPLETED event")
	}
	return ledger
}

func assertResumedResponseScopeListener(t *testing.T, baseURL string) {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse functional API URL: %v", err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("functional API URL port %q: %v", parsed.Port(), err)
	}
	if port == 7437 {
		t.Fatal("functional resumed-successor scenario used reserved production port 7437")
	}
}

func assertResumedSuccessorIdentity(t *testing.T, session factoryapi.FactorySession) {
	t.Helper()
	if strings.TrimSpace(session.Id) == "" || !session.IsDefault {
		t.Fatalf("resumed Factory Session = %#v, want a named default session", session)
	}
	if _, err := uuid.Parse(session.Id); err != nil {
		t.Fatalf("resumed successor public session ID = %q, want UUID: %v", session.Id, err)
	}
	if session.Runtime.StreamIdentity == nil {
		t.Fatal("resumed Factory Session has no stream identity")
	}
	if _, err := uuid.Parse(session.Runtime.StreamIdentity.FactorySessionID); err != nil {
		t.Fatalf(
			"resumed successor canonical session ID = %q, want a preallocated UUID: %v",
			session.Runtime.StreamIdentity.FactorySessionID,
			err,
		)
	}
	if session.Runtime.StreamIdentity.StreamGenerationID == "" {
		t.Fatal("resumed successor stream generation ID is empty")
	}
}

func assertResumedSuccessorIdentityStable(
	t *testing.T,
	first, second factoryapi.FactorySession,
) {
	t.Helper()
	if first.Id != second.Id {
		t.Fatalf("resumed successor public session ID changed from %q to %q", first.Id, second.Id)
	}
	if first.Runtime.StreamIdentity == nil || second.Runtime.StreamIdentity == nil {
		t.Fatal("resumed successor identity disappeared between reads")
	}
	if first.Runtime.StreamIdentity.FactorySessionID != second.Runtime.StreamIdentity.FactorySessionID {
		t.Fatalf(
			"resumed successor canonical session ID changed from %q to %q",
			first.Runtime.StreamIdentity.FactorySessionID,
			second.Runtime.StreamIdentity.FactorySessionID,
		)
	}
	if first.Runtime.StreamIdentity.StreamGenerationID != second.Runtime.StreamIdentity.StreamGenerationID {
		t.Fatalf(
			"resumed successor stream generation ID changed from %q to %q",
			first.Runtime.StreamIdentity.StreamGenerationID,
			second.Runtime.StreamIdentity.StreamGenerationID,
		)
	}
}

func assertResumedHistoricalPrefix(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	ledger resumedResponseScopeLedger,
) {
	t.Helper()
	if len(events) < ledger.eventCount {
		t.Fatalf("public resumed event count = %d, want at least %d", len(events), ledger.eventCount)
	}
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.Id == "" {
			t.Fatal("public resumed Factory Event has empty ID")
		}
		if _, duplicate := seen[event.Id]; duplicate {
			t.Fatalf("public resumed Factory Events duplicate ID %q", event.Id)
		}
		seen[event.Id] = struct{}{}
	}
	for eventID := range ledger.eventIDs {
		if _, found := seen[eventID]; !found {
			t.Fatalf("public resumed Factory Events omitted historical event %q", eventID)
		}
	}
	if got := support.CountFactoryEvents(events, factoryapi.FactoryEventTypeSessionCompleted); got < ledger.sessionCompleted {
		t.Fatalf("public SESSION_COMPLETED count = %d, want at least %d", got, ledger.sessionCompleted)
	}
}

func assertResumedSuccessorLifecycle(
	t testing.TB,
	events []factoryapi.FactoryEvent,
	canonicalSessionID string,
) {
	t.Helper()
	var startedEvents, completedEvents []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Context.SessionId == nil || *event.Context.SessionId != canonicalSessionID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeSessionStarted:
			startedEvents = append(startedEvents, event)
		case factoryapi.FactoryEventTypeSessionCompleted:
			completedEvents = append(completedEvents, event)
		}
	}
	if len(startedEvents) != 1 || len(completedEvents) != 1 {
		t.Fatalf(
			"successor lifecycle events for %q = started:%d completed:%d, want exactly one of each",
			canonicalSessionID,
			len(startedEvents),
			len(completedEvents),
		)
	}
	started, err := startedEvents[0].Payload.AsSessionStartedEventPayload()
	if err != nil {
		t.Fatalf("decode successor SESSION_STARTED %q: %v", startedEvents[0].Id, err)
	}
	completed, err := completedEvents[0].Payload.AsSessionCompletedEventPayload()
	if err != nil {
		t.Fatalf("decode successor SESSION_COMPLETED %q: %v", completedEvents[0].Id, err)
	}
	wantDuration := completed.CompletedAt.Sub(started.StartedAt).Milliseconds()
	if wantDuration < 0 {
		t.Fatalf("successor lifecycle times are reversed: started=%s completed=%s", started.StartedAt, completed.CompletedAt)
	}
	if completed.DurationMillis == nil || *completed.DurationMillis != wantDuration {
		t.Fatalf(
			"successor lifecycle duration = %#v, want %d ms from %s to %s",
			completed.DurationMillis,
			wantDuration,
			started.StartedAt,
			completed.CompletedAt,
		)
	}
}

type resumedScopeSnapshot struct {
	workCount               int
	relationCount           int
	workIDsDigest           string
	workDigest              string
	canonicalEventCount     int
	canonicalEventIDsDigest string
	canonicalIdentityDigest string
	responseEventCount      int
	responseEventIDsDigest  string
	responseIdentityDigest  string
}

func captureOpenResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
) (resumedScopeSnapshot, []factoryapi.FactoryEvent, []factoryapi.FactoryResponseEvent) {
	t.Helper()
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	responseEvents := readOpenResumedResponseEvents(t, baseURL, sessionID)
	return buildResumedScopeSnapshot(t, baseURL, sessionID, events, responseEvents), events, responseEvents
}

func captureOpenResumedScopeSnapshotOnly(t testing.TB, baseURL, sessionID string) resumedScopeSnapshot {
	t.Helper()
	snapshot, _, _ := captureOpenResumedScopeSnapshot(t, baseURL, sessionID)
	return snapshot
}

func captureClosedResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
	responseEvents []factoryapi.FactoryResponseEvent,
) resumedScopeSnapshot {
	t.Helper()
	if responseEvents == nil {
		responseEvents = readClosedResumedResponseEvents(t, baseURL, sessionID)
	}
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	return buildResumedScopeSnapshot(t, baseURL, sessionID, events, responseEvents)
}

func buildResumedScopeSnapshot(
	t testing.TB,
	baseURL, sessionID string,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) resumedScopeSnapshot {
	t.Helper()
	works := readAllResumedSessionWork(t, baseURL, sessionID)
	workJSON := make([]json.RawMessage, 0, len(works))
	workIDs := make([]string, 0, len(works))
	relationCount := 0
	for _, item := range works {
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("marshal public Work snapshot: %v", err)
		}
		workJSON = append(workJSON, encoded)
		workIDs = append(workIDs, support.StringPointerValue(item.WorkId))
		relationCount += len(support.FactoryRelationsValue(item.Relations))
	}
	sort.Slice(workJSON, func(i, j int) bool { return string(workJSON[i]) < string(workJSON[j]) })
	sort.Strings(workIDs)
	return resumedScopeSnapshot{
		workCount:               len(works),
		relationCount:           relationCount,
		workIDsDigest:           digestResumedJSON(t, workIDs),
		workDigest:              digestResumedJSON(t, workJSON),
		canonicalEventCount:     len(events),
		canonicalEventIDsDigest: digestResumedJSON(t, resumedCanonicalEventIDs(events)),
		canonicalIdentityDigest: digestResumedJSON(t, resumedCanonicalEventIdentities(events)),
		responseEventCount:      len(responseEvents),
		responseEventIDsDigest:  digestResumedJSON(t, resumedResponseEventIDs(responseEvents)),
		responseIdentityDigest:  digestResumedJSON(t, resumedResponseEventIdentities(responseEvents)),
	}
}

func assertResumedScopeSnapshotUnchanged(t testing.TB, before, after resumedScopeSnapshot, boundary string) {
	t.Helper()
	if before != after {
		t.Fatalf("%s mutated public/canonical state: before=%#v after=%#v", boundary, before, after)
	}
}

func readAllResumedSessionWork(t testing.TB, baseURL, sessionID string) []factoryapi.Work {
	t.Helper()
	params := url.Values{}
	params.Set("includeSuperseded", "true")
	params.Set("maxResults", "1000")
	var results []factoryapi.Work
	for {
		page := support.GetJSON[factoryapi.ListWorkResponse](
			t,
			support.SessionWorkURL(baseURL, sessionID, "/work?"+params.Encode()),
		)
		results = append(results, page.Results...)
		if page.PaginationContext == nil || page.PaginationContext.NextToken == nil || strings.TrimSpace(*page.PaginationContext.NextToken) == "" {
			return results
		}
		params.Set("nextToken", *page.PaginationContext.NextToken)
	}
}

func resumedCanonicalEventIDs(events []factoryapi.FactoryEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.Id)
	}
	return ids
}

type resumedCanonicalEventIdentity struct {
	ID              string                      `json:"id"`
	Type            factoryapi.FactoryEventType `json:"type"`
	Sequence        int                         `json:"sequence"`
	SessionSequence *int                        `json:"sessionSequence,omitempty"`
}

func resumedCanonicalEventIdentities(events []factoryapi.FactoryEvent) []resumedCanonicalEventIdentity {
	identities := make([]resumedCanonicalEventIdentity, 0, len(events))
	for _, event := range events {
		identities = append(identities, resumedCanonicalEventIdentity{
			ID: event.Id, Type: event.Type, Sequence: event.Context.Sequence,
			SessionSequence: event.Context.SessionSequence,
		})
	}
	return identities
}

type resumedResponseEventIdentity struct {
	EventID    string                               `json:"eventId"`
	Sequence   int64                                `json:"sequence"`
	Kind       factoryapi.FactoryResponseEventKind  `json:"kind"`
	Phase      factoryapi.FactoryResponseEventPhase `json:"phase"`
	RunID      string                               `json:"runId"`
	DispatchID string                               `json:"dispatchId,omitempty"`
}

func resumedResponseEventIDs(events []factoryapi.FactoryResponseEvent) []string {
	ids := make([]string, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.EventId)
	}
	return ids
}

func resumedResponseEventIdentities(events []factoryapi.FactoryResponseEvent) []resumedResponseEventIdentity {
	identities := make([]resumedResponseEventIdentity, 0, len(events))
	for _, event := range events {
		identities = append(identities, resumedResponseEventIdentity{
			EventID: event.EventId, Sequence: event.Sequence, Kind: event.Kind,
			Phase: event.Phase, RunID: event.RunId,
			DispatchID: support.StringPointerValue(event.DispatchId),
		})
	}
	return identities
}

func digestResumedJSON(t testing.TB, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal resumed snapshot digest: %v", err)
	}
	digest := sha256.Sum256(encoded)
	return strings.ToUpper(hex.EncodeToString(digest[:]))
}

func readOpenResumedResponseEvents(
	t testing.TB,
	baseURL, sessionID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, sessionID),
	)
	defer stream.Close()
	var events []factoryapi.FactoryResponseEvent
	for len(events) < 4096 {
		result := stream.TryNextFrameResult(500 * time.Millisecond)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
		case support.FactoryResponseEventStreamOutcomeTimeout:
			return events
		case support.FactoryResponseEventStreamOutcomeEOF:
			t.Fatalf("resumed response-event stream closed while successor is live after %d frames", len(events))
		case support.FactoryResponseEventStreamOutcomeCanceled:
			t.Fatalf("resumed response-event stream canceled while successor is live")
		default:
			t.Fatalf("resumed response-event stream failed while successor is live: %s", result.Diagnostic())
		}
	}
	t.Fatal("resumed response-event open snapshot exceeded 4096 frames")
	return nil
}

func readClosedResumedResponseEvents(t testing.TB, baseURL, sessionID string) []factoryapi.FactoryResponseEvent {
	t.Helper()
	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(baseURL, sessionID),
	)
	defer stream.Close()
	var events []factoryapi.FactoryResponseEvent
	for len(events) < 4096 {
		result := stream.TryNextFrameResult(resumedResponseScopeStreamTimeout)
		switch result.Outcome {
		case support.FactoryResponseEventStreamOutcomeFrame:
			events = append(events, result.Frame.Event)
		case support.FactoryResponseEventStreamOutcomeEOF:
			return events
		case support.FactoryResponseEventStreamOutcomeTimeout:
			t.Fatalf("terminal resumed response-event stream did not close after %d frames", len(events))
		default:
			t.Fatalf("terminal resumed response-event stream failed: %s", result.Diagnostic())
		}
	}
	t.Fatal("terminal resumed response-event stream exceeded 4096 frames")
	return nil
}

func resumedResponseScopeCursor(event factoryapi.FactoryEvent) support.FactoryEventReadCursor {
	sequence := support.ReconnectSequenceForFactoryEvent(event)
	return support.FactoryEventReadCursor{AfterEventID: event.Id, AfterSequence: &sequence}
}

func lastResumedResponseSequence(events []factoryapi.FactoryResponseEvent) int64 {
	var last int64
	for _, event := range events {
		if event.Sequence > last {
			last = event.Sequence
		}
	}
	return last
}

func resumedResponseScopeWork(workID, name, payloadMarker string) factoryapi.Work {
	return resumedResponseScopeWorkOfType(resumedResponseScopeWorkType, workID, name, payloadMarker)
}

func resumedResponseScopeWorkOfType(workType, workID, name, payloadMarker string) factoryapi.Work {
	item := factoryapi.Work{
		Name:         name,
		WorkTypeName: &workType,
		Payload:      map[string]any{"marker": payloadMarker},
	}
	if workID != "" {
		item.WorkId = &workID
	}
	return item
}

func resumedResponseScopeGeneratedWorkID(ledger resumedResponseScopeLedger, workType string) string {
	return fmt.Sprintf("work-%s-%d", workType, ledger.historicalWorkMaxima[workType]+1)
}

func assertReservedWorkReadable(t testing.TB, baseURL, sessionID string, reserved factoryapi.Work) {
	t.Helper()
	got := support.GetJSON[factoryapi.Work](
		t,
		support.SessionWorkURL(baseURL, sessionID, "/work/"+url.PathEscape(resumedResponseScopeReservedWorkID)),
	)
	if support.StringPointerValue(got.WorkId) != resumedResponseScopeReservedWorkID {
		t.Fatalf("reserved historical Work ID = %q, want %q", support.StringPointerValue(got.WorkId), resumedResponseScopeReservedWorkID)
	}
	if support.StringPointerValue(got.WorkTypeName) != support.StringPointerValue(reserved.WorkTypeName) {
		t.Fatalf("reserved historical Work type = %q, want %q", support.StringPointerValue(got.WorkTypeName), support.StringPointerValue(reserved.WorkTypeName))
	}
}

func putResumedWorkRequest(t testing.TB, baseURL, sessionID string, request factoryapi.WorkRequest) (int, []byte) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal resumed Work request: %v", err)
	}
	endpoint := support.SessionWorkURL(
		baseURL,
		sessionID,
		"/work-requests/"+url.PathEscape(request.RequestId),
	)
	httpRequest, err := http.NewRequestWithContext(t.Context(), http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build resumed Work request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT resumed Work request: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resumed Work request response: %v", err)
	}
	return response.StatusCode, responseBody
}

func resumedInlineJSON(t testing.TB, request factoryapi.WorkRequest) string {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal CLI resumed Work request: %v", err)
	}
	return string(encoded)
}

func assertWorkRequestConflict(t testing.TB, status int, body []byte, boundary string) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("%s Work conflict status = %d, want 409: %s", boundary, status, strings.TrimSpace(string(body)))
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode %s Work conflict: %v", boundary, err)
	}
	if response.Code != factoryapi.ErrorResponseCodeCONFLICT || response.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("%s Work conflict = %#v, want CONFLICT/CONFLICT", boundary, response)
	}
	if strings.Contains(string(body), resumedResponseScopeSecretMarker) {
		t.Fatalf("%s Work conflict echoed untrusted payload marker", boundary)
	}
}

func executeResumedResponseScopeCLI(
	t testing.TB,
	server *support.FunctionalAPIServer,
	args []string,
) (string, string, error) {
	t.Helper()
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = home
	inputs.Input.Stdin = strings.NewReader("")
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	err := server.Execute(t, inputs.Input)
	return inputs.Stdout(), inputs.Stderr(), err
}

func readResumedFutureEventsUntilWorkTerminal(
	t testing.TB,
	stream *support.FactoryEventStream,
	workID string,
) []factoryapi.FactoryEvent {
	t.Helper()
	var events []factoryapi.FactoryEvent
	for len(events) < 4096 {
		event, ok := stream.TryNextEvent(resumedResponseScopeStreamTimeout)
		if !ok {
			t.Fatalf("timed out waiting for factory event stream payload within %s after %d events", resumedResponseScopeStreamTimeout, len(events))
		}
		events = append(events, event)
		if event.Type != factoryapi.FactoryEventTypeWorkStateChange {
			if event.Type == factoryapi.FactoryEventTypeDispatchResponse && resumedEventContainsWork(event, workID) {
				payload, err := event.Payload.AsDispatchResponseEventPayload()
				if err != nil {
					t.Fatalf("decode generated terminal dispatch response %q: %v", event.Id, err)
				}
				if payload.TransitionId == "validate" && payload.Outcome == factoryapi.WorkOutcomeAccepted {
					return events
				}
			}
			continue
		}
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		if err != nil {
			t.Fatalf("decode generated Work state event %q: %v", event.Id, err)
		}
		if payload.WorkId != workID {
			continue
		}
		if payload.ToState == "complete" || payload.ToState == "failed" {
			// Keep the canonical Work-state transition in the progress evidence;
			// the public Work projection may still lag the dispatch response, so
			// the caller also waits on that customer-visible read model.
			return events
		}
	}
	t.Fatalf("read %d resumed Factory Events without generated Work %q terminal state", len(events), workID)
	return nil
}

func waitForResumedWorkTerminal(
	t testing.TB,
	baseURL, sessionID, workID string,
) factoryapi.Work {
	t.Helper()
	deadline := time.NewTimer(resumedResponseScopeStreamTimeout)
	defer deadline.Stop()
	// The canonical stream exposes dispatch completion, but the public Work
	// projection has no separate readiness signal and can lag that event. Read
	// the customer-visible Work endpoint until its typed terminal state appears;
	// the timer is only a failure ceiling, not a fixed completion delay.
	poll := time.NewTicker(25 * time.Millisecond)
	defer poll.Stop()
	for {
		work := support.GetJSON[factoryapi.Work](
			t,
			support.SessionWorkURL(baseURL, sessionID, "/work/"+url.PathEscape(workID)),
		)
		if work.State != nil &&
			(work.State.Type == factoryapi.WorkStateTypeTERMINAL || work.State.Type == factoryapi.WorkStateTypeFAILED) {
			return work
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			t.Fatalf("generated Work %q did not reach a public terminal state within %s: %#v", workID, resumedResponseScopeStreamTimeout, work.State)
		}
	}
}

func assertGeneratedWorkCanonicalProgress(t testing.TB, events []factoryapi.FactoryEvent, workID string) string {
	t.Helper()
	workRequestSeen := false
	workProgressSeen := false
	var dispatchID string
	for _, event := range events {
		if !resumedEventContainsWork(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			workRequestSeen = true
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, err := event.Payload.AsWorkStateChangeEventPayload()
			if err != nil {
				t.Fatalf("decode generated Work progress event %q: %v", event.Id, err)
			}
			if payload.WorkId == workID && payload.FromState != payload.ToState {
				workProgressSeen = true
			}
		case factoryapi.FactoryEventTypeDispatchRequest,
			factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
			factoryapi.FactoryEventTypeScriptRequest,
			factoryapi.FactoryEventTypeScriptResponse,
			factoryapi.FactoryEventTypeAgentRunResponse,
			factoryapi.FactoryEventTypeDispatchResponse:
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				workProgressSeen = true
			}
			if event.Context.DispatchId != nil && *event.Context.DispatchId != "" {
				dispatchID = *event.Context.DispatchId
			}
		}
	}
	if !workRequestSeen {
		t.Fatalf("generated Work %q has no public WORK_REQUEST event", workID)
	}
	if !workProgressSeen {
		t.Fatalf("generated Work %q has no public dispatch progress", workID)
	}
	if dispatchID == "" {
		t.Fatalf("generated Work %q has no correlated public dispatch", workID)
	}
	return dispatchID
}

func resumedEventContainsWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func readResumedResponseEventsUntilDispatchTerminal(
	t testing.TB,
	stream *support.FactoryResponseEventStream,
	dispatchID string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	var events []factoryapi.FactoryResponseEvent
	for len(events) < 4096 {
		result := stream.TryNextFrameResult(resumedResponseScopeStreamTimeout)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("read generated Response Events: %s", result.Diagnostic())
		}
		event := result.Frame.Event
		if support.StringPointerValue(event.DispatchId) != dispatchID {
			continue
		}
		events = append(events, event)
		if resumedResponseEventIsTerminal(event) {
			return events
		}
	}
	t.Fatalf("read 4096 Response Events without terminal dispatch %q", dispatchID)
	return nil
}

func resumedResponseEventIsTerminal(event factoryapi.FactoryResponseEvent) bool {
	if event.Kind == factoryapi.FactoryResponseEventKindRun {
		return event.Phase == factoryapi.FactoryResponseEventPhaseCompleted ||
			event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled
	}
	return event.Kind == factoryapi.FactoryResponseEventKindError &&
		(event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
			event.Phase == factoryapi.FactoryResponseEventPhaseCanceled)
}

func assertGeneratedWorkResponseProgress(
	t testing.TB,
	events []factoryapi.FactoryResponseEvent,
	dispatchID, sessionID string,
) {
	t.Helper()
	if len(events) < 2 {
		t.Fatalf("generated Response Events for dispatch %q = %d, want progress and terminal", dispatchID, len(events))
	}
	progress := false
	terminal := false
	for _, event := range events {
		if event.FactorySessionId != sessionID || support.StringPointerValue(event.DispatchId) != dispatchID {
			t.Fatalf("generated Response Event correlation = %#v, want session %q dispatch %q", event, sessionID, dispatchID)
		}
		if !resumedResponseEventIsTerminal(event) {
			progress = true
		}
		if resumedResponseEventIsTerminal(event) {
			terminal = true
		}
	}
	if !progress || !terminal {
		t.Fatalf("generated Response Events progress=%t terminal=%t events=%#v", progress, terminal, events)
	}
}

func assertGeneratedWorkTerminal(t testing.TB, item factoryapi.Work, workID string) {
	t.Helper()
	if support.StringPointerValue(item.WorkId) != workID || item.State == nil {
		t.Fatalf("generated Work = %#v, want terminal Work %q", item, workID)
	}
	if item.State.Type != factoryapi.WorkStateTypeTERMINAL && item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("generated Work %q state = %#v, want terminal or failed", workID, item.State)
	}
}

func assertGeneratedSuffixesAboveHistory(
	t testing.TB,
	baseURL, sessionID string,
	ledger resumedResponseScopeLedger,
) {
	t.Helper()
	works := readAllResumedSessionWork(t, baseURL, sessionID)
	newWorkCount := 0
	for _, item := range works {
		workID := support.StringPointerValue(item.WorkId)
		match := resumedResponseScopeWorkIDPattern.FindStringSubmatch(workID)
		if len(match) != 3 {
			continue
		}
		value, err := strconv.Atoi(match[2])
		if err != nil {
			t.Fatalf("parse public generated Work suffix %q: %v", workID, err)
		}
		if _, historical := ledger.historicalWorkIDs[workID]; historical {
			continue
		}
		newWorkCount++
		if value <= ledger.historicalWorkMaxima[match[1]] {
			t.Fatalf("generated Work %q suffix = %d, historical %s maximum = %d", workID, value, match[1], ledger.historicalWorkMaxima[match[1]])
		}
	}
	if newWorkCount == 0 {
		t.Fatal("public Work board has no generated higher-suffix Work")
	}
}

func assertResumedResponseScopeNoWarnings(
	t testing.TB,
	canonicalEvents []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	for _, value := range []any{canonicalEvents, responseEvents} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal resumed warning scan: %v", err)
		}
		text := string(encoded)
		if strings.Contains(text, "CANONICAL_EVENT_PUBLISH_FAILED") || strings.Contains(strings.ToLower(text), "publication-complete") {
			t.Fatalf("resumed successor emitted a publication warning")
		}
	}
}

func postResumedLifecycleControl(t testing.TB, baseURL, sessionID, operation string) (int, []byte) {
	t.Helper()
	body := bytes.NewReader([]byte(`{}`))
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/" + operation
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("build resumed lifecycle control: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST resumed lifecycle control: %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read resumed lifecycle control: %v", err)
	}
	return response.StatusCode, responseBody
}

func assertTerminalResumedSession(t testing.TB, session factoryapi.FactorySession) {
	t.Helper()
	if session.Runtime.LifecycleControlStatus == nil {
		t.Fatalf("terminal resumed session has no lifecycle status: %#v", session)
	}
	switch *session.Runtime.LifecycleControlStatus {
	case factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		factoryapi.FactorySessionDurableLifecycleStatusFailed,
		factoryapi.FactorySessionDurableLifecycleStatusCanceled,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted,
		factoryapi.FactorySessionDurableLifecycleStatusTimedOut,
		factoryapi.FactorySessionDurableLifecycleStatusTerminated:
	default:
		t.Fatalf("resumed successor lifecycle status = %q, want terminal", *session.Runtime.LifecycleControlStatus)
	}
}

func assertResumedResponseEventOrder(
	t testing.TB,
	events []factoryapi.FactoryResponseEvent,
	sessionID, dispatchID string,
) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("terminal response-event replay is empty")
	}
	seen := make(map[string]struct{}, len(events))
	var previousSequence int64
	matchingDispatch := false
	for index, event := range events {
		if event.EventId == "" || event.FactorySessionId != sessionID {
			t.Fatalf("terminal Response Event %d = %#v, want stable session/event identity", index, event)
		}
		if _, duplicate := seen[event.EventId]; duplicate {
			t.Fatalf("terminal Response Events duplicate ID %q", event.EventId)
		}
		seen[event.EventId] = struct{}{}
		if index > 0 && event.Sequence <= previousSequence {
			t.Fatalf("terminal Response Event sequence %d after %d is not strictly increasing", event.Sequence, previousSequence)
		}
		previousSequence = event.Sequence
		if support.StringPointerValue(event.DispatchId) == dispatchID {
			matchingDispatch = true
		}
	}
	if !matchingDispatch {
		t.Fatalf("terminal Response Event replay omitted dispatch %q", dispatchID)
	}
}

func assertTerminalLifecycleConflict(t testing.TB, status int, body []byte, boundary string) {
	t.Helper()
	if status != http.StatusConflict {
		t.Fatalf("%s terminal lifecycle status = %d, want 409: %s", boundary, status, strings.TrimSpace(string(body)))
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode %s terminal lifecycle conflict: %v", boundary, err)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("%s terminal lifecycle response = %#v, want TERMINAL_SESSION", boundary, response)
	}
}

func assertRecordedSuccessorExists(t testing.TB, successorPath string) {
	t.Helper()
	stat, err := os.Stat(successorPath)
	if err != nil {
		t.Fatalf("successor recording was not created: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatal("successor recording is empty")
	}
}

type resumedResponseScopeProviderRunner struct {
	gate         chan struct{}
	release      sync.Once
	mu           sync.Mutex
	requests     []platformprocess.CommandRequest
	callSignal   chan struct{}
	returnSignal chan struct{}
	returns      int
}

func newResumedResponseScopeProviderRunner(gate chan struct{}) *resumedResponseScopeProviderRunner {
	return &resumedResponseScopeProviderRunner{
		gate: gate, callSignal: make(chan struct{}, 128), returnSignal: make(chan struct{}, 128),
	}
}

func (runner *resumedResponseScopeProviderRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneResumedCommandRequest(request))
	runner.mu.Unlock()
	select {
	case runner.callSignal <- struct{}{}:
	default:
	}
	if !strings.EqualFold(strings.TrimSpace(request.Command), string(modelprovider.ProviderCodex)) {
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected provider command %q", request.Command)
	}
	result := platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(resumedResponseScopeControlledOutput)}
	if !bytes.Contains(request.Stdin, []byte(resumedResponseScopeGeneratedName)) &&
		!bytes.Contains(request.Stdin, []byte(resumedResponseScopeGeneratedMarker)) {
		runner.recordReturn()
		return result, nil
	}
	select {
	case <-runner.gate:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	runner.recordReturn()
	return result, nil
}

func (runner *resumedResponseScopeProviderRunner) WaitForCall(ctx context.Context) error {
	for {
		runner.mu.Lock()
		calls := len(runner.requests)
		runner.mu.Unlock()
		if calls > 0 {
			return nil
		}
		select {
		case <-runner.callSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *resumedResponseScopeProviderRunner) WaitForCallCount(ctx context.Context, want int) error {
	for {
		if runner.CallCount() >= want {
			return nil
		}
		select {
		case <-runner.callSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) ReturnCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.returns
}

func (runner *resumedResponseScopeProviderRunner) WaitForReturnCount(ctx context.Context, want int) error {
	for {
		if runner.ReturnCount() >= want {
			return nil
		}
		select {
		case <-runner.returnSignal:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (runner *resumedResponseScopeProviderRunner) recordReturn() {
	runner.mu.Lock()
	runner.returns++
	runner.mu.Unlock()
	select {
	case runner.returnSignal <- struct{}{}:
	default:
	}
}

func (runner *resumedResponseScopeProviderRunner) WorkDirs() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	result := make([]string, 0, len(runner.requests))
	for _, request := range runner.requests {
		result = append(result, request.WorkDir)
	}
	return result
}

func (runner *resumedResponseScopeProviderRunner) Release() {
	runner.release.Do(func() { close(runner.gate) })
}

func (runner *resumedResponseScopeProviderRunner) AssertSafeRequests(t testing.TB) {
	t.Helper()
	runner.mu.Lock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index := range runner.requests {
		requests[index] = cloneResumedCommandRequest(runner.requests[index])
	}
	runner.mu.Unlock()
	if len(requests) == 0 {
		t.Fatal("controlled ProviderCommandRunner observed no requests")
	}
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatalf("marshal controlled provider request: %v", err)
		}
		if strings.Contains(string(encoded), resumedResponseScopeSecretMarker) {
			t.Fatalf("controlled provider request leaked rejected Work payload marker")
		}
	}
}

func cloneResumedCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*resumedResponseScopeProviderRunner)(nil)
