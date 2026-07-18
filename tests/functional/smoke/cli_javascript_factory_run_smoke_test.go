package smoke

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

const javascriptFactoryRunTimeout = 30 * time.Second

const parallelJavaScriptFanoutResult = "alpha=ALPHA_RESULT;beta=BETA_RESULT"
const orderedJavaScriptPipelineResult = "ordered-pipeline-complete"

func TestJavaScriptFactoryRun_RealCLIParallelFanoutCorrelatesChildren(t *testing.T) {
	t.Parallel()

	isolatedRoot := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	binaryPath := buildYouCLIBinary(t)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	isolatedHome := t.TempDir()
	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), javascriptFactoryRunTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binaryPath,
		"--json", "run",
		"--factory", "./parallel_fanout.js",
		"--with-mock-workers="+mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"--server", fmt.Sprintf("http://127.0.0.1:%d", port),
		"--quiet",
	)
	cmd.Dir = isolatedRoot
	cmd.Env = javascriptFactoryRunEnvironmentForHome(isolatedHome)
	cmd.WaitDelay = 2 * time.Second

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	assertParallelFanoutProcessResult(t, ctx, runErr, stdout.String(), stderr.String())
	assertParallelFanoutResult(t, decodeSingleJavaScriptFactoryRunResult(t, stdout.String()))
	functionalevidence.Covers(t, "cli/you.run")
}

func assertParallelFanoutProcessResult(t *testing.T, ctx context.Context, runErr error, stdout, stderr string) {
	t.Helper()
	if ctx.Err() != nil {
		t.Fatalf("you run parallel fanout timed out after %s: %v\nstdout:\n%s\nstderr:\n%s", javascriptFactoryRunTimeout, ctx.Err(), stdout, stderr)
	}
	if runErr != nil {
		t.Fatalf("you run parallel fanout exited non-zero: %v\nstdout:\n%s\nstderr:\n%s", runErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful quiet JSON invocation", stderr)
	}
}

type parallelFanoutEvidence struct {
	FinalValue string `json:"finalValue"`
	Children   []struct {
		Name         string `json:"name"`
		DispatchID   string `json:"dispatchId"`
		ResultStatus string `json:"resultStatus"`
		Response     string `json:"response"`
		RawResponse  string `json:"rawResponse"`
	} `json:"children"`
}

func assertParallelFanoutResult(t *testing.T, result factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted || result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("terminal outcome = %s (%s), want COMPLETED (SUCCEEDED)", result.SyncOutcome, result.Status)
	}
	if result.EffectivePolicy == nil || result.EffectivePolicy.AdditionalProperties["allowNetwork"] != false {
		t.Fatalf("effective policy = %#v, want public network disabled", result.EffectivePolicy)
	}
	if result.Result == nil || result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal || result.Result.PrimaryResult == nil || len(*result.Result.PrimaryResult) != 1 {
		t.Fatalf("result = %#v, want exactly one FINAL Factory Session result with one content part", result.Result)
	}
	part, err := (*result.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode parallel fanout result content part: %v", err)
	}
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode parallel fanout evidence: %v", err)
	}
	var evidence parallelFanoutEvidence
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode parallel fanout evidence: %v", err)
	}
	assertParallelFanoutEvidence(t, evidence)
}

func assertParallelFanoutEvidence(t *testing.T, evidence parallelFanoutEvidence) {
	t.Helper()
	if evidence.FinalValue != parallelJavaScriptFanoutResult || len(evidence.Children) != 2 {
		t.Fatalf("parallel fanout evidence = %#v, want final %q and exactly 2 children", evidence, parallelJavaScriptFanoutResult)
	}
	wantResponses := map[string]string{"alpha": "ALPHA_RESULT", "beta": "BETA_RESULT"}
	seenNames, seenDispatches := map[string]bool{}, map[string]bool{}
	for _, child := range evidence.Children {
		wantResponse, expected := wantResponses[child.Name]
		responseMarker := ":" + child.Name + ":" + wantResponse + ":"
		if !expected || seenNames[child.Name] || child.DispatchID == "" || seenDispatches[child.DispatchID] || child.ResultStatus != "COMPLETED" || child.Response != wantResponse || !strings.Contains(child.RawResponse, responseMarker) {
			t.Fatalf("child evidence = %#v, want one distinct completed dispatch correlated to %q", child, wantResponse)
		}
		seenNames[child.Name], seenDispatches[child.DispatchID] = true, true
	}
	for name := range wantResponses {
		if !seenNames[name] {
			t.Fatalf("missing child evidence for %q", name)
		}
	}
}

func TestJavaScriptFactoryRun_RealCLIProvesOrderedTwoStagePipeline(t *testing.T) {
	t.Parallel()

	workingRoot := support.LegacyFixtureDir(t, "dynamic")
	isolatedRoot := testutil.CopyFixtureDir(t, workingRoot)
	binaryPath := buildYouCLIBinary(t)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)
	isolatedHome := t.TempDir()

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), javascriptFactoryRunTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--factory", "./ordered_pipeline.js",
		"--with-mock-workers="+mockWorkersPath,
		"--output", "primary",
		"--no-record",
		"--server", serverURL,
	)
	cmd.Dir = isolatedRoot
	cmd.Env = javascriptFactoryRunEnvironmentForHome(isolatedHome)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	assertJavaScriptPipelineProcessResult(t, ctx, runErr, stdout.String(), stderr.String())
	result := decodeSingleJavaScriptFactoryRunResult(t, stdout.String())
	assertOrderedJavaScriptPipelineResult(t, result)
}

func assertJavaScriptPipelineProcessResult(t *testing.T, ctx context.Context, runErr error, stdout, stderr string) {
	t.Helper()
	if ctx.Err() != nil {
		t.Fatalf("you run ordered pipeline timed out after %s: %v\nstdout:\n%s\nstderr:\n%s", javascriptFactoryRunTimeout, ctx.Err(), stdout, stderr)
	}
	if runErr != nil {
		t.Fatalf("you run ordered pipeline exited non-zero: %v\nstdout:\n%s\nstderr:\n%s", runErr, stdout, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", stderr)
	}
}

func assertOrderedJavaScriptPipelineResult(t *testing.T, result factoryapi.FactorySessionSyncExecutionResponse) {
	t.Helper()
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted ||
		result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("terminal outcome = %s (%s), want COMPLETED (SUCCEEDED)", result.SyncOutcome, result.Status)
	}
	if result.Result == nil || result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want exactly one FINAL Factory Session result", result.Result)
	}
	if result.Result.PrimaryResult == nil || len(*result.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.Result.PrimaryResult)
	}
	part, err := (*result.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	var evidence orderedJavaScriptPipelineEvidence
	encoded, err := json.Marshal(part.Json)
	if err != nil {
		t.Fatalf("encode ordered pipeline evidence: %v", err)
	}
	if err := json.Unmarshal(encoded, &evidence); err != nil {
		t.Fatalf("decode ordered pipeline evidence: %v", err)
	}
	assertOrderedJavaScriptPipelineEvidence(t, evidence)
}

type orderedJavaScriptPipelineEvidence struct {
	FinalValue string `json:"finalValue"`
	Stages     []struct {
		Stage        string `json:"stage"`
		ChildIndex   int    `json:"childIndex"`
		DispatchID   string `json:"dispatchId"`
		ResultStatus string `json:"resultStatus"`
		Response     string `json:"response"`
	} `json:"stages"`
	Dependency struct {
		PriorDispatchID    string `json:"priorDispatchId"`
		ObservedByStageTwo bool   `json:"observedByStageTwo"`
	} `json:"dependency"`
}

func assertOrderedJavaScriptPipelineEvidence(t *testing.T, evidence orderedJavaScriptPipelineEvidence) {
	t.Helper()
	if evidence.FinalValue != orderedJavaScriptPipelineResult {
		t.Fatalf("final customer value = %q, want %q", evidence.FinalValue, orderedJavaScriptPipelineResult)
	}
	if len(evidence.Stages) != 2 {
		t.Fatalf("stage evidence count = %d, want exactly 2", len(evidence.Stages))
	}
	for index, wantStage := range []string{"stage-one", "stage-two"} {
		stage := evidence.Stages[index]
		if stage.Stage != wantStage || stage.ChildIndex != index+1 || stage.DispatchID == "" || stage.ResultStatus != "COMPLETED" {
			t.Fatalf("stage evidence[%d] = %#v, want %s child %d with one completed dispatch result", index, stage, wantStage, index+1)
		}
		if !strings.Contains(stage.Response, ":"+wantStage+":") {
			t.Fatalf("stage evidence[%d] response = %q, want deterministic mock response for %s", index, stage.Response, wantStage)
		}
	}
	if evidence.Stages[0].DispatchID == evidence.Stages[1].DispatchID {
		t.Fatalf("stage dispatch IDs are duplicated: %q", evidence.Stages[0].DispatchID)
	}
	if evidence.Dependency.PriorDispatchID != evidence.Stages[0].DispatchID || !evidence.Dependency.ObservedByStageTwo {
		t.Fatalf("stage-two dependency evidence = %#v, want completed stage-one dispatch %q", evidence.Dependency, evidence.Stages[0].DispatchID)
	}
}

func TestJavaScriptFactoryRun_RealCLIUsesMockWorkersAndReturnsPrimaryResult(t *testing.T) {
	t.Parallel()

	workingRoot := support.LegacyFixtureDir(t, "dynamic")
	isolatedRoot := testutil.CopyFixtureDir(t, workingRoot)
	binaryPath := buildYouCLIBinary(t)
	mockWorkersPath := writeDefaultMockWorkersConfig(t)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve loopback port: %v", err)
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	ctx, cancel := context.WithTimeout(context.Background(), javascriptFactoryRunTimeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--factory", "./basic.js",
		"--with-mock-workers",
		"--no-record",
		"--server", serverURL,
		mockWorkersPath,
	)
	cmd.Dir = isolatedRoot
	cmd.Env = javascriptFactoryRunEnvironment(t)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("you run --factory timed out after %s: %v\nstdout:\n%s\nstderr:\n%s", javascriptFactoryRunTimeout, ctx.Err(), stdout.String(), stderr.String())
	}
	if runErr != nil {
		t.Fatalf("you run --factory exited non-zero: %v\nstdout:\n%s\nstderr:\n%s", runErr, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty stderr on successful JSON invocation", stderr.String())
	}

	result := decodeSingleJavaScriptFactoryRunResult(t, stdout.String())
	if result.SyncOutcome != factoryapi.FactorySessionSyncExecutionOutcomeCompleted ||
		result.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("terminal outcome = %s (%s), want exactly one COMPLETED (SUCCEEDED) outcome", result.SyncOutcome, result.Status)
	}
	if result.Result == nil || result.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("result = %#v, want one FINAL Factory Session result", result.Result)
	}
	if result.Result.PrimaryResult == nil || len(*result.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want exactly one content part", result.Result.PrimaryResult)
	}
	part, err := (*result.Result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result content part: %v", err)
	}
	if got, ok := part.Json.(string); !ok || got != "<SUCCESS>" {
		t.Fatalf("primary result = %#v, want exact string %q", part.Json, "<SUCCESS>")
	}
	if result.EffectivePolicy == nil || result.EffectivePolicy.AdditionalProperties["allowNetwork"] != false {
		t.Fatalf("effective policy = %#v, want public network disabled", result.EffectivePolicy)
	}
}

func decodeSingleJavaScriptFactoryRunResult(t *testing.T, stdout string) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stdout))
	var result factoryapi.FactorySessionSyncExecutionResponse
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode CLI stdout as Factory Session result: %v\nstdout:\n%s", err, stdout)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout contained more than one terminal JSON result: %v\nstdout:\n%s", err, stdout)
	}
	return result
}

func javascriptFactoryRunEnvironment(t *testing.T) []string {
	t.Helper()

	return javascriptFactoryRunEnvironmentForHome(t.TempDir())
}

func javascriptFactoryRunEnvironmentForHome(isolatedHome string) []string {
	environment := make([]string, 0, len(os.Environ())+6)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upperName := strings.ToUpper(name)
		if upperName == "HOME" || upperName == "USERPROFILE" || upperName == "APPDATA" ||
			upperName == "LOCALAPPDATA" || upperName == "XDG_CONFIG_HOME" || upperName == "XDG_CACHE_HOME" ||
			strings.HasPrefix(upperName, "YOU_") || strings.Contains(upperName, "API_KEY") ||
			strings.Contains(upperName, "TOKEN") || strings.Contains(upperName, "SECRET") ||
			strings.Contains(upperName, "CREDENTIAL") || strings.HasPrefix(upperName, "AWS_") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"HOME="+isolatedHome,
		"USERPROFILE="+isolatedHome,
		"APPDATA="+filepath.Join(isolatedHome, "AppData", "Roaming"),
		"LOCALAPPDATA="+filepath.Join(isolatedHome, "AppData", "Local"),
		"XDG_CONFIG_HOME="+filepath.Join(isolatedHome, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(isolatedHome, ".cache"),
	)
}
