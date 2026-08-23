package deep_research

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedDeepResearchStaleNamedInvocationRefreshesThroughCustomerProcess
// proves the automatic upgrade route at the customer boundary. The stale
// definition is deliberately written after the first initialization, so the
// second Process.Execute must reconcile it before named-command composition.
func TestPackagedDeepResearchStaleNamedInvocationRefreshesThroughCustomerProcess(t *testing.T) {
	fixture := prepareStaleDeepResearchFixture(t)
	assertDeepResearchExternalNames(t, fixture.stalePayload, "model-provider", "model", "researchDepth", "maxSubagents")

	commandArgs := []string{
		"you", "run", "--named", factorydefinitions.PackagedDeepResearchFactoryName,
		"--maxSubagents", "0", "What is a Petri net?",
	}
	inputs := support.FakeInputs(t.Context(), commandArgs)
	inputs.Input.Env = fixture.environment
	inputs.Input.WorkingDirectory = fixture.workingDirectory
	invocationErr := fixture.process.Execute(inputs.Input)
	if invocationErr != nil && strings.Contains(invocationErr.Error(), "cli.composition.long-name-collision") {
		t.Fatalf(
			"refreshed named deep-research invocation still returned the stale composition collision: %v\nstdout:\n%s\nstderr:\n%s",
			invocationErr,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	activePayload, err := os.ReadFile(fixture.factoryPath)
	if err != nil {
		t.Fatalf("read refreshed deep-research materialization: %v", err)
	}
	assertDeepResearchExternalNames(t, activePayload, "research-model-provider", "research-model", "research-depth", "max-subagents")
	assertDeepResearchExternalNamesAbsent(t, activePayload, "model-provider", "model", "researchDepth", "maxSubagents")

	backupRoot := filepath.Join(filepath.Dir(filepath.Dir(fixture.factoryDir)), ".you-packaged-backups")
	backupEntries, err := os.ReadDir(backupRoot)
	if err != nil {
		t.Fatalf("read packaged Factory backup root %s: %v", backupRoot, err)
	}
	var backupDirs []string
	for _, entry := range backupEntries {
		if entry.IsDir() {
			backupDirs = append(backupDirs, filepath.Join(backupRoot, entry.Name()))
		}
	}
	if len(backupDirs) != 1 {
		t.Fatalf("packaged Factory backup directories = %v, want exactly one preserved prior copy", backupDirs)
	}
	backupPayload, err := os.ReadFile(filepath.Join(backupDirs[0], factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read preserved stale deep-research backup: %v", err)
	}
	if !bytes.Equal(backupPayload, fixture.stalePayload) {
		t.Fatalf("preserved stale backup differs from the complete pre-refresh factory.json")
	}
	if filepath.Clean(backupDirs[0]) == filepath.Clean(fixture.factoryDir) {
		t.Fatalf("backup path %s is still the active Factory path", backupDirs[0])
	}
	structuredArgs := []string{
		"you", "--json", "run", "--named", factorydefinitions.PackagedDeepResearchFactoryName,
		"--maxSubagents", "0", "What is a Petri net?",
	}
	structuredInputs := support.FakeInputs(t.Context(), structuredArgs)
	structuredInputs.Input.Env = fixture.environment
	structuredInputs.Input.WorkingDirectory = fixture.workingDirectory
	if err := fixture.process.Execute(structuredInputs.Input); err != nil {
		t.Fatalf(
			"structured refreshed named deep-research invocation error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			structuredInputs.Stdout(),
			structuredInputs.Stderr(),
		)
	}
	structuredResponse := support.DecodeInvocationResponseJSON(t, structuredInputs.Stdout())
	if structuredResponse.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("structured refreshed invocation status = %q, want COMPLETED", structuredResponse.Status)
	}
	if fixture.provider.CallCount() != 2 {
		t.Fatalf("provider calls = %d, want one lead synthesis for each command after --maxSubagents 0", fixture.provider.CallCount())
	}
	request := fixture.provider.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("provider command = %q, want codex", request.Command)
	}
	if !strings.Contains(string(request.Stdin), "What is a Petri net?") {
		t.Fatalf("provider prompt = %q, want the exact customer topic", string(request.Stdin))
	}
	t.Logf(
		"customer command %q refreshed %s and reached provider; backup=%s; invocation_error=%v; stdout=%q; stderr=%q; structured_command=%q; structured_status=%q; structured_provider_result=%t",
		strings.Join(commandArgs, " "),
		fixture.factoryPath,
		backupDirs[0],
		invocationErr,
		inputs.Stdout(),
		inputs.Stderr(),
		strings.Join(structuredArgs, " "),
		structuredResponse.Status,
		strings.Contains(structuredInputs.Stdout(), "deep research provider reached"),
	)
}

type staleDeepResearchFixture struct {
	process          support.Process
	provider         *testutil.ProviderCommandRunner
	environment      []string
	workingDirectory string
	factoryDir       string
	factoryPath      string
	stalePayload     []byte
}

func prepareStaleDeepResearchFixture(t *testing.T) staleDeepResearchFixture {
	t.Helper()
	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	providerResult := platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(`{"answer":"deep research provider reached"}`),
	}
	provider := testutil.NewProviderCommandRunner(providerResult, providerResult)
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: provider,
	})
	if err != nil {
		t.Fatalf("root.BuildProcess() error = %v", err)
	}
	support.CleanupProcess(t, process)
	environment := deepResearchCustomerEnvironment(homeDir)
	factoryDir := support.InstallPackagedFactoryWithProcess(
		t,
		process,
		environment,
		workingDirectory,
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	factoryPath := filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile)
	currentPayload, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read current deep-research materialization: %v", err)
	}
	stalePayload := staleDeepResearchPayload(currentPayload)
	if err := os.WriteFile(factoryPath, stalePayload, 0o600); err != nil {
		t.Fatalf("write stale deep-research materialization: %v", err)
	}
	return staleDeepResearchFixture{
		process:          process,
		provider:         provider,
		environment:      environment,
		workingDirectory: workingDirectory,
		factoryDir:       factoryDir,
		factoryPath:      factoryPath,
		stalePayload:     stalePayload,
	}
}

func deepResearchCustomerEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch {
		case strings.EqualFold(name, "HOME"), strings.EqualFold(name, "USERPROFILE"),
			strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL_PROVIDER"), strings.EqualFold(name, "YOU_DEFAULT_WORKER_MODEL"):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(
		environment,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"YOU_DEFAULT_WORKER_MODEL_PROVIDER=CODEX",
		"YOU_DEFAULT_WORKER_MODEL=gpt-5",
	)
}

func staleDeepResearchPayload(currentPayload []byte) []byte {
	stalePayload := append([]byte(nil), currentPayload...)
	for _, replacement := range []struct{ current, stale string }{
		{current: "research-model-provider", stale: "model-provider"},
		{current: "research-model", stale: "model"},
		{current: "research-depth", stale: "researchDepth"},
		{current: "max-subagents", stale: "maxSubagents"},
	} {
		stalePayload = bytes.ReplaceAll(stalePayload, []byte(replacement.current), []byte(replacement.stale))
	}
	return stalePayload
}

func assertDeepResearchExternalNames(t testing.TB, payload []byte, names ...string) {
	t.Helper()
	content := string(payload)
	for _, name := range names {
		if !strings.Contains(content, `"externalName": "`+name+`"`) {
			t.Fatalf("definition is missing externalName %q", name)
		}
	}
}

func assertDeepResearchExternalNamesAbsent(t testing.TB, payload []byte, names ...string) {
	t.Helper()
	content := string(payload)
	for _, name := range names {
		if strings.Contains(content, `"externalName": "`+name+`"`) {
			t.Fatalf("definition still contains stale externalName %q", name)
		}
	}
}

// TestPackagedDeepResearchRequiredInputCompletes proves that invoking the
// packaged @you/deep-research Factory with only the required research topic
// completes through structured provider responses, runs the expected specialist-and-lead dispatch
// sequence for a delegating topic shape, and returns a primary synthesis that
// reflects the submitted topic.
func TestPackagedDeepResearchRequiredInputCompletes(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research required topic %d with enough breadth for specialist delegation",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	runner := testutil.NewProviderCommandRunner(
		providerResult(support.CodexSuccessStdout(`{"evidence":"technical specialist evidence"}`)),
		providerResult(support.CodexSuccessStdout(`{"evidence":"tradeoff specialist evidence"}`)),
		providerResult(support.CodexSuccessStdout(`{"answer":"lead-research-synthesis: synthesized specialist evidence"}`)),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})

	args := map[string]any{"topic": topic}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-required-input",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		resultJSON, _ := json.Marshal(response.Result)
		session := support.GetJSON[map[string]any](t, server.URL()+"/factory-sessions/"+response.SessionId)
		sessionJSON, _ := json.Marshal(session)
		dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
		dispatchJSON, _ := json.Marshal(dispatches)
		t.Fatalf("session status = %q, want SUCCEEDED; result = %s; session = %s; dispatches = %s; response = %#v", response.Status, resultJSON, sessionJSON, dispatchJSON, response)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result part", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	primaryText := string(primary)
	for _, want := range []string{
		topic,
		`"researchDepth":2`,
		`"maxSubagents":2`,
		"lead-research-synthesis",
		`"role":"technical"`,
		`"role":"tradeoffs"`,
	} {
		if !strings.Contains(primaryText, want) {
			t.Fatalf("primary result = %s, want substring %q", primaryText, want)
		}
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf(
			"dispatch count = %d, want two bounded specialist dispatches and one lead synthesis",
			len(dispatches.Dispatches),
		)
	}
	labels := make(map[string]bool)
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			labels[*dispatch.Label] = true
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
		}
	}
	for _, want := range []string{
		"research-specialist-technical",
		"research-specialist-tradeoffs",
		"lead-research-synthesis",
	} {
		if !labels[want] {
			t.Fatalf("dispatch labels = %#v, want %q", labels, want)
		}
	}
	requests := runner.Requests()
	if len(requests) != 3 {
		t.Fatalf("provider calls = %d, want two specialists and one lead synthesis", len(requests))
	}
	leadInput := string(requests[2].Stdin) + " " + strings.Join(requests[2].Args, " ")
	for _, evidence := range []string{"technical specialist evidence", "tradeoff specialist evidence"} {
		if !strings.Contains(leadInput, evidence) {
			t.Fatalf("lead request does not contain validated specialist evidence %q: %s", evidence, leadInput)
		}
	}
}

// TestPackagedDeepResearchOptionalInputsReachWorkers proves that optional
// deep-research overrides such as research depth, specialist cap, and approved
// model execution selection reach structured provider workers and are observable on dispatch
// execution selection and the primary synthesis result.
func TestPackagedDeepResearchOptionalInputsReachWorkers(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research optional overrides %d with enough breadth for specialist delegation",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	runner := testutil.NewProviderCommandRunner(
		providerResult(support.CodexSuccessStdout(`{"evidence":"optional specialist evidence"}`)),
		providerResult(support.CodexSuccessStdout(`{"answer":"lead-research-synthesis: optional synthesized evidence"}`)),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})

	args := map[string]any{
		"topic":           topic,
		"researchDepth":   3,
		"maxSubagents":    1,
		"modelProvider":   "CODEX",
		"model":           "gpt-5",
		"reasoningEffort": "medium",
	}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-optional-inputs",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("session status = %q, want SUCCEEDED; response = %#v", response.Status, response)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one synthesized result part", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	primaryText := string(primary)
	for _, want := range []string{
		topic,
		`"researchDepth":3`,
		`"maxSubagents":1`,
		`"modelProvider":"CODEX"`,
		`"model":"gpt-5"`,
		`"reasoningEffort":"medium"`,
		"lead-research-synthesis",
	} {
		if !strings.Contains(primaryText, want) {
			t.Fatalf("primary result = %s, want substring %q", primaryText, want)
		}
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf(
			"dispatch count = %d, want one bounded specialist dispatch and one lead synthesis",
			len(dispatches.Dispatches),
		)
	}
	labels := make(map[string]bool)
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			labels[*dispatch.Label] = true
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
		}
		if dispatch.ModelProvider == nil || !strings.EqualFold(*dispatch.ModelProvider, "CODEX") ||
			dispatch.Model == nil || *dispatch.Model != "gpt-5" ||
			dispatch.ReasoningEffort == nil || *dispatch.ReasoningEffort != "medium" {
			t.Fatalf(
				"dispatch execution selection = provider=%#v model=%#v reasoning=%#v, want approved overrides",
				dispatch.ModelProvider,
				dispatch.Model,
				dispatch.ReasoningEffort,
			)
		}
	}
	if !labels["research-specialist-technical"] || !labels["lead-research-synthesis"] {
		t.Fatalf(
			"dispatch labels = %#v, want technical specialist and lead synthesis",
			labels,
		)
	}
	if labels["research-specialist-tradeoffs"] {
		t.Fatalf("dispatch labels = %#v, want tradeoffs specialist omitted when maxSubagents is 1", labels)
	}
	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want one specialist and one lead synthesis", len(requests))
	}
	leadInput := string(requests[1].Stdin) + " " + strings.Join(requests[1].Args, " ")
	if !strings.Contains(leadInput, "optional specialist evidence") {
		t.Fatalf("lead request does not contain validated specialist evidence: %s", leadInput)
	}
}

// TestPackagedDeepResearchRetriesSchemaMismatchBeforeSynthesis proves that a
// completed provider response with the wrong structured shape is retried once
// before its validated evidence is passed to lead synthesis.
func TestPackagedDeepResearchRetriesSchemaMismatchBeforeSynthesis(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research schema retry %d with enough breadth for specialist delegation",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	runner := testutil.NewProviderCommandRunner(
		providerResult(support.CodexSuccessStdout(`{"wrong":"not evidence"}`)),
		providerResult(support.CodexSuccessStdout(`{"evidence":"recovered specialist evidence"}`)),
		providerResult(support.CodexSuccessStdout(`{"answer":"recovered evidence synthesis"}`)),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})

	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-schema-retry",
		map[string]any{"topic": topic, "maxSubagents": 1},
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		resultJSON, _ := json.Marshal(response.Result)
		t.Fatalf("session status = %q, want SUCCEEDED; result = %s; response = %#v", response.Status, resultJSON, response)
	}
	primary, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	if !strings.Contains(string(primary), "recovered evidence synthesis") {
		t.Fatalf("primary result = %s, want recovered evidence synthesis", primary)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider calls = %d, want failed specialist, one bounded retry, and lead synthesis", runner.CallCount())
	}
	requests := runner.Requests()
	leadInput := string(requests[2].Stdin) + " " + strings.Join(requests[2].Args, " ")
	if !strings.Contains(leadInput, "recovered specialist evidence") || strings.Contains(leadInput, "not evidence") {
		t.Fatalf("lead request evidence = %s, want only the recovered validated evidence", leadInput)
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want initial specialist, bounded retry, and lead synthesis", len(dispatches.Dispatches))
	}
	statuses := make(map[string]factoryapi.FactoryDispatchStatus)
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Label != nil {
			statuses[*dispatch.Label] = dispatch.Status
		}
	}
	for label, wantStatus := range map[string]factoryapi.FactoryDispatchStatus{
		"research-specialist-technical":       factoryapi.FactoryDispatchStatusFAILED,
		"research-specialist-technical-retry": factoryapi.FactoryDispatchStatusCOMPLETED,
		"lead-research-synthesis":             factoryapi.FactoryDispatchStatusCOMPLETED,
	} {
		if statuses[label] != wantStatus {
			t.Fatalf("dispatch statuses = %#v, want %q=%q", statuses, label, wantStatus)
		}
	}
}

// TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome proves that a
// configured mock-worker rejection during packaged @you/deep-research invocation
// returns a failed public terminal outcome without a completed success primary
// result attributable to the failing run.
func TestPackagedDeepResearchWorkerFailureReturnsFailedOutcome(t *testing.T) {
	topic := fmt.Sprintf(
		"functional packaged deep research worker failure %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedDeepResearchFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		MockWorkersConfig:         packagedDeepResearchRejectingMockWorkersConfig(),
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--provider", "CODEX", "--model", "gpt-5"},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: packagedDeepResearchFailingCommandRunner{},
		},
	})

	args := map[string]any{
		"topic":        topic,
		"maxSubagents": 0,
	}
	response := startPackagedDeepResearchInvocation(
		t,
		server,
		factoryDir,
		"packaged-deep-research-worker-failure",
		args,
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusFailed {
		t.Fatalf("session status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.Result != nil && response.Result.PrimaryResult != nil && len(*response.Result.PrimaryResult) > 0 {
		t.Fatalf("primary result = %#v, want no completed success primary result after worker failure", response.Result)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("sessionId is empty, want durable JavaScript session ID")
	}

	dispatches := listFactorySessionDispatches(t, server.URL(), response.SessionId)
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf(
			"dispatch count = %d, want one lead synthesis dispatch when maxSubagents is 0",
			len(dispatches.Dispatches),
		)
	}
	dispatch := dispatches.Dispatches[0]
	if dispatch.Label == nil || *dispatch.Label != "lead-research-synthesis" {
		t.Fatalf("dispatch label = %#v, want lead-research-synthesis", dispatch.Label)
	}
	if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || strings.TrimSpace(dispatch.FailureDetail.Message) == "" {
		t.Fatalf("dispatch failureDetail = %#v, want stable public failure record", dispatch.FailureDetail)
	}
}

type packagedDeepResearchFailingCommandRunner struct{}

func (packagedDeepResearchFailingCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("packaged deep research provider failure")
}

func packagedDeepResearchRejectingMockWorkersConfig() *workers.MockWorkersConfig {
	exitCode := 7
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			RunType: workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stderr:   "packaged deep research mock worker failure",
				ExitCode: &exitCode,
			},
		}},
	}
}

func startPackagedDeepResearchInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	factoryDir string,
	requestID string,
	args map[string]any,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factory := support.GetJSON[factoryapi.Factory](t, server.URL()+"/factory-sessions/~default/factory")
	_ = factoryDir
	javascript := factory.Orchestrator.Javascript
	return postJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		server.URL()+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: requestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
				InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
					Dialect:      javascript.Dialect,
					Entrypoint:   javascript.Entrypoint,
					InlineSource: *javascript.InlineSource,
					Metadata:     javascript.Metadata,
				},
			},
			Args:         &args,
			Orchestrator: factory.Orchestrator,
		},
		"start packaged deep-research invocation",
	)
}

func providerResult(stdout []byte) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: stdout}
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var payload bytes.Buffer
		_, _ = payload.ReadFrom(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, payload.String())
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}

func listFactorySessionDispatches(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()
	response, err := http.Get(strings.TrimSuffix(serverURL, "/") + "/factory-sessions/" + sessionID + "/dispatches")
	if err != nil {
		t.Fatalf("GET /factory-sessions/%s/dispatches: %v", sessionID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var payload bytes.Buffer
		_, _ = payload.ReadFrom(response.Body)
		t.Fatalf(
			"GET /factory-sessions/%s/dispatches status = %d, want 200: %s",
			sessionID,
			response.StatusCode,
			payload.String(),
		)
	}
	var decoded factoryapi.ListFactorySessionDispatchesResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode dispatch list: %v", err)
	}
	return decoded
}
