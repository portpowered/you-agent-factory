package omni_media_probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/tests/internal/localai/corpusv2"
)

type probeV2CallPlan struct {
	kind           string
	sample         *corpusv2.CorpusV2Sample
	prompt         string
	promptIdentity RecordedIdentity
}

type probeV2CallOutcome struct {
	call    ProbeCallReportV2
	process ProcessEvidence
	output  *RecordedIdentity
}

func (runner RunnerV2) Run(ctx context.Context, inputPath, reportPath string) (ProbeReportV2, error) {
	if runner.Executor == nil {
		return ProbeReportV2{}, validationError(CodeProbeExecutionFailure, "executor", "public CLI executor", "nil", nil)
	}
	input, err := ReadProbeInputV2(inputPath)
	if err != nil {
		return ProbeReportV2{}, err
	}
	return runner.RunInput(ctx, input, inputPath, reportPath)
}

func (runner RunnerV2) RunInput(ctx context.Context, input ProbeInputV2, inputPath, reportPath string) (ProbeReportV2, error) {
	if runner.Executor == nil {
		return ProbeReportV2{}, validationError(CodeProbeExecutionFailure, "executor", "public CLI executor", "nil", nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	prepared, err := runner.admit(ctx, input, inputPath, reportPath)
	if err != nil {
		return ProbeReportV2{}, err
	}
	plans, err := buildProbeV2CallPlan(prepared.manifest)
	if err != nil {
		return ProbeReportV2{}, err
	}
	if err := ctx.Err(); err != nil {
		return ProbeReportV2{}, probeContextError(err)
	}
	release, err := reserveProbeHeavyOwner(ctx)
	if err != nil {
		return ProbeReportV2{}, err
	}
	defer release()

	roots, err := createProbeRoots(input.ProbeRoot, prepared.reportPath)
	if err != nil {
		return ProbeReportV2{}, err
	}
	listener, port, err := reserveProbePort(input.Limits.ForbiddenPort)
	if err != nil {
		if cleanupErr := cleanupProbeRoots(roots, prepared.reportPath); cleanupErr != nil {
			return ProbeReportV2{}, probeV2CleanupError()
		}
		return ProbeReportV2{}, fmt.Errorf("reserve corpus runner v2 port: %w", err)
	}
	roots.Port = port

	report := newProbeReportV2(prepared, port)
	report.Mode = "EXECUTE"
	report.Status = "INCONCLUSIVE"
	report.Cleanup = CleanupEvidence{}
	report, executionErr := runner.executeProbeV2Plan(ctx, prepared, roots, plans, report)
	listenerErr := listener.Close()
	rootsErr := cleanupProbeRoots(roots, prepared.reportPath)
	if listenerErr != nil || rootsErr != nil {
		return ProbeReportV2{}, probeV2CleanupError()
	}
	if executionErr != nil {
		return ProbeReportV2{}, executionErr
	}
	report.Cleanup = CleanupEvidence{Checked: true}
	if err := report.Validate(); err != nil {
		return ProbeReportV2{}, fmt.Errorf("validate corpus runner v2 execution report: %w", err)
	}
	if err := WriteProbeReportV2Atomic(prepared.reportPath, report); err != nil {
		return ProbeReportV2{}, fmt.Errorf("persist corpus runner v2 execution report: %w", err)
	}
	return report, nil
}

func (runner RunnerV2) executeProbeV2Plan(ctx context.Context, prepared admittedProbeInputV2, roots probeRoots, plans []probeV2CallPlan, report ProbeReportV2) (ProbeReportV2, error) {
	budget := &probeCallBudget{maxCalls: prepared.input.Limits.MaxCalls}
	for index, plan := range plans {
		outcome, err := runner.executeProbeV2Call(ctx, plan, index+1, roots, budget)
		if err != nil {
			return ProbeReportV2{}, err
		}
		report.Calls = append(report.Calls, outcome.call)
		report.Processes = append(report.Processes, outcome.process)
		if outcome.output != nil {
			report.Outputs = append(report.Outputs, *outcome.output)
		}
		if outcome.call.Status != "PASS" {
			report.Status = outcome.call.Status
			failure := *outcome.call.Failure
			report.Failure = &failure
			return report, nil
		}
	}
	report.Status = "PASS"
	return report, nil
}

func (runner RunnerV2) executeProbeV2Call(ctx context.Context, plan probeV2CallPlan, ordinal int, roots probeRoots, budget *probeCallBudget) (probeV2CallOutcome, error) {
	call := plan.reportCall(ordinal)
	if err := ctx.Err(); err != nil {
		return notStartedProbeV2Call(call, ordinal, err), nil
	}
	if plan.sample != nil {
		if err := verifyProbeV2File(plan.sample.Clip); err != nil {
			failure := probeFailure("runner", string(CodeProbeIdentityMismatch), "pinned clip identity remains unchanged", "clip identity changed after admission", "refresh corpus admission before another execution")
			call.Status = "FAIL"
			call.Failure = failure
			return probeV2CallOutcome{call: call, process: notStartedV2Process(ordinal)}, nil
		}
	}
	if err := budget.reserve(); err != nil {
		return probeV2CallOutcome{}, err
	}
	request := plan.executionRequest(roots.Paths, roots.Port)
	callContext, cancel := context.WithTimeout(ctx, time.Duration(ProbeV2PerCallTimeoutSeconds)*time.Second)
	observation, executeErr := runner.Executor.Execute(callContext, request)
	callContextErr := callContext.Err()
	cancel()
	process := v2ProcessEvidence(observation.Process, ordinal, callContextErr)
	if observation.OwnedProcessSurvivors != 0 || observation.OwnedListenerSurvivors != 0 || observation.PartialOutputs != 0 || (process.Started && !process.Exited) {
		return probeV2CallOutcome{}, probeV2CleanupError()
	}
	status, failure, output := classifyProbeV2Call(observation, executeErr, callContextErr)
	call.Status = status
	call.Failure = failure
	call.Output = output
	return probeV2CallOutcome{call: call, process: process, output: output}, nil
}

func buildProbeV2CallPlan(manifest corpusv2.CorpusV2Manifest) ([]probeV2CallPlan, error) {
	if len(manifest.Samples) != len(corpusv2.DefaultCorpusV2Authority().ExpectedSamples) {
		return nil, validationError(CodeProbeInvalidReport, "corpus.selectedSamples", "exact nine pinned samples", "sample count changed", nil)
	}
	plans := make([]probeV2CallPlan, 0, int(ProbeV2MaxCalls))
	warmup := []byte(probeV2WarmupPrompt)
	plans = append(plans, probeV2CallPlan{kind: probeV2CallWarmup, prompt: string(warmup), promptIdentity: inlineV2Identity("warmup-prompt", warmup)})
	for index := range manifest.Samples {
		sample := manifest.Samples[index]
		question, identity, err := prepareProbeV2Question(sample)
		if err != nil {
			return nil, err
		}
		if err := verifyProbeV2File(sample.Clip); err != nil {
			return nil, err
		}
		kind := probeV2CallSample
		if index == 0 {
			kind = probeV2CallCanary
		}
		plans = append(plans, probeV2CallPlan{kind: kind, sample: &sample, prompt: question, promptIdentity: identity})
	}
	return plans, nil
}

func prepareProbeV2Question(sample corpusv2.CorpusV2Sample) (string, RecordedIdentity, error) {
	data, err := os.ReadFile(sample.Prompt.Path)
	if err != nil {
		return "", RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, "corpus.prompt", "readable pinned sibling prompt", "prompt is unavailable", nil)
	}
	if int64(len(data)) != sample.Prompt.Bytes || !strings.EqualFold(hashBytes(data), sample.Prompt.SHA256) {
		return "", RecordedIdentity{}, validationError(CodeProbeIdentityMismatch, "corpus.prompt", "pinned prompt digest and byte count", "prompt identity changed", nil)
	}
	question := strings.TrimSpace(string(data)) + probeV2AntiEchoInstruction
	questionBytes := []byte(question)
	if strings.TrimSpace(string(data)) == "" || len(questionBytes) > probeV2MaxQuestionBytes {
		return "", RecordedIdentity{}, validationError(CodeProbeInvalidInput, "corpus.prompt", "non-empty bounded question", "prompt is empty or exceeds the question limit", nil)
	}
	return question, inlineV2Identity("derived-question", questionBytes), nil
}

func verifyProbeV2File(identity corpusv2.CorpusV2FileIdentity) error {
	info, err := os.Lstat(identity.Path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != identity.Bytes {
		return validationError(CodeProbeIdentityMismatch, "corpus.file", "pinned regular file and byte count", "source identity changed", nil)
	}
	file, err := os.Open(identity.Path)
	if err != nil {
		return validationError(CodeProbeIdentityMismatch, "corpus.file", "readable pinned file", "source identity is unavailable", nil)
	}
	hasher := sha256.New()
	count, copyErr := io.Copy(hasher, file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || count != identity.Bytes || !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), identity.SHA256) {
		return validationError(CodeProbeIdentityMismatch, "corpus.file", "pinned file digest and byte count", "source identity changed", nil)
	}
	return nil
}

func inlineV2Identity(label string, content []byte) RecordedIdentity {
	digest := hashBytes(content)
	return RecordedIdentity{
		Identity:     "sha256:" + digest,
		PathIdentity: pathIdentity(label + ":" + digest),
		Bytes:        int64(len(content)),
		SHA256:       digest,
	}
}

func (plan probeV2CallPlan) executionRequest(roots RootPaths, port int) ExecutionRequest {
	inputs := []RequestInput{{Order: 0, Name: "prompt", Modality: "TEXT", MediaType: "text/plain", Bytes: plan.promptIdentity.Bytes, SHA256: plan.promptIdentity.SHA256}}
	command := []string{"models", "invoke", "llm", "--operation", "OMNI", "--input", "prompt=" + plan.prompt}
	journey := JourneyName(strings.ToLower(plan.kind))
	if plan.sample != nil {
		clip := plan.sample.Clip
		inputs = append(inputs, RequestInput{Order: 1, Name: "video", Modality: "VIDEO", MediaType: "video/mp4", Bytes: clip.Bytes, SHA256: clip.SHA256})
		command = append(command, "--input", "video=@"+clip.Path)
	}
	return ExecutionRequest{Journey: journey, Command: command, Inputs: inputs, Roots: roots, Port: port}
}

func (plan probeV2CallPlan) reportCall(ordinal int) ProbeCallReportV2 {
	inputs := []RecordedIdentity{plan.promptIdentity}
	var sampleIdentity *string
	command := []string{"models", "invoke", "llm", "--operation", "OMNI", "--input", "prompt=sha256:" + plan.promptIdentity.SHA256}
	if plan.sample != nil {
		clip := recordedCorpusV2File(plan.sample.Clip)
		inputs = append(inputs, clip)
		identity := probeV2SampleIdentity(plan.sample.Study, plan.sample.Band, plan.sample.Attempt)
		sampleIdentity = &identity
		command = append(command, "--input", "video=@<"+clip.PathIdentity+">")
	}
	return ProbeCallReportV2{Ordinal: ordinal, Kind: plan.kind, SampleIdentity: sampleIdentity, Status: "INCONCLUSIVE", Command: command, Inputs: inputs}
}

func probeV2SampleIdentity(study, band, attempt string) string {
	return study + "/" + band + "/" + attempt
}

func notStartedProbeV2Call(call ProbeCallReportV2, ordinal int, err error) probeV2CallOutcome {
	code := string(CodeProbeCancelled)
	observed := "call was cancelled before process start"
	if errors.Is(err, context.DeadlineExceeded) {
		code = string(CodeProbeTimedOut)
		observed = "call deadline expired before process start"
	}
	call.Status = "INCONCLUSIVE"
	call.Failure = probeFailure("runner", code, "call starts within its per-call context", observed, "inspect cancellation and cleanup evidence before retrying")
	return probeV2CallOutcome{call: call, process: notStartedV2Process(ordinal)}
}

func notStartedV2Process(ordinal int) ProcessEvidence {
	return ProcessEvidence{Identity: fmt.Sprintf("not-started-call-%02d", ordinal), Kind: "public-cli", Owner: "runner"}
}

func v2ProcessEvidence(process ProcessEvidence, ordinal int, contextErr error) ProcessEvidence {
	identity := strings.TrimSpace(process.Identity)
	if identity == "" || reportStringContainsAbsolutePath(identity) || validateProbeLabel(identity, "process.identity") != nil {
		identity = fmt.Sprintf("call-%02d", ordinal)
		if process.PID > 0 {
			identity = fmt.Sprintf("pid-%d", process.PID)
		}
	}
	kind := strings.TrimSpace(process.Kind)
	if kind == "" || reportStringContainsAbsolutePath(kind) || validateProbeLabel(kind, "process.kind") != nil {
		kind = "public-cli"
	}
	owner := strings.TrimSpace(process.Owner)
	if owner == "" || reportStringContainsAbsolutePath(owner) || validateProbeLabel(owner, "process.owner") != nil {
		owner = "runner"
	}
	return ProcessEvidence{
		Identity: identity, Kind: kind, PID: process.PID, Owner: owner,
		Started: process.Started, Exited: process.Exited, ExitCode: process.ExitCode,
		TimedOut: process.TimedOut || errors.Is(contextErr, context.DeadlineExceeded),
	}
}

func classifyProbeV2Call(observation ExecutionObservation, executeErr, contextErr error) (string, *ReportFailure, *RecordedIdentity) {
	if observation.TimedOut || observation.Process.TimedOut || errors.Is(contextErr, context.DeadlineExceeded) || failureCodeIs(observation.Failure, CodeProbeTimedOut) {
		return "INCONCLUSIVE", probeFailure("runner", string(CodeProbeTimedOut), "public CLI call completes within 180 seconds", "per-call deadline expired", "inspect timeout and cleanup evidence before a fresh validation"), nil
	}
	if observation.Cancelled || errors.Is(contextErr, context.Canceled) || failureCodeIs(observation.Failure, CodeProbeCancelled) {
		return "INCONCLUSIVE", probeFailure("runner", string(CodeProbeCancelled), "public CLI call completes before cancellation", "call was cancelled", "inspect cancellation and cleanup evidence before a fresh validation"), nil
	}
	if executeErr != nil || observation.Failure != nil || (observation.Process.Started && observation.Process.ExitCode != 0) || !observation.Process.Started {
		return "FAIL", probeFailure("cli", string(CodeProbeExecutionFailure), "public CLI exits successfully", "CLI returned a failure result", "inspect the redacted process evidence and correct the CLI failure"), nil
	}
	if !observation.Process.Exited {
		return "INCONCLUSIVE", probeFailure("runner", string(CodeProbeExecutionFailure), "public CLI reaches a terminal process state", "process result is incomplete", "inspect process completion evidence"), nil
	}
	if len(observation.Outputs) != 1 {
		return "INCONCLUSIVE", probeFailure("runner", string(CodeProbeOutputFailure), "one non-empty gradeable text output", "output count is empty or malformed", "inspect the output mapping before fresh validation"), nil
	}
	output, ok := sanitizeProbeV2Output(observation.Outputs[0])
	if !ok {
		return "INCONCLUSIVE", probeFailure("runner", string(CodeProbeOutputFailure), "one non-empty gradeable text output", "output identity is empty or malformed", "inspect the output mapping before fresh validation"), nil
	}
	return "PASS", nil, &output
}

func failureCodeIs(failure *ReportFailure, code ValidationCode) bool {
	return failure != nil && failure.Code == string(code)
}

func sanitizeProbeV2Output(output RecordedIdentity) (RecordedIdentity, bool) {
	output.SHA256 = strings.ToLower(output.SHA256)
	output.Identity = "sha256:" + output.SHA256
	if err := validateRecordedIdentity(output, "output", true); err != nil {
		return RecordedIdentity{}, false
	}
	return output, true
}

func probeV2CleanupError() error {
	return validationError(CodeProbeCleanupFailure, "cleanup", "zero owned processes, listeners, and partial outputs", "cleanup could not be proven", nil)
}
