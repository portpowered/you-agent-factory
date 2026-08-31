package parameters_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parameterProcessCloseTimeout = 5 * time.Second

type parameterProcessFixture struct {
	observerRuntime   *parameterRuntime
	handlerRuntime    *parameterRuntime
	observations      *cliObservationLog
	submissions       *invocationSubmissionObservation
	providerRunner    *support.ShapedProviderCommandRunner
	lifecycleEffects  *atomic.Int32
	operatorMutations *atomic.Int32
}

var parameterProcesses *parameterProcessFixture

// TestMain constructs the two immutable process variants once for the package.
// The observer root remains separate because CLIObserver intercepts handler
// execution; all handler cases, including the missing-asset witness, share the
// handler root with fresh inputs and serialized Process.Execute calls.
func TestMain(m *testing.M) {
	fixture, err := buildParameterProcessFixture()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build parameter functional process fixture: %v\n", err)
		os.Exit(1)
	}
	parameterProcesses = fixture

	exitCode := m.Run()
	if closeErr := fixture.close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "close parameter functional process fixture: %v\n", closeErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

func buildParameterProcessFixture() (*parameterProcessFixture, error) {
	observations := &cliObservationLog{}
	submissions := &invocationSubmissionObservation{}
	providerRunner := support.NewShapedProviderCommandRunner(
		successfulProviderResults(64)...,
	)
	operatorMutations := &atomic.Int32{}

	observerProcess, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{
		CLIObserver: observations.observe,
		OperatorSettingsFileSystem: mutationTrackingOperatorSettingsFileSystem{
			mutations: operatorMutations,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build observer process: %w", err)
	}

	lifecycleEffects := &atomic.Int32{}
	handlerRuntime, err := newParameterRuntime(serviceedges.Edges{
		ProviderCommandRunner: providerRunner,
		SubmissionRecorder:    submissions.observe,
		APIServerStarter: func(context.Context, platformhttpserver.StartRequest) error {
			lifecycleEffects.Add(1)
			return nil
		},
		BrowserOpener: func(context.Context, string) error {
			lifecycleEffects.Add(1)
			return nil
		},
		RuntimeHostObserver: func(factorysessions.RuntimeHostBinding) {
			lifecycleEffects.Add(1)
		},
		FactorySessionIDGenerator: func() string {
			lifecycleEffects.Add(1)
			return uuid.NewString()
		},
	})
	if err != nil {
		_ = observerProcess.Close(context.Background())
		return nil, fmt.Errorf("build handler process: %w", err)
	}

	return &parameterProcessFixture{
		observerRuntime:   newParameterRuntimeFromProcess(observerProcess),
		handlerRuntime:    handlerRuntime,
		observations:      observations,
		submissions:       submissions,
		providerRunner:    providerRunner,
		lifecycleEffects:  lifecycleEffects,
		operatorMutations: operatorMutations,
	}, nil
}

// parameterRuntime owns one package-level production graph. Its execution
// lock keeps mutable service state from overlapping while each caller still
// supplies fresh input, streams, environment, and working-directory state.
type parameterRuntime struct {
	process support.ApplicationProcess

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func newParameterRuntime(edges serviceedges.Edges) (*parameterRuntime, error) {
	process, err := support.BuildProcessWithContext(context.Background(), edges)
	if err != nil {
		return nil, err
	}
	return newParameterRuntimeFromProcess(process), nil
}

func newParameterRuntimeFromProcess(process support.ApplicationProcess) *parameterRuntime {
	return &parameterRuntime{process: process}
}

func (runtime *parameterRuntime) execute(input root.Input) error {
	if runtime == nil || runtime.process == nil {
		return errors.New("parameter functional runtime is nil")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return errors.New("parameter functional runtime is closed")
	}
	return runtime.process.Execute(input)
}

func (runtime *parameterRuntime) close(ctx context.Context) error {
	if runtime == nil || runtime.process == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runtime.closeOnce.Do(func() {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		runtime.closed = true
		runtime.closeErr = runtime.process.Close(ctx)
	})
	return runtime.closeErr
}

func successfulProviderResults(count int) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for index := range results {
		results[index] = platformprocess.CommandResult{Stdout: []byte("Done. COMPLETE")}
	}
	return results
}

func (fixture *parameterProcessFixture) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), parameterProcessCloseTimeout)
	defer cancel()

	var closeErr error
	runtimes := []struct {
		name    string
		runtime *parameterRuntime
	}{
		{name: "observer", runtime: fixture.observerRuntime},
		{name: "handler", runtime: fixture.handlerRuntime},
	}
	for _, entry := range runtimes {
		name, runtime := entry.name, entry.runtime
		if err := runtime.close(ctx); err != nil {
			closeErr = fmt.Errorf("%s process: %w", name, err)
		}
	}
	return closeErr
}

type cliObservationLog struct {
	mu      sync.Mutex
	results []cliobservation.Result
}

func (log *cliObservationLog) observe(observed platformprocess.CLIObservation) error {
	result, err := cliobservation.Decode(observed)
	if err != nil {
		return err
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	log.results = append(log.results, result)
	return nil
}

func (log *cliObservationLog) snapshot() []cliobservation.Result {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]cliobservation.Result(nil), log.results...)
}

func parameterInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.WorkingDirectory = t.TempDir()
	inputs.Input.Env = spineEnvironment(inputs.Input.Env, t.TempDir())
	return inputs
}

func executeParameterObservation(t *testing.T, args []string) cliobservation.Result {
	t.Helper()
	if parameterProcesses == nil {
		t.Fatal("parameter process fixture is not initialized")
	}
	before := len(parameterProcesses.observations.snapshot())
	inputs := parameterInputs(t, args)
	if err := parameterProcesses.observerRuntime.execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(parser observation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	results := parameterProcesses.observations.snapshot()
	if got := len(results) - before; got != 1 {
		t.Fatalf("detached CLI observation delta = %d, want 1", got)
	}
	return results[before]
}

const (
	spinePositionalValue = "Ship the café résumé plan"
	spinePriorityValue   = "urgent"
	spineCallbackValue   = "https://example.com/callback?token=abc123&scope=read%3Dwrite"
	spineFirstTagValue   = "alpha"
	spineSecondTagValue  = "beta"
	spineMetadataValue   = `{"user":{"name":"alice","roles":["admin","editor"]},"version":2}`
	spineNullableValue   = "null"
	spineEmptyString     = `""`
	spineEmptyObject     = `{}`
	spineEmptyArray      = `[]`
)

// TestCLIParameterReusableProcessSpine establishes the shared process shape
// for the parameter package. The reusable root-built processes are immutable
// and their customer invocations run in lexical order with fresh inputs.
func TestCLIParameterReusableProcessSpine(t *testing.T) {
	if parameterProcesses == nil {
		t.Fatal("parameter process fixture is not initialized")
	}

	t.Run("observer root parses generic flags", func(t *testing.T) {
		testObserverRootParsesGenericFlags(t)
	})

	t.Run("full handler submits combined signature once", func(t *testing.T) {
		testFullHandlerSubmitsCombinedSignature(t)
	})
}

func testObserverRootParsesGenericFlags(t *testing.T) {
	t.Helper()
	first := executeSpineObservation(t, []string{
		"you", "--server", "https://factory.example", "-v",
		"worker-sessions", "list", "--state", "RESERVED", "--state", "RUNNING", "--json",
	})
	if first.Parse.CommandPath != "you worker-sessions list" || len(first.Parse.Positionals) != 0 {
		t.Fatalf("first observed parse = %#v, want worker-sessions list with no positionals", first.Parse)
	}
	assertSpineParsedFlag(t, first, "server", true, "https://factory.example")
	assertSpineParsedFlag(t, first, "verbose", true, "true")
	assertSpineParsedFlag(t, first, "json", true, "true")
	assertSpineParsedFlag(t, first, "state", true, "[RESERVED,RUNNING]")

	second := executeSpineObservation(t, []string{
		"you", "--server", "https://second.example", "worker-sessions", "list", "--state", "COMPLETED",
	})
	if second.Parse.CommandPath != "you worker-sessions list" {
		t.Fatalf("second observed command path = %q, want you worker-sessions list", second.Parse.CommandPath)
	}
	assertSpineParsedFlag(t, second, "server", true, "https://second.example")
	assertSpineParsedFlag(t, second, "state", true, "[COMPLETED]")
	verbose, found := cliobservation.Flag(second.Parse, "verbose")
	if !found || verbose.Changed {
		t.Fatalf("second observed --verbose parse = %#v found=%v, want unchanged", verbose, found)
	}
	firstState, found := cliobservation.Flag(first.Parse, "state")
	if !found || firstState.Value != "[RESERVED,RUNNING]" {
		t.Fatalf("first detached state observation = %#v found=%v, want [RESERVED,RUNNING]", firstState, found)
	}
}

func testFullHandlerSubmitsCombinedSignature(t *testing.T) {
	t.Helper()
	beforeSubmissions := len(parameterProcesses.submissions.snapshot())
	beforeProviderCalls := parameterProcesses.providerRunner.CallCount()
	factoryDir := scaffoldCombinedInvocationFactory(t)
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	inputs := spineInputs(t, combinedSignatureArgs(factoryPath))
	if err := parameterProcesses.handlerRuntime.execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(combined parameter invocation) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}

	records := parameterProcesses.submissions.snapshot()
	if got := len(records) - beforeSubmissions; got != 1 {
		t.Fatalf("canonical submission delta = %d, want 1; records=%#v", got, records)
	}
	arguments := records[beforeSubmissions].Request.InvocationArguments
	if arguments == nil {
		t.Fatal("submitted invocation arguments = nil")
	}
	assertCombinedSignatureArguments(t, arguments)
	if got := parameterProcesses.providerRunner.CallCount() - beforeProviderCalls; got != 1 {
		t.Fatalf("controlled provider command call delta = %d, want 1", got)
	}
}

func combinedSignatureArgs(factoryPath string) []string {
	return []string{
		"you", "run", "--factory", factoryPath, "--no-record", spinePositionalValue,
		"--priority=" + spinePriorityValue, "--callback=" + spineCallbackValue,
		"--tag=" + spineFirstTagValue, "--tag=" + spineSecondTagValue,
		"--metadata=" + spineMetadataValue, "--nullable=" + spineNullableValue,
		"--emptyString=" + spineEmptyString, "--emptyObject=" + spineEmptyObject,
		"--emptyArray=" + spineEmptyArray,
	}
}

func assertCombinedSignatureArguments(t *testing.T, arguments *work.InvocationArguments) {
	t.Helper()
	assertSpineArgument(t, arguments, "input", []string{spinePositionalValue}, work.ArgumentSourceKindPositional)
	assertSpineArgument(t, arguments, "priority", []string{spinePriorityValue}, work.ArgumentSourceKindNamed)
	assertSpineArgument(t, arguments, "callback", []string{spineCallbackValue}, work.ArgumentSourceKindNamed)
	assertSpineArgument(t, arguments, "tag", []string{spineFirstTagValue, spineSecondTagValue}, work.ArgumentSourceKindNamed)
	assertSpineJSONArgument(t, arguments, "metadata", spineMetadataValue)
	assertSpineJSONArgument(t, arguments, "nullable", spineNullableValue)
	assertSpineJSONArgument(t, arguments, "emptyString", spineEmptyString)
	assertSpineJSONArgument(t, arguments, "emptyObject", spineEmptyObject)
	assertSpineJSONArgument(t, arguments, "emptyArray", spineEmptyArray)

	values := []string{
		arguments.Arguments["nullable"].Values[0], arguments.Arguments["emptyString"].Values[0],
		arguments.Arguments["emptyObject"].Values[0], arguments.Arguments["emptyArray"].Values[0],
	}
	for left, value := range values {
		for right := left + 1; right < len(values); right++ {
			if value == values[right] {
				t.Fatalf("JSON values at indexes %d and %d normalized to %q", left, right, value)
			}
		}
	}
}

func executeSpineObservation(t *testing.T, args []string) cliobservation.Result {
	t.Helper()
	return executeParameterObservation(t, args)
}

func assertSpineParsedFlag(
	t *testing.T,
	result cliobservation.Result,
	name string,
	wantChanged bool,
	wantValue string,
) {
	t.Helper()
	flag, found := cliobservation.Flag(result.Parse, name)
	if !found || flag.Changed != wantChanged || flag.Value != wantValue {
		t.Fatalf(
			"observed --%s parse = %#v found=%v, want changed=%v value=%q",
			name,
			flag,
			found,
			wantChanged,
			wantValue,
		)
	}
}

func assertSpineArgument(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	wantValues []string,
	wantKind work.ArgumentSourceKind,
) {
	t.Helper()
	argument, found := arguments.Arguments[name]
	if !found {
		t.Fatalf("invocation argument %q missing from %#v", name, arguments.Arguments)
	}
	if len(argument.Values) != len(wantValues) {
		t.Fatalf("invocation argument %q values = %#v, want %#v", name, argument.Values, wantValues)
	}
	for index, want := range wantValues {
		if argument.Values[index] != want {
			t.Fatalf("invocation argument %q value[%d] = %q, want %q", name, index, argument.Values[index], want)
		}
	}
	if len(argument.Sources) == 0 {
		t.Fatalf("invocation argument %q sources = nil, want %q", name, wantKind)
	}
	for index, source := range argument.Sources {
		if source.Kind != string(wantKind) {
			t.Fatalf(
				"invocation argument %q source[%d] kind = %q, want %q",
				name,
				index,
				source.Kind,
				wantKind,
			)
		}
	}
}

func assertSpineJSONArgument(
	t *testing.T,
	arguments *work.InvocationArguments,
	name string,
	want string,
) {
	t.Helper()
	assertSpineArgument(t, arguments, name, []string{want}, work.ArgumentSourceKindNamed)
	argument := arguments.Arguments[name]
	if !json.Valid([]byte(argument.Values[0])) {
		t.Fatalf("invocation argument %q value is not valid JSON: %q", name, argument.Values[0])
	}
}

func spineInputs(t *testing.T, args []string) *support.CapturedInputs {
	t.Helper()
	return parameterInputs(t, args)
}

func spineEnvironment(environment []string, home string) []string {
	filtered := make([]string, 0, len(environment)+2)
	for _, item := range environment {
		if strings.HasPrefix(item, "HOME=") || strings.HasPrefix(item, "USERPROFILE=") {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, "HOME="+home, "USERPROFILE="+home)
}

func scaffoldCombinedInvocationFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": "combined-parameter-spine",
		"invocationSignature": map[string]any{
			"parameters": []any{
				map[string]any{
					"name":     "input",
					"required": true,
					"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
				},
				map[string]any{
					"name":     "priority",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "callback",
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":      "tag",
					"valueMode": "REPEATED",
					"required":  true,
					"bindings":  []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "metadata",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "nullable",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyString",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyObject",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
				map[string]any{
					"name":     "emptyArray",
					"typeHint": work.InvocationParameterTypeHintJSON,
					"required": true,
					"bindings": []any{map[string]any{"kind": "NAMED"}},
				},
			},
		},
		"workTypes": []any{map[string]any{
			"name":             "task",
			"handlingBehavior": []any{"DEFAULT"},
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "complete", "type": "TERMINAL"},
				map[string]any{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []any{map[string]any{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "processor",
			"inputs":    []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":   []any{map[string]any{"workType": "task", "state": "complete"}},
			"onFailure": []any{map[string]any{"workType": "task", "state": "failed"}},
		}},
	})
}
