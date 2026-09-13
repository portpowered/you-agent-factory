package tts_clean_install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const helperReadinessTimeout = 10 * time.Second

func requireWindowsProcessBoundary(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("TTS OS-boundary evidence requires Windows")
	}
	if _, err := verifiedHelperPath(); err != nil {
		t.Fatalf("verified deterministic helper: %v", err)
	}
	if _, err := verifiedPowerShellPath(); err != nil {
		t.Fatalf("verified PowerShell host: %v", err)
	}
}

func TestI01DynamicOwnedListenerAndI05SentinelIsolation(t *testing.T) {
	requireWindowsProcessBoundary(t)
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}

	sentinel, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("start unrelated sentinel listener: %v", err)
	}
	defer sentinel.Close()
	sentinelAddress := sentinel.Addr().String()
	sentinelPort := sentinel.Addr().(*net.TCPAddr).Port
	if sentinelPort == forbiddenListenerPort {
		t.Fatalf("unrelated sentinel bound forbidden port %d", forbiddenListenerPort)
	}

	run, err := startOwnedLoopbackListener(preflight.Plan)
	if err != nil {
		t.Fatalf("start owned listener helper: %v", err)
	}
	defer stopUnfinishedHelper(run)
	readyContext, cancel := context.WithTimeout(context.Background(), helperReadinessTimeout)
	ready, err := run.WaitReady(readyContext, "listener-ready")
	cancel()
	if err != nil {
		t.Fatalf("listener readiness: %v", err)
	}
	if ready.Address != "127.0.0.1" || ready.Port <= 0 || ready.Port == forbiddenListenerPort {
		t.Fatalf("listener readiness = %#v, want IPv4 dynamic non-%d listener", ready, forbiddenListenerPort)
	}
	ownedAddress := net.JoinHostPort(ready.Address, strconv.Itoa(ready.Port))
	if err := dialAddress(ownedAddress); err != nil {
		t.Fatalf("owned listener was not reachable before cleanup: %v", err)
	}
	if !processAlive(ready.PID) {
		t.Fatalf("owned listener process %d was not alive at readiness", ready.PID)
	}
	if err := run.Stop(); err != nil {
		t.Fatalf("stop owned listener helper: %v", err)
	}
	result, err := run.Wait(context.Background())
	if err != nil {
		t.Fatalf("wait owned listener helper: %v", err)
	}
	if result.OwnedSurvivors != 0 || processAlive(ready.PID) {
		t.Fatalf("owned listener cleanup = result=%+v alive=%t, want zero survivors", result, processAlive(ready.PID))
	}
	if !listenerUnavailable(ownedAddress) {
		t.Fatalf("owned listener %s remained reachable after cleanup", ownedAddress)
	}
	if err := dialAddress(sentinelAddress); err != nil {
		t.Fatalf("unrelated sentinel listener was affected by owned cleanup: %v", err)
	}
	if _, err := cleanupOwnedRoots(preflight.Plan); err != nil {
		t.Fatalf("cleanup listener roots: %v", err)
	}
}

func TestI02ControlledJourneyCrossesHelperProcessBoundary(t *testing.T) {
	requireWindowsProcessBoundary(t)
	f := newFixture(t)
	result, observation, report := runControlledJourney(t, f, "")
	if result.ExitCode != 0 || result.TimedOut || result.Cancelled {
		t.Fatalf("controlled journey process = %+v, want successful completion", result)
	}
	if result.StderrBytes != 0 || result.StdoutBytes == 0 {
		t.Fatalf("controlled journey streams = stdout=%d stderr=%d, want event output and no stderr", result.StdoutBytes, result.StderrBytes)
	}
	if report.Verdict != journeyInconclusive {
		t.Fatalf("controlled journey verdict = %q, want INCONCLUSIVE for real TTS edge", report.Verdict)
	}
	if len(observation.Steps) != len(journeyStepOrder) || len(report.Commands) != len(journeyStepOrder) {
		t.Fatalf("controlled journey shape = steps=%d commands=%d, want %d each", len(observation.Steps), len(report.Commands), len(journeyStepOrder))
	}
	if report.Network.Attempts != 0 || report.Network.WarmOfflineAttempts != 0 {
		t.Fatalf("controlled journey network = %#v, want zero attempts", report.Network)
	}
	if report.Cleanup.SurvivingProcesses != 0 || report.Cleanup.SurvivingListeners != 0 || report.Cleanup.RemovedRuntimeRoots == 0 {
		t.Fatalf("controlled journey cleanup = %#v, want zero survivors and removed roots", report.Cleanup)
	}
	if report.Journeys.WarmOffline.Status != journeyPass || report.Readiness[1].CacheBytes <= 0 || !report.Readiness[1].CacheReused {
		t.Fatalf("warm evidence = phase=%#v readiness=%#v", report.Journeys.WarmOffline, report.Readiness)
	}
	if err := WriteReportAtomic(f.invocation.ReportPath, report, nil); err != nil {
		t.Fatalf("publish controlled journey report: %v", err)
	}
	body, err := os.ReadFile(f.invocation.ReportPath)
	if err != nil {
		t.Fatalf("read controlled journey report: %v", err)
	}
	var published Report
	if err := json.Unmarshal(body, &published); err != nil {
		t.Fatalf("decode controlled journey report: %v", err)
	}
	if err := ValidateReport(published); err != nil {
		t.Fatalf("published controlled journey report: %v", err)
	}
	if bytes.Contains(body, []byte(f.root)) || bytes.Contains(body, []byte("PowerShell")) {
		t.Fatalf("published report contains ambient helper details: %s", body)
	}

	fresh := newFixture(t)
	if _, err := Preflight(fresh.invocation); err != nil {
		t.Fatalf("fresh empty-root preflight after journey cleanup: %v", err)
	}
}

func TestI03ConcurrentPipeDrainCapturesExactStreams(t *testing.T) {
	requireWindowsProcessBoundary(t)
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	run, err := startHelper(preflight.Plan, "burst", true, "-OutputBytes", strconv.Itoa(helperOutputBytes))
	if err != nil {
		t.Fatalf("start burst helper: %v", err)
	}
	defer stopUnfinishedHelper(run)
	waitContext, cancel := context.WithTimeout(context.Background(), helperReadinessTimeout)
	result, err := run.Wait(waitContext)
	cancel()
	if err != nil {
		t.Fatalf("wait burst helper: %v", err)
	}
	if result.ExitCode != 0 || result.TimedOut || result.Cancelled {
		t.Fatalf("burst helper result = %+v, want exit 0 without timeout/cancellation", result)
	}
	want := expectedBurstBytes(helperOutputBytes)
	if !bytes.Equal(result.Stdout, want) || !bytes.Equal(result.Stderr, want) {
		t.Fatalf("burst streams were not captured exactly: stdout=%d/%d stderr=%d/%d", len(result.Stdout), len(want), len(result.Stderr), len(want))
	}
	if result.StdoutSHA256 != sha256Hex(want) || result.StderrSHA256 != sha256Hex(want) {
		t.Fatalf("burst hashes = stdout=%s stderr=%s, want %s", result.StdoutSHA256, result.StderrSHA256, sha256Hex(want))
	}
	if result.StdoutBytes != int64(len(want)) || result.StderrBytes != int64(len(want)) {
		t.Fatalf("burst byte counts = stdout=%d stderr=%d, want %d", result.StdoutBytes, result.StderrBytes, len(want))
	}
}

func TestI04TimeoutAndCancellationTerminateOnlyRecordedTree(t *testing.T) {
	requireWindowsProcessBoundary(t)
	cases := []struct {
		name       string
		cancelTree bool
	}{
		{name: "timeout", cancelTree: false},
		{name: "explicit cancellation", cancelTree: true},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			f := newFixture(t)
			preflight, err := Preflight(f.invocation)
			if err != nil {
				t.Fatalf("Preflight: %v", err)
			}
			sentinelReadyPath := filepath.Join(f.outputRoot, "sentinel.ready")
			sentinel, err := startHelper(preflight.Plan, "sentinel", false, "-ReadyPath", sentinelReadyPath)
			if err != nil {
				t.Fatalf("start unrelated sentinel process: %v", err)
			}
			defer stopUnfinishedHelper(sentinel)
			readyContext, cancelReady := context.WithTimeout(context.Background(), helperReadinessTimeout)
			sentinelEvent, err := sentinel.WaitReady(readyContext, "sentinel-ready")
			cancelReady()
			if err != nil {
				t.Fatalf("sentinel readiness: %v", err)
			}
			if !processAlive(sentinelEvent.PID) {
				t.Fatalf("sentinel process %d was not alive at readiness", sentinelEvent.PID)
			}

			readyPath := filepath.Join(f.outputRoot, "tree.ready")
			descendantReadyPath := filepath.Join(f.outputRoot, "descendant.ready")
			run, err := startHelper(preflight.Plan, "tree", true, "-ReadyPath", readyPath, "-DescendantReadyPath", descendantReadyPath)
			if err != nil {
				t.Fatalf("start owned tree helper: %v", err)
			}
			defer stopUnfinishedHelper(run)
			readyContext, cancelReady = context.WithTimeout(context.Background(), helperReadinessTimeout)
			ready, err := run.WaitReady(readyContext, "ready")
			cancelReady()
			if err != nil {
				t.Fatalf("owned tree readiness: %v", err)
			}
			if ready.PID <= 0 || ready.DescendantPID <= 0 || !processAlive(ready.PID) || !processAlive(ready.DescendantPID) {
				t.Fatalf("owned tree readiness = %#v, want live root and descendant", ready)
			}

			var waitContext context.Context
			var cancelWait context.CancelFunc
			if testCase.cancelTree {
				waitContext, cancelWait = context.WithCancel(context.Background())
				cancelWait()
			} else {
				waitContext, cancelWait = context.WithTimeout(context.Background(), 500*time.Millisecond)
			}
			result, err := run.Wait(waitContext)
			cancelWait()
			if err != nil {
				t.Fatalf("owned tree termination: %v", err)
			}
			if testCase.cancelTree && !result.Cancelled {
				t.Fatalf("explicit cancellation result = %+v, want cancelled", result)
			}
			if !testCase.cancelTree && !result.TimedOut {
				t.Fatalf("timeout result = %+v, want timed out", result)
			}
			if result.OwnedSurvivors != 0 || processAlive(ready.PID) || processAlive(ready.DescendantPID) {
				t.Fatalf("owned tree survivors: result=%+v root=%t descendant=%t", result, processAlive(ready.PID), processAlive(ready.DescendantPID))
			}
			if !processAlive(sentinelEvent.PID) {
				t.Fatalf("unrelated sentinel process %d did not survive owned cleanup", sentinelEvent.PID)
			}

			if err := sentinel.Stop(); err != nil {
				t.Fatalf("stop sentinel: %v", err)
			}
			if _, err := sentinel.Wait(context.Background()); err != nil {
				t.Fatalf("wait sentinel: %v", err)
			}
			for _, path := range []string{sentinelReadyPath, readyPath, descendantReadyPath} {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("remove readiness file %s: %v", path, err)
				}
			}
			if _, err := cleanupOwnedRoots(preflight.Plan); err != nil {
				t.Fatalf("cleanup owned tree roots: %v", err)
			}
		})
	}
}

func TestI06I07I08FailureEvidenceRemainsComplete(t *testing.T) {
	requireWindowsProcessBoundary(t)
	f := newFixture(t)
	_, observation, report := runControlledJourney(t, f, "")
	if report.Verdict != journeyInconclusive {
		t.Fatalf("baseline controlled report = %q, want INCONCLUSIVE", report.Verdict)
	}

	t.Run("invalid audio blocks pass", func(t *testing.T) {
		invalid := observation
		invalid.Audio = append([]AudioArtifact(nil), observation.Audio...)
		invalid.Audio[0] = AudioArtifact{Name: "cold-tts.wav", MediaType: "audio/wav", Data: makePCM16WAV([]int16{0, 0, 0}, 8000)}
		failure, err := BuildControlledJourneyReport(planForObservation(f, t), invalid)
		if err != nil {
			t.Fatalf("BuildControlledJourneyReport invalid audio: %v", err)
		}
		if failure.Verdict != journeyFail || !findingForTest(failure, "audio-cold-tts.wav") {
			t.Fatalf("invalid audio report = verdict %q findings=%#v, want FAIL audio finding", failure.Verdict, failure.Findings)
		}
		if err := ValidateReport(failure); err != nil {
			t.Fatalf("invalid audio report validation: %v", err)
		}
	})

	t.Run("warm network blocks pass", func(t *testing.T) {
		invalid := observation
		invalid.Network.WarmOfflineAttempts = 1
		failure, err := BuildControlledJourneyReport(planForObservation(f, t), invalid)
		if err != nil {
			t.Fatalf("BuildControlledJourneyReport warm network: %v", err)
		}
		if failure.Verdict != journeyFail || !findingForTest(failure, "warm-offline-network") {
			t.Fatalf("warm network report = verdict %q findings=%#v, want FAIL network finding", failure.Verdict, failure.Findings)
		}
		if err := ValidateReport(failure); err != nil {
			t.Fatalf("warm network report validation: %v", err)
		}
	})

	t.Run("post preflight failure cleans and permits retry", func(t *testing.T) {
		failureFixture := newFixture(t)
		result, failedObservation, failure := runControlledJourney(t, failureFixture, JourneyStepModelsInspect)
		if result.ExitCode != 17 || failure.Verdict != journeyFail {
			t.Fatalf("controlled phase failure = result=%+v verdict=%q, want exit 17/FAIL", result, failure.Verdict)
		}
		if failedObservation.Cleanup.SurvivingProcesses != 0 || failure.Cleanup.RemovedRuntimeRoots == 0 {
			t.Fatalf("failure cleanup = observation=%#v report=%#v, want zero survivors and removed roots", failedObservation.Cleanup, failure.Cleanup)
		}
		if !findingForTest(failure, "discovery") {
			t.Fatalf("failure findings = %#v, want failed discovery phase", failure.Findings)
		}
		if err := WriteReportAtomic(failureFixture.invocation.ReportPath, failure, nil); err != nil {
			t.Fatalf("publish failure report: %v", err)
		}
		fresh := newFixture(t)
		if _, err := Preflight(fresh.invocation); err != nil {
			t.Fatalf("fresh retry after failed journey cleanup: %v", err)
		}
	})
}

func runControlledJourney(t *testing.T, f *fixture, failPhase string) (helperResult, JourneyObservation, Report) {
	t.Helper()
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	arguments := []string{
		"-OutputRoot", preflight.Plan.Isolation.OutputRoot,
		"-ArtifactPath", preflight.Plan.Manifest.Build.ArtifactPath,
		"-Offline",
	}
	if failPhase != "" {
		arguments = append(arguments, "-FailPhase", failPhase)
	}
	run, err := startHelper(preflight.Plan, "journey", true, arguments...)
	if err != nil {
		t.Fatalf("start controlled journey helper: %v", err)
	}
	defer stopUnfinishedHelper(run)
	waitContext, cancel := context.WithTimeout(context.Background(), helperReadinessTimeout)
	result, err := run.Wait(waitContext)
	cancel()
	if err != nil {
		t.Fatalf("wait controlled journey helper: %v", err)
	}
	wantExit := 0
	if failPhase != "" {
		wantExit = 17
	}
	if result.ExitCode != wantExit {
		t.Fatalf("controlled journey helper exit=%d, want %d; stdout=%s stderr=%s", result.ExitCode, wantExit, result.Stdout, result.Stderr)
	}

	observation := observationFromHelper(t, preflight.Plan, result)
	cleanup, err := cleanupOwnedRoots(preflight.Plan)
	if err != nil {
		t.Fatalf("cleanup controlled journey roots: %v", err)
	}
	cleanup.OwnedProcesses = 1
	cleanup.SurvivingProcesses = 0
	cleanup.SurvivingListeners = 0
	observation.Cleanup = cleanup
	report, err := BuildControlledJourneyReport(preflight.Plan, observation)
	if err != nil {
		t.Fatalf("BuildControlledJourneyReport: %v", err)
	}
	return result, observation, report
}

func observationFromHelper(t *testing.T, plan SealedPlan, result helperResult) JourneyObservation {
	t.Helper()
	byPhase := make(map[string]helperEvent, len(result.Events))
	for _, event := range result.Events {
		if event.Event == "phase" {
			if _, exists := byPhase[event.Phase]; exists {
				t.Fatalf("helper emitted duplicate phase %q", event.Phase)
			}
			byPhase[event.Phase] = event
		}
	}
	steps := make([]JourneyStep, 0, len(journeyStepOrder))
	commands := make([]CommandEvidence, 0, len(byPhase))
	readiness := make([]ReadinessEvidence, 0, 2)
	network := NetworkEvidence{Policy: "none"}
	for _, phase := range journeyStepOrder {
		event, ok := byPhase[phase]
		status := journeyInconclusive
		evidence := []string{"not run after controlled phase failure"}
		if ok {
			status = event.Status
			evidence = append([]string(nil), event.Evidence...)
			if len(evidence) == 0 {
				evidence = []string{phase + " emitted no textual detail"}
			}
			commands = append(commands, CommandEvidence{
				Phase: phase, Argv: append([]string(nil), event.Argv...), ExitCode: phaseExitCode(event, result),
				StartedAt:   result.StartedAt.UTC().Format(time.RFC3339Nano),
				EndedAt:     result.EndedAt.UTC().Format(time.RFC3339Nano),
				StdoutBytes: event.lineBytes, StderrBytes: 0,
				StdoutSHA256: event.lineSHA256, StderrSHA256: sha256Hex(nil),
				TimedOut: result.TimedOut, Cancelled: result.Cancelled,
			})
			if event.ReadinessState != "" {
				readiness = append(readiness, ReadinessEvidence{
					Phase: phaseNameForReadiness(phase), ReadinessState: event.ReadinessState,
					LifecycleState: event.LifecycleState, CacheBytes: event.CacheBytes,
					CacheReused: event.CacheReused,
				})
			}
			network.Attempts += event.NetworkAttempts
		}
		steps = append(steps, JourneyStep{Name: phase, Status: status, Evidence: evidence})
	}

	observation := JourneyObservation{Steps: steps, Commands: commands, Readiness: readiness, Network: network}
	if _, ok := byPhase[JourneyStepInstalledIdentity]; ok {
		artifactInfo, err := os.Stat(plan.Manifest.Build.ArtifactPath)
		if err != nil {
			t.Fatalf("stat sealed artifact for installed identity: %v", err)
		}
		observation.InstalledIdentity = &Identity{
			Name: "installed-build", Identity: plan.Manifest.Build.Identity,
			Path:  pathIdentity(filepath.Join(plan.Isolation.WorkRoot, "installed-build.bin")),
			Bytes: artifactInfo.Size(), SHA256: plan.Manifest.Build.SHA256,
		}
	}
	for _, audio := range []struct {
		name string
		path string
	}{
		{name: "cold-tts.wav", path: filepath.Join(plan.Isolation.RuntimeRoot, "audio", "cold-tts.wav")},
		{name: "warm-offline-tts.wav", path: filepath.Join(plan.Isolation.RuntimeRoot, "audio", "warm-offline-tts.wav")},
	} {
		data, err := os.ReadFile(audio.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read helper audio %s: %v", audio.name, err)
		}
		observation.Audio = append(observation.Audio, AudioArtifact{Name: audio.name, MediaType: "audio/wav", Data: data})
	}
	return observation
}

func phaseExitCode(event helperEvent, result helperResult) int {
	if event.Status == journeyFail {
		return result.ExitCode
	}
	return 0
}

func phaseNameForReadiness(phase string) string {
	return strings.TrimSuffix(phase, "-readiness")
}

func planForObservation(f *fixture, t *testing.T) SealedPlan {
	t.Helper()
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight for derived observation: %v", err)
	}
	return preflight.Plan
}

func stopUnfinishedHelper(run *helperRun) {
	if run == nil {
		return
	}
	run.mu.Lock()
	waited := run.waited
	run.mu.Unlock()
	if waited {
		return
	}
	_ = run.Stop()
	_, _ = run.Wait(context.Background())
}
