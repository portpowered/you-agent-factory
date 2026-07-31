package validate_persist

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	durableBaselineFactoryName  = "validate-persist-baseline"
	persistThenRunFactoryName   = "validate-persist-run"
	persistThenRunPrimaryResult = "persist-from-file run COMPLETE"
)

const invalidFactoryWithDanglingWorker = `{
	"name":"invalid-validate-persist",
	"workTypes":[{
		"name":"task",
		"states":[
			{"name":"init","type":"INITIAL"},
			{"name":"complete","type":"TERMINAL"},
			{"name":"failed","type":"FAILED"}
		]
	}],
	"workstations":[{
		"name":"execute",
		"worker":"missing-worker",
		"inputs":[{"workType":"task","state":"init"}],
		"outputs":[{"workType":"task","state":"complete"}],
		"onFailure":[{"workType":"task","state":"failed"}],
		"type":"MODEL_WORKSTATION"
	}]
}`

// TestCLIFactoryValidateRejectsInvalidDefinitionActionably proves the public
// Factory CLI validate command rejects an invalid authored definition with
// actionable diagnostics before runtime execution or persistence side effects.
func TestCLIFactoryValidateRejectsInvalidDefinitionActionably(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		wants []string
	}{
		{
			name: "semantic validation failure",
			body: invalidFactoryWithDanglingWorker,
			wants: []string{
				"factory validation found blocking issues",
				"factory.worker.danglingReference",
				"missing-worker",
			},
		},
		{
			name: "parse failure",
			body: `{"name":["invalid"]}`,
			wants: []string{
				"parse factory config",
				"name",
				"cannot unmarshal",
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			sourcePath := writeFactoryFile(t, test.body)
			assertValidateRejectedActionably(t, sourcePath, test.wants, validateOptions{})
		})
	}
}

// TestCLIFactoryValidateDoesNotMutateOnFailure proves a failed public Factory
// CLI validate of an invalid candidate leaves an existing durable Factory
// definition unchanged on disk and in the public factory list.
func TestCLIFactoryValidateDoesNotMutateOnFailure(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	sourceDir := support.ScaffoldSingleStepFactory(t, durableBaselineFactoryName)
	factoryDir := support.CreateNamedFactory(
		t,
		home,
		workingDirectory,
		durableBaselineFactoryName,
		filepath.Join(sourceDir, "factory.json"),
	)
	env := customerHomeEnvironment(home)
	before := captureDurableBaseline(t, home, workingDirectory, factoryDir)

	invalidPath := writeFactoryFile(t, invalidFactoryWithDanglingWorker)
	assertValidateRejectedActionably(
		t,
		invalidPath,
		[]string{
			"factory validation found blocking issues",
			"factory.worker.danglingReference",
		},
		validateOptions{
			env:              env,
			workingDirectory: workingDirectory,
		},
	)

	after := captureDurableBaseline(t, home, workingDirectory, factoryDir)
	assertDurableBaselineUnchanged(t, before, after)
}

// TestCLIFactoryPersistFromFileThenRunSucceeds proves persist-from-file through
// the public Factory CLI create path produces a durable Factory that succeeds on
// a subsequent public run invocation with a customer-visible primary result.
func TestCLIFactoryPersistFromFileThenRunSucceeds(t *testing.T) {
	home := t.TempDir()
	workingDirectory := t.TempDir()
	env := customerHomeEnvironment(home)

	sourceDir := support.ScaffoldSingleStepFactory(t, persistThenRunFactoryName)
	support.UpdateFactoryConfig(t, sourceDir, func(cfg map[string]any) {
		workTypes, ok := cfg["workTypes"].([]any)
		if !ok || len(workTypes) == 0 {
			t.Fatal("persist-then-run factory missing workTypes")
		}
		workType, ok := workTypes[0].(map[string]any)
		if !ok {
			t.Fatal("persist-then-run factory workType has unexpected shape")
		}
		workType["handlingBehavior"] = []any{"DEFAULT"}
	})
	support.WriteAgentConfig(
		t,
		sourceDir,
		"processor",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	sourcePath := filepath.Join(sourceDir, interfaces.FactoryConfigFile)

	factoryDir := support.CreateNamedFactory(
		t,
		home,
		workingDirectory,
		persistThenRunFactoryName,
		sourcePath,
	)
	if _, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile)); err != nil {
		t.Fatalf("persisted %s missing after factory create --from: %v", interfaces.FactoryConfigFile, err)
	}

	providerRunner := support.NewStaticSuccessCommandRunner(persistThenRunPrimaryResult)
	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{APIServerStarter: api.Start}
	support.ConfigureWorkerCommands(t, &edges, providerRunner, nil)
	process := support.BuildProcess(t, edges)

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", persistThenRunFactoryName,
		"--no-record",
		"--with-server",
		"prove persist-from-file then run",
	})
	inputs.Input.Env = env
	inputs.Input.WorkingDirectory = workingDirectory

	command := support.StartProcessCommand(t, process, inputs.Input)
	_ = api.WaitForURL(t)
	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(factory run) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want %q", response.Status, factoryapi.InvocationTerminalStatusCompleted)
	}
	if got := invocationPrimaryResultText(t, response); got != persistThenRunPrimaryResult {
		t.Fatalf("primaryResult = %q, want %q", got, persistThenRunPrimaryResult)
	}
}

type validateOptions struct {
	env              []string
	workingDirectory string
}

func assertValidateRejectedActionably(
	t *testing.T,
	factorySource string,
	wants []string,
	options validateOptions,
) {
	t.Helper()

	runner := support.NewRecordingCommandRunner("runtime must not execute")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "factory", "config", "validate", factorySource,
	})
	if options.env != nil {
		inputs.Input.Env = options.env
	}
	if options.workingDirectory != "" {
		inputs.Input.WorkingDirectory = options.workingDirectory
	} else {
		inputs.Input.WorkingDirectory = filepath.Dir(factorySource)
	}
	err := support.BuildProcess(
		t,
		serviceedges.Edges{ProviderCommandRunner: runner},
	).Execute(inputs.Input)
	if err == nil {
		t.Fatal("Process.Execute(factory config validate) error = nil, want rejection")
	}

	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("validate diagnostic missing %q:\n%s", want, diagnostic)
		}
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 before validate completes", runner.CallCount())
	}
}

type durableBaseline struct {
	factoryJSON []byte
	factory     factoryapi.Factory
	listEntries []factoryListEntry
	listEntry   factoryListEntry
}

type factoryListEntry struct {
	Name             string `json:"name"`
	FactoryDirectory string `json:"factoryDirectory"`
	Description      string `json:"description"`
}

func captureDurableBaseline(
	t *testing.T,
	home string,
	workingDirectory string,
	factoryDir string,
) durableBaseline {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(factoryDir, "factory.json"))
	if err != nil {
		t.Fatalf("read durable factory.json: %v", err)
	}
	factory, err := support.LoadedFactory(t, factoryDir)
	if err != nil {
		t.Fatalf("LoadedFactory(%s): %v", factoryDir, err)
	}
	listEntries := executeFactoryList(t, home, workingDirectory)
	entry, ok := findFactoryListEntry(t, listEntries, durableBaselineFactoryName)
	if !ok {
		t.Fatalf("factory list missing durable Factory %q", durableBaselineFactoryName)
	}
	if entry.FactoryDirectory != factoryDir {
		t.Fatalf(
			"factory list directory = %q, want durable directory %q",
			entry.FactoryDirectory,
			factoryDir,
		)
	}
	return durableBaseline{
		factoryJSON: append([]byte(nil), factoryJSON...),
		factory:     factory,
		listEntries: append([]factoryListEntry(nil), listEntries...),
		listEntry:   entry,
	}
}

func assertDurableBaselineUnchanged(t *testing.T, before, after durableBaseline) {
	t.Helper()

	if !bytes.Equal(before.factoryJSON, after.factoryJSON) {
		t.Fatalf(
			"durable factory.json changed after failed validate\nbefore:\n%s\nafter:\n%s",
			before.factoryJSON,
			after.factoryJSON,
		)
	}
	if !reflect.DeepEqual(before.factory, after.factory) {
		t.Fatalf(
			"loaded durable Factory changed after failed validate\nbefore: %#v\nafter: %#v",
			before.factory,
			after.factory,
		)
	}
	if !reflect.DeepEqual(before.listEntry, after.listEntry) {
		t.Fatalf(
			"factory list entry changed after failed validate\nbefore: %#v\nafter: %#v",
			before.listEntry,
			after.listEntry,
		)
	}
	if !reflect.DeepEqual(before.listEntries, after.listEntries) {
		t.Fatalf(
			"factory list inventory changed after failed validate\nbefore: %#v\nafter: %#v",
			before.listEntries,
			after.listEntries,
		)
	}
}

func executeFactoryList(t *testing.T, home, workingDirectory string) []factoryListEntry {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{"you", "--json", "factory", "list"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = workingDirectory
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(factory list) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	var entries []factoryListEntry
	if err := json.Unmarshal([]byte(inputs.Stdout()), &entries); err != nil {
		t.Fatalf("decode factory list: %v\n%s", err, inputs.Stdout())
	}
	return entries
}

func findFactoryListEntry(
	t *testing.T,
	entries []factoryListEntry,
	name string,
) (factoryListEntry, bool) {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return factoryListEntry{}, false
}

func writeFactoryFile(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	return path
}

func customerHomeEnvironment(home string) []string {
	return append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text content part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult text part: %v", err)
	}
	return part.Text
}
