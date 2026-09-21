package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type probeV2ScriptedExecutor struct {
	mu            sync.Mutex
	requests      []ExecutionRequest
	deadlines     []time.Time
	remaining     []time.Duration
	active        int
	maximumActive int
	handler       func(int, context.Context, ExecutionRequest) (ExecutionObservation, error)
}

func (executor *probeV2ScriptedExecutor) Execute(ctx context.Context, request ExecutionRequest) (ExecutionObservation, error) {
	executor.mu.Lock()
	index := len(executor.requests)
	executor.requests = append(executor.requests, cloneProbeV2Request(request))
	executor.active++
	if executor.active > executor.maximumActive {
		executor.maximumActive = executor.active
	}
	if deadline, ok := ctx.Deadline(); ok {
		executor.deadlines = append(executor.deadlines, deadline)
		executor.remaining = append(executor.remaining, time.Until(deadline))
	} else {
		executor.deadlines = append(executor.deadlines, time.Time{})
		executor.remaining = append(executor.remaining, 0)
	}
	handler := executor.handler
	executor.mu.Unlock()
	defer func() {
		executor.mu.Lock()
		executor.active--
		executor.mu.Unlock()
	}()
	if handler != nil {
		return handler(index, ctx, request)
	}
	return successfulProbeV2Observation(index, request), nil
}

func (executor *probeV2ScriptedExecutor) snapshot() ([]ExecutionRequest, []time.Time, []time.Duration, int) {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	requests := make([]ExecutionRequest, len(executor.requests))
	for index, request := range executor.requests {
		requests[index] = cloneProbeV2Request(request)
	}
	return requests, append([]time.Time(nil), executor.deadlines...), append([]time.Duration(nil), executor.remaining...), executor.maximumActive
}

func cloneProbeV2Request(request ExecutionRequest) ExecutionRequest {
	request.Command = append([]string(nil), request.Command...)
	request.Inputs = append([]RequestInput(nil), request.Inputs...)
	return request
}

func successfulProbeV2Observation(index int, request ExecutionRequest) ExecutionObservation {
	outputBytes := []byte(fmt.Sprintf("gradeable controlled response %02d", index+1))
	outputPath := filepath.Join(request.Roots.Output, fmt.Sprintf("response-%02d.txt", index+1))
	return ExecutionObservation{Process: ProcessEvidence{
		Identity: fmt.Sprintf("controlled-process-%02d", index+1), Kind: "controlled-executor", PID: 1000 + index,
		Owner: "probe-v2-test", Started: true, Exited: true, ExitCode: 0,
	}, Outputs: []RecordedIdentity{{
		Identity: outputPath, PathIdentity: pathIdentity(outputPath), Bytes: int64(len(outputBytes)), SHA256: hashBytes(outputBytes),
	}}}
}

func portableCorpusManifestV2(t *testing.T) CorpusV2Manifest {
	t.Helper()
	_, manifest := portableCorpusV2Fixture(t)
	return cloneCorpusV2Manifest(manifest)
}

func cachedCorpusReaderV2(manifest CorpusV2Manifest) corpusV2ManifestReader {
	return func(ctx context.Context, _ CorpusV2Authority) (CorpusV2Manifest, error) {
		if err := ctx.Err(); err != nil {
			return CorpusV2Manifest{}, err
		}
		return cloneCorpusV2Manifest(manifest), nil
	}
}

func TestProbeRunnerV2ExecutionRunsWarmupCanaryAndEightSamplesSerially(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	input, inputPath, reportPath := validProbeInputV2(t, "execute-ten-call-success")
	before := snapshotProbeV2CorpusFiles(t, manifest)
	executor := &probeV2ScriptedExecutor{}
	runner := newPortableRunnerV2(t, executor)
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	report, err := runner.Run(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run controlled v2 corpus sequence: %v", err)
	}
	if report.Mode != "EXECUTE" || report.Status != "PASS" || report.Failure != nil || len(report.Calls) != 10 || len(report.Processes) != 10 || len(report.Outputs) != 10 {
		t.Fatalf("execution result mode/status/calls/processes/outputs = %s/%s/%d/%d/%d", report.Mode, report.Status, len(report.Calls), len(report.Processes), len(report.Outputs))
	}
	if report.Cleanup != (CleanupEvidence{Checked: true}) || report.Corpus.CopiedBytes != 0 || report.Corpus.UploadedBytes != 0 || !report.Corpus.ReadOnly {
		t.Fatalf("execution cleanup or corpus accounting is not clean: cleanup=%#v copied=%d uploaded=%d readOnly=%t", report.Cleanup, report.Corpus.CopiedBytes, report.Corpus.UploadedBytes, report.Corpus.ReadOnly)
	}
	if report.Policy.Platform != runtime.GOOS+"/"+runtime.GOARCH || report.Policy.Port == ProbeForbiddenPort || report.Policy.PerCallTimeoutSeconds != 180 || report.Policy.NetworkPolicy != ProbeNetworkPolicy || report.Policy.DownloadBytes != 0 || report.Policy.PaidUSD != 0 || report.Policy.MaxHeavyProcesses != 1 || report.Policy.MaxCalls != 10 || report.Policy.MaxRetries != 0 {
		t.Fatalf("execution policy does not preserve the declared environment and budgets: %#v", report.Policy)
	}
	requests, deadlines, remaining, maximumActive := executor.snapshot()
	if len(requests) != 10 || len(deadlines) != 10 || len(remaining) != 10 || maximumActive != 1 {
		t.Fatalf("executor calls/deadlines/concurrency = %d/%d/%d/%d, want ten bounded serial calls", len(requests), len(deadlines), len(remaining), maximumActive)
	}
	for _, request := range requests {
		if request.Port != report.Policy.Port {
			t.Fatal("executor port differs from the isolated report port")
		}
	}
	assertProbeV2RequestSequence(t, requests, report.Calls, manifest)
	for index, value := range remaining {
		if deadlines[index].IsZero() || value <= 170*time.Second || value > 180*time.Second {
			t.Fatalf("executor call %d did not receive a fresh 180-second deadline", index+1)
		}
	}
	if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned probe roots survived execution: %v", err)
	}
	assertProbeV2CorpusFilesUnchanged(t, before)
	persisted, err := readProbeReportV2(reportPath, runner.corpusAuthority())
	if err != nil || persisted.Status != "PASS" || len(persisted.Calls) != 10 {
		t.Fatalf("persisted execution report status/calls = %s/%d, err=%v", persisted.Status, len(persisted.Calls), err)
	}
	assertProbeV2ReportIsRedacted(t, reportPath, input, manifest)
}

func assertProbeV2RequestSequence(t *testing.T, requests []ExecutionRequest, calls []ProbeCallReportV2, manifest CorpusV2Manifest) {
	t.Helper()
	for index, request := range requests {
		call := calls[index]
		if index == 0 {
			if call.Kind != probeV2CallWarmup || call.SampleIdentity != nil || len(request.Inputs) != 1 || len(request.Command) != 7 || request.Journey != JourneyName("warmup") {
				t.Fatalf("first controlled call is not the text-only warm-up at ordinal %d", index+1)
			}
			if request.Command[6] != "prompt="+probeV2WarmupPrompt {
				t.Fatalf("warm-up prompt changed from the fixed non-video sentinel at ordinal %d", index+1)
			}
		} else {
			sampleIndex := index - 1
			wantKind := probeV2CallSample
			if sampleIndex == 0 {
				wantKind = probeV2CallCanary
			}
			if call.Kind != wantKind || call.SampleIdentity == nil || *call.SampleIdentity != probeV2SampleIdentity(manifest.Samples[sampleIndex].Study, manifest.Samples[sampleIndex].Band, manifest.Samples[sampleIndex].Attempt) || len(request.Inputs) != 2 || len(request.Command) != 9 {
				t.Fatalf("controlled call %d does not match the deterministic video sample order", index+1)
			}
			if request.Command[7] != "--input" || request.Command[8] != "video=@"+manifest.Samples[sampleIndex].Clip.Path {
				t.Fatalf("controlled call %d does not use the selected public @video input", index+1)
			}
			if call.Inputs[1] != recordedCorpusV2File(manifest.Samples[sampleIndex].Clip) {
				t.Fatalf("call %d input identity differs from the pinned selected clip", index+1)
			}
			promptSource, err := os.ReadFile(manifest.Samples[sampleIndex].Prompt.Path)
			if err != nil {
				t.Fatalf("read selected sibling prompt for call %d: %v", index+1, err)
			}
			wantPrompt := strings.TrimSpace(string(promptSource)) + probeV2AntiEchoInstruction
			if request.Command[6] != "prompt="+wantPrompt {
				t.Fatalf("call %d prompt is not derived from the pinned sibling with the fixed anti-echo instruction", index+1)
			}
			videoInput := request.Inputs[1]
			clip := manifest.Samples[sampleIndex].Clip
			if videoInput.Name != "video" || videoInput.Modality != "VIDEO" || videoInput.MediaType != "video/mp4" || videoInput.Bytes != clip.Bytes || videoInput.SHA256 != clip.SHA256 {
				t.Fatalf("call %d executor video identity differs from the pinned @ input", index+1)
			}
		}
		if len(request.Command) < 7 || request.Command[0] != "models" || request.Command[1] != "invoke" || request.Command[2] != "llm" || request.Command[3] != "--operation" || request.Command[4] != "OMNI" || request.Command[5] != "--input" || !strings.HasPrefix(request.Command[6], "prompt=") {
			t.Fatalf("controlled call %d does not use the shipped OMNI CLI grammar", index+1)
		}
		prompt := strings.TrimPrefix(request.Command[6], "prompt=")
		if hashBytes([]byte(prompt)) != request.Inputs[0].SHA256 || request.Inputs[0].Name != "prompt" || request.Inputs[0].MediaType != "text/plain" {
			t.Fatalf("controlled call %d prompt digest or text input metadata mismatch", index+1)
		}
		if call.Command[6] != "prompt=sha256:"+request.Inputs[0].SHA256 || strings.Contains(call.Command[6], prompt) {
			t.Fatalf("report call %d did not replace prompt content with its digest", index+1)
		}
	}
}

type probeV2FailureCase struct {
	name          string
	index         int
	status        string
	wantCalls     int
	wantCode      ValidationCode
	failedOutcome func(ExecutionRequest, int) ExecutionObservation
}

func TestProbeRunnerV2FailuresStopAtTheMinimalPrefixWithoutRetry(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	cases := []probeV2FailureCase{
		{name: "warmup cli failure", index: 0, status: "FAIL", wantCalls: 1, wantCode: CodeProbeExecutionFailure, failedOutcome: failedProbeV2Observation},
		{name: "warmup timeout", index: 0, status: "INCONCLUSIVE", wantCalls: 1, wantCode: CodeProbeTimedOut, failedOutcome: timedOutProbeV2Observation},
		{name: "warmup cancellation", index: 0, status: "INCONCLUSIVE", wantCalls: 1, wantCode: CodeProbeCancelled, failedOutcome: cancelledProbeV2Observation},
		{name: "warmup empty output", index: 0, status: "INCONCLUSIVE", wantCalls: 1, wantCode: CodeProbeOutputFailure, failedOutcome: emptyProbeV2OutputObservation},
		{name: "warmup malformed output", index: 0, status: "INCONCLUSIVE", wantCalls: 1, wantCode: CodeProbeOutputFailure, failedOutcome: malformedProbeV2OutputObservation},
		{name: "canary cli failure", index: 1, status: "FAIL", wantCalls: 2, wantCode: CodeProbeExecutionFailure, failedOutcome: failedProbeV2Observation},
		{name: "canary timeout", index: 1, status: "INCONCLUSIVE", wantCalls: 2, wantCode: CodeProbeTimedOut, failedOutcome: timedOutProbeV2Observation},
		{name: "canary cancellation", index: 1, status: "INCONCLUSIVE", wantCalls: 2, wantCode: CodeProbeCancelled, failedOutcome: cancelledProbeV2Observation},
		{name: "canary empty output", index: 1, status: "INCONCLUSIVE", wantCalls: 2, wantCode: CodeProbeOutputFailure, failedOutcome: emptyProbeV2OutputObservation},
		{name: "canary malformed output", index: 1, status: "INCONCLUSIVE", wantCalls: 2, wantCode: CodeProbeOutputFailure, failedOutcome: malformedProbeV2OutputObservation},
		{name: "later sample cli failure", index: 4, status: "FAIL", wantCalls: 5, wantCode: CodeProbeExecutionFailure, failedOutcome: failedProbeV2Observation},
		{name: "later sample ungradeable output", index: 4, status: "INCONCLUSIVE", wantCalls: 5, wantCode: CodeProbeOutputFailure, failedOutcome: malformedProbeV2OutputObservation},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input, inputPath, reportPath := validProbeInputV2(t, strings.ReplaceAll(testCase.name, " ", "-"))
			executor := &probeV2ScriptedExecutor{handler: func(index int, _ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
				if index == testCase.index {
					return testCase.failedOutcome(request, index), nil
				}
				return successfulProbeV2Observation(index, request), nil
			}}
			runner := newPortableRunnerV2(t, executor)
			runner.corpusReader = cachedCorpusReaderV2(manifest)
			report, err := runner.Run(context.Background(), inputPath, reportPath)
			if err != nil {
				t.Fatalf("run controlled %s case: %v", testCase.name, err)
			}
			if report.Status != testCase.status || len(report.Calls) != testCase.wantCalls || len(report.Processes) != testCase.wantCalls {
				t.Fatalf("%s result status/calls/processes = %s/%d/%d", testCase.name, report.Status, len(report.Calls), len(report.Processes))
			}
			if report.Failure == nil || report.Failure.Code != string(testCase.wantCode) || report.Calls[len(report.Calls)-1].Failure == nil || report.Calls[len(report.Calls)-1].Failure.Code != string(testCase.wantCode) {
				t.Fatalf("%s did not preserve the expected typed terminal classification", testCase.name)
			}
			process := report.Processes[testCase.index]
			wantExitCode := 0
			wantTimedOut := false
			if testCase.wantCode == CodeProbeExecutionFailure {
				wantExitCode = 17
			} else if testCase.wantCode == CodeProbeTimedOut {
				wantExitCode = -1
				wantTimedOut = true
			} else if testCase.wantCode == CodeProbeCancelled {
				wantExitCode = -1
			}
			if process.Identity != fmt.Sprintf("controlled-process-%02d", testCase.index+1) || process.Kind != "controlled-executor" || process.PID != 1000+testCase.index || process.Owner != "probe-v2-test" || !process.Started || !process.Exited || process.ExitCode != wantExitCode || process.TimedOut != wantTimedOut {
				t.Fatalf("%s process evidence does not match the exact controlled observation", testCase.name)
			}
			requests, _, _, _ := executor.snapshot()
			if len(requests) != testCase.wantCalls {
				t.Fatalf("%s made %d executor calls, want the minimal prefix of %d", testCase.name, len(requests), testCase.wantCalls)
			}
			if report.Cleanup != (CleanupEvidence{Checked: true}) || len(report.Outputs) != testCase.index {
				t.Fatalf("%s cleanup/output prefix = %#v/%d", testCase.name, report.Cleanup, len(report.Outputs))
			}
			if _, err := os.Stat(input.ProbeRoot); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s retained an owned probe root: %v", testCase.name, err)
			}
			body, err := os.ReadFile(reportPath)
			if err != nil {
				t.Fatalf("read %s report: %v", testCase.name, err)
			}
			if bytes.Contains(body, []byte("PRIVATE_PROMPT_SENTINEL")) {
				t.Fatalf("%s persisted private failure or prompt text", testCase.name)
			}
		})
	}
}

func TestProbeRunnerV2RecordsTimeoutBeforeProcessStart(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	input, inputPath, reportPath := validProbeInputV2(t, "timeout-before-process-start")
	executor := &probeV2ScriptedExecutor{handler: func(_ int, _ context.Context, _ ExecutionRequest) (ExecutionObservation, error) {
		return ExecutionObservation{TimedOut: true, Process: ProcessEvidence{
			Identity: "controlled-before-start", Kind: "controlled-executor", Owner: "probe-v2-test", TimedOut: true,
		}}, nil
	}}
	runner := newPortableRunnerV2(t, executor)
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	report, err := runner.Run(context.Background(), inputPath, reportPath)
	if err != nil {
		t.Fatalf("run controlled pre-start timeout: %v", err)
	}
	if report.Status != "INCONCLUSIVE" || len(report.Calls) != 1 || report.Calls[0].Failure == nil || report.Calls[0].Failure.Code != string(CodeProbeTimedOut) {
		t.Fatalf("pre-start timeout result status/error = %s/%v", report.Status, err)
	}
	if len(report.Processes) != 1 || len(report.Outputs) != 0 {
		t.Fatalf("pre-start timeout evidence calls/processes/outputs = %d/%d/%d", len(report.Calls), len(report.Processes), len(report.Outputs))
	}
	process := report.Processes[0]
	if process.Started || process.Exited || process.PID != 0 || !process.TimedOut || report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("pre-start timeout process/cleanup evidence = %#v/%#v", process, report.Cleanup)
	}
	requests, _, _, _ := executor.snapshot()
	if len(requests) != 1 {
		t.Fatalf("pre-start timeout made %d executor calls, want one and no retry", len(requests))
	}
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("pre-start timeout retained owned roots: %v", statErr)
	}
}

func failedProbeV2Observation(request ExecutionRequest, index int) ExecutionObservation {
	observation := successfulProbeV2Observation(index, request)
	observation.Process.ExitCode = 17
	observation.Process.Identity = fmt.Sprintf("controlled-process-%02d", index+1)
	observation.Failure = &ReportFailure{Owner: "cli", Code: "CLI_COMMAND_FAILED", Expected: "PRIVATE_PROMPT_SENTINEL", Observed: "PRIVATE_PROMPT_SENTINEL", NextAction: "PRIVATE_PROMPT_SENTINEL"}
	return observation
}

func timedOutProbeV2Observation(request ExecutionRequest, index int) ExecutionObservation {
	observation := successfulProbeV2Observation(index, request)
	observation.Process.TimedOut = true
	observation.Process.ExitCode = -1
	observation.TimedOut = true
	return observation
}

func cancelledProbeV2Observation(request ExecutionRequest, index int) ExecutionObservation {
	observation := successfulProbeV2Observation(index, request)
	observation.Process.ExitCode = -1
	observation.Cancelled = true
	return observation
}

func emptyProbeV2OutputObservation(request ExecutionRequest, index int) ExecutionObservation {
	observation := successfulProbeV2Observation(index, request)
	observation.Outputs = nil
	return observation
}

func malformedProbeV2OutputObservation(request ExecutionRequest, index int) ExecutionObservation {
	observation := successfulProbeV2Observation(index, request)
	observation.Outputs = []RecordedIdentity{{Identity: "empty", PathIdentity: "bad-path", Bytes: 0, SHA256: "bad-digest"}}
	return observation
}

func TestProbeRunnerV2DoesNotPublishUntilOwnedCleanupIsZero(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	cases := []struct {
		name   string
		mutate func(*ExecutionObservation)
	}{
		{name: "process survivor", mutate: func(observation *ExecutionObservation) { observation.OwnedProcessSurvivors = 1 }},
		{name: "listener survivor", mutate: func(observation *ExecutionObservation) { observation.OwnedListenerSurvivors = 1 }},
		{name: "partial output", mutate: func(observation *ExecutionObservation) { observation.PartialOutputs = 1 }},
		{name: "nonterminal process", mutate: func(observation *ExecutionObservation) { observation.Process.Exited = false }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input, inputPath, reportPath := validProbeInputV2(t, strings.ReplaceAll(testCase.name, " ", "-"))
			executor := &probeV2ScriptedExecutor{handler: func(index int, _ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
				observation := successfulProbeV2Observation(index, request)
				testCase.mutate(&observation)
				return observation, nil
			}}
			runner := newPortableRunnerV2(t, executor)
			runner.corpusReader = cachedCorpusReaderV2(manifest)
			_, err := runner.Run(context.Background(), inputPath, reportPath)
			if !hasValidationCode(err, CodeProbeCleanupFailure) {
				t.Fatalf("%s execution error = %v, want cleanup failure", testCase.name, err)
			}
			requests, _, _, _ := executor.snapshot()
			if len(requests) != 1 {
				t.Fatalf("%s made %d executor calls, want one and no expansion", testCase.name, len(requests))
			}
			if _, statErr := os.Stat(reportPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s published a report with cleanup unproven: %v", testCase.name, statErr)
			}
			if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s left owned roots after cleanup: %v", testCase.name, statErr)
			}
		})
	}
}

func TestProbeRunnerV2SerializesCompetingRunsAndHonorsCancellation(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	firstInput, firstInputPath, firstReportPath := validProbeInputV2(t, "competing-run-first")
	secondInput, secondInputPath, secondReportPath := validProbeInputV2(t, "competing-run-second")
	started := make(chan struct{})
	firstExecutor := &probeV2ScriptedExecutor{handler: func(index int, ctx context.Context, request ExecutionRequest) (ExecutionObservation, error) {
		if index == 0 {
			close(started)
			<-ctx.Done()
			observation := successfulProbeV2Observation(index, request)
			observation.Cancelled = true
			return observation, nil
		}
		return successfulProbeV2Observation(index, request), nil
	}}
	firstRunner := newPortableRunnerV2(t, firstExecutor)
	firstRunner.corpusReader = cachedCorpusReaderV2(manifest)
	firstContext, cancelFirst := context.WithCancel(context.Background())
	type runResult struct {
		report ProbeReportV2
		err    error
	}
	firstDone := make(chan runResult, 1)
	go func() {
		report, err := firstRunner.RunInput(firstContext, firstInput, firstInputPath, firstReportPath)
		firstDone <- runResult{report: report, err: err}
	}()
	<-started

	secondExecutor := &probeV2ScriptedExecutor{}
	secondRunner := newPortableRunnerV2(t, secondExecutor)
	secondRunner.corpusReader = cachedCorpusReaderV2(manifest)
	_, secondErr := secondRunner.Run(context.Background(), secondInputPath, secondReportPath)
	if !hasValidationCode(secondErr, CodeProbeHeavyOwnerBusy) {
		cancelFirst()
		t.Fatalf("competing runner error = %v, want one-owner admission", secondErr)
	}
	secondRequests, _, _, _ := secondExecutor.snapshot()
	if len(secondRequests) != 0 {
		cancelFirst()
		t.Fatalf("competing runner reached its executor %d times", len(secondRequests))
	}
	if _, statErr := os.Stat(secondInput.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		cancelFirst()
		t.Fatalf("busy runner created an owned root: %v", statErr)
	}
	if _, statErr := os.Stat(secondReportPath); !errors.Is(statErr, os.ErrNotExist) {
		cancelFirst()
		t.Fatalf("busy runner published a report before acquiring ownership: %v", statErr)
	}
	cancelFirst()
	firstResult := <-firstDone
	if firstResult.err != nil || firstResult.report.Status != "INCONCLUSIVE" || firstResult.report.Calls[0].Failure.Code != string(CodeProbeCancelled) {
		t.Fatalf("cancelled owner result status/error = %s/%v", firstResult.report.Status, firstResult.err)
	}
	firstRequests, _, _, maximumActive := firstExecutor.snapshot()
	if len(firstRequests) != 1 || maximumActive != 1 || firstResult.report.Cleanup != (CleanupEvidence{Checked: true}) {
		t.Fatalf("cancelled owner calls/concurrency/cleanup = %d/%d/%#v", len(firstRequests), maximumActive, firstResult.report.Cleanup)
	}
	if _, statErr := os.Stat(firstInput.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("cancelled owner retained its root: %v", statErr)
	}
	secondReport, err := secondRunner.Run(context.Background(), secondInputPath, secondReportPath)
	if err != nil || secondReport.Status != "PASS" {
		t.Fatalf("runner could not acquire released ownership: status=%s error=%v", secondReport.Status, err)
	}
}

func TestProbeRunnerV2RejectsExistingReportWithoutOverwriteOrExecution(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	input, inputPath, reportPath := validProbeInputV2(t, "existing-report")
	sentinel := []byte("operator-owned report bytes")
	if err := os.WriteFile(reportPath, sentinel, 0o600); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}
	executor := &probeV2ScriptedExecutor{}
	runner := newPortableRunnerV2(t, executor)
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	_, err := runner.Run(context.Background(), inputPath, reportPath)
	if !hasValidationCode(err, CodeProbeInvalidReport) {
		t.Fatalf("existing report error = %v, want no-overwrite rejection", err)
	}
	requests, _, _, _ := executor.snapshot()
	if len(requests) != 0 {
		t.Fatalf("existing report admitted %d executor calls", len(requests))
	}
	actual, err := os.ReadFile(reportPath)
	if err != nil || !bytes.Equal(actual, sentinel) {
		t.Fatalf("existing report was changed: read error=%v", err)
	}
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("existing report path created an owned root: %v", statErr)
	}
}

func TestProbeRunnerV2AtomicPersistenceDoesNotOverwriteConcurrentDestination(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	input, inputPath, reportPath := validProbeInputV2(t, "report-race")
	started := make(chan struct{})
	continueCall := make(chan struct{})
	executor := &probeV2ScriptedExecutor{handler: func(index int, _ context.Context, request ExecutionRequest) (ExecutionObservation, error) {
		if index == 0 {
			close(started)
			<-continueCall
			return failedProbeV2Observation(request, index), nil
		}
		return successfulProbeV2Observation(index, request), nil
	}}
	runner := newPortableRunnerV2(t, executor)
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	type runResult struct {
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		_, err := runner.Run(context.Background(), inputPath, reportPath)
		done <- runResult{err: err}
	}()
	<-started
	sentinel := []byte("destination created after admission")
	if err := os.WriteFile(reportPath, sentinel, 0o600); err != nil {
		close(continueCall)
		t.Fatalf("create competing destination: %v", err)
	}
	close(continueCall)
	result := <-done
	if !hasValidationCode(result.err, CodeProbeInvalidReport) {
		t.Fatalf("concurrent destination write error = %v, want no-overwrite rejection", result.err)
	}
	actual, err := os.ReadFile(reportPath)
	if err != nil || !bytes.Equal(actual, sentinel) {
		t.Fatalf("concurrent destination was changed: read error=%v", err)
	}
	assertNoProbeV2ReportTemporaryFiles(t, filepath.Dir(reportPath))
	if _, statErr := os.Stat(input.ProbeRoot); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("concurrent destination run retained its roots: %v", statErr)
	}
}

func TestProbeRunnerV2AtomicWriteFailureLeavesNoAcceptedPartialReport(t *testing.T) {
	manifest := portableCorpusManifestV2(t)
	_, inputPath, preflightPath := validProbeInputV2(t, "atomic-write-failure")
	runner := newPortableRunnerV2(t, nil)
	runner.corpusReader = cachedCorpusReaderV2(manifest)
	report, err := runner.Preflight(context.Background(), inputPath, preflightPath)
	if err != nil {
		t.Fatalf("prepare valid v2 report for atomic persistence check: %v", err)
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	parentSentinel := []byte("parent file remains intact")
	if err := os.WriteFile(parentFile, parentSentinel, 0o600); err != nil {
		t.Fatalf("write blocking parent file: %v", err)
	}
	reportPath := filepath.Join(parentFile, "report.json")
	if err := writeProbeReportV2Atomic(reportPath, report, runner.corpusAuthority()); !hasValidationCode(err, CodeProbeReportPersist) {
		t.Fatalf("atomic write error = %v, want typed persistence failure", err)
	}
	actual, err := os.ReadFile(parentFile)
	if err != nil || !bytes.Equal(actual, parentSentinel) {
		t.Fatalf("failed write changed blocking parent: read error=%v", err)
	}
	if _, err := readProbeReportV2(reportPath, runner.corpusAuthority()); err == nil {
		t.Fatal("failed atomic write exposed an accepted partial report")
	}
}

func snapshotProbeV2CorpusFiles(t *testing.T, manifest CorpusV2Manifest) []RecordedIdentity {
	t.Helper()
	identities := make([]RecordedIdentity, 0, len(manifest.Samples)*2)
	for _, sample := range manifest.Samples {
		for _, file := range []CorpusV2FileIdentity{sample.Prompt, sample.Clip} {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				t.Fatal("read pinned source before execution")
			}
			identities = append(identities, RecordedIdentity{Identity: file.Identity, Bytes: int64(len(data)), SHA256: hashBytes(data)})
		}
	}
	return identities
}

func assertProbeV2CorpusFilesUnchanged(t *testing.T, before []RecordedIdentity) {
	t.Helper()
	manifest := portableCorpusManifestV2(t)
	index := 0
	for _, sample := range manifest.Samples {
		for _, file := range []CorpusV2FileIdentity{sample.Prompt, sample.Clip} {
			data, err := os.ReadFile(file.Path)
			if err != nil {
				t.Fatal("read pinned source after execution")
			}
			if int64(len(data)) != before[index].Bytes || hashBytes(data) != before[index].SHA256 {
				t.Fatalf("pinned corpus source changed at selected index %d", index)
			}
			index++
		}
	}
}

func assertProbeV2ReportIsRedacted(t *testing.T, reportPath string, input ProbeInputV2, manifest CorpusV2Manifest) {
	t.Helper()
	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report for redaction assertions: %v", err)
	}
	for _, secretPath := range []string{input.Build.Path, input.Dependencies.Model.Path, input.Dependencies.Projector.Path, input.Dependencies.Backend.Path, input.CorpusInput.Path, input.ProbeRoot, CorpusV2Repository} {
		if strings.Contains(string(body), secretPath) {
			t.Fatal("execution report contains an absolute source or workspace path")
		}
	}
	for _, call := range mustReadProbeReportV2(t, reportPath).Calls {
		if strings.Contains(call.Command[6], "prompt=") && !strings.HasPrefix(call.Command[6], "prompt=sha256:") {
			t.Fatal("execution report contains prompt content")
		}
		if call.Kind != probeV2CallWarmup && !strings.Contains(call.Command[8], call.Inputs[1].PathIdentity) {
			t.Fatal("execution report does not replace the video path with its digest identity")
		}
	}
	for _, sample := range manifest.Samples {
		if strings.Contains(string(body), sample.Prompt.Path) || strings.Contains(string(body), sample.Clip.Path) {
			t.Fatal("execution report contains a corpus source path")
		}
	}
}

func mustReadProbeReportV2(t *testing.T, path string) ProbeReportV2 {
	t.Helper()
	authority, _ := portableCorpusV2Fixture(t)
	report, err := readProbeReportV2(path, authority)
	if err != nil {
		t.Fatalf("read validated execution report: %v", err)
	}
	return report
}

func assertNoProbeV2ReportTemporaryFiles(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("inspect report directory after atomic write: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".omni-video-runner-v2-") {
			t.Fatal("atomic report write left a temporary file")
		}
	}
}
