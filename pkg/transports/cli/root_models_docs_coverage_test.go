package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	factorycli "github.com/portpowered/infinite-you/pkg/transports/cli/factory"
	initcmd "github.com/portpowered/infinite-you/pkg/transports/cli/init"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	submitcli "github.com/portpowered/infinite-you/pkg/transports/cli/submit"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

func TestNewModelsDocsHandlerRegistryWiresHandwrittenCommands(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	registry, _, err := newModelsDocsHandlerRegistry(globals, diagnostics, &cliOperatorDefaultsOptions{}, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newModelsDocsHandlerRegistry() error = %v", err)
	}
	for _, commandID := range []string{
		"you.docs",
		"you.models.list",
		"you.models.inspect",
		"you.models.invoke",
		"you.models.pull",
	} {
		if _, err := registry.Lookup(commandID); err != nil {
			t.Fatalf("Lookup(%s) error = %v", commandID, err)
		}
	}
}

func TestNewLegacyModelsFamilyCommandBuildsHandwrittenTree(t *testing.T) {
	root := NewLegacyModelsFamilyCommand()
	models, _, err := root.Find([]string{"models"})
	if err != nil {
		t.Fatalf("Find(models) error = %v", err)
	}
	if models.RunE != nil {
		t.Fatal("legacy you models must remain non-runnable")
	}
	list, _, err := root.Find([]string{"models", "list"})
	if err != nil {
		t.Fatalf("Find(models list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("legacy models list must keep handwritten RunE")
	}
}

func TestNewGeneratedModelsFamilyParityCommandBuildsDetachedTree(t *testing.T) {
	registry, invokeFlags, err := newModelsDocsHandlerRegistry(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, &cliOperatorDefaultsOptions{}, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newModelsDocsHandlerRegistry() error = %v", err)
	}
	root, err := NewGeneratedModelsFamilyParityCommand(registry, invokeFlags)
	if err != nil {
		t.Fatalf("NewGeneratedModelsFamilyParityCommand() error = %v", err)
	}
	if _, _, err := root.Find([]string{"models", "invoke"}); err != nil {
		t.Fatalf("Find(models invoke) error = %v", err)
	}
}

func TestNewLegacyDocsFamilyCommandBuildsHandwrittenTree(t *testing.T) {
	root := NewLegacyDocsFamilyCommand()
	docs, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("Find(docs) error = %v", err)
	}
	if docs.RunE == nil {
		t.Fatal("legacy you docs must keep handwritten RunE")
	}
}

func TestNewGeneratedDocsFamilyParityCommandBuildsDetachedTree(t *testing.T) {
	registry, invokeFlags, err := newModelsDocsHandlerRegistry(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, &cliOperatorDefaultsOptions{}, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newModelsDocsHandlerRegistry() error = %v", err)
	}
	root, err := NewGeneratedDocsFamilyParityCommand(registry, invokeFlags)
	if err != nil {
		t.Fatalf("NewGeneratedDocsFamilyParityCommand() error = %v", err)
	}
	docs, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("Find(docs) error = %v", err)
	}
	if docs.RunE == nil {
		t.Fatal("generated docs parity command must attach handwritten RunE")
	}
}

func TestLegacyDocsFamilyExecutesHandwrittenIndexAndTopicPaths(t *testing.T) {
	var stdout, stderr bytes.Buffer
	root := NewLegacyDocsFamilyCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"docs"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute legacy docs index: %v", err)
	}
	if !strings.Contains(stdout.String(), "# Docs") {
		t.Fatalf("stdout = %q, want packaged docs index", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	root.SetArgs([]string{"--verbose", "docs", "models"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute legacy docs topic: %v", err)
	}
	if !strings.Contains(stderr.String(), "docs request topic=models") {
		t.Fatalf("stderr = %q, want verbose docs diagnostics", stderr.String())
	}
}

func TestLegacyModelsFamilyExecutesHandwrittenLeafCommands(t *testing.T) {
	originalList := listModels
	originalInspect := inspectModel
	originalPull := pullModel
	originalInvoke := invokeModel
	defer func() {
		listModels = originalList
		inspectModel = originalInspect
		pullModel = originalPull
		invokeModel = originalInvoke
	}()

	var listed bool
	var inspected, pulled string
	var invocations []modelscli.InvokeConfig
	listModels = func(modelscli.ListConfig) error { listed = true; return nil }
	inspectModel = func(cfg modelscli.InspectConfig) error {
		inspected = cfg.ModelName
		return nil
	}
	pullModel = func(cfg modelscli.PullConfig) error {
		pulled = cfg.ModelName
		return nil
	}
	invokeModel = func(cfg modelscli.InvokeConfig) error {
		invocations = append(invocations, cfg)
		return nil
	}

	root := NewLegacyModelsFamilyCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, args := range [][]string{
		{"models", "list"},
		{"models", "inspect", "OMNIVOICE_Q4_K_M"},
		{"models", "pull", "OMNIVOICE_Q4_K_M"},
		{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "./speech.wav"},
	} {
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute legacy %v: %v", args, err)
		}
	}
	if !listed || inspected != "OMNIVOICE_Q4_K_M" || pulled != "OMNIVOICE_Q4_K_M" || len(invocations) != 1 {
		t.Fatalf("legacy routing = listed %t inspected %q pulled %q invocations %d", listed, inspected, pulled, len(invocations))
	}
	if invocations[0].Operation != "TTS" || invocations[0].Text != "hello" || invocations[0].OutputPath != "./speech.wav" {
		t.Fatalf("legacy invoke config = %#v, want operation/text/output bindings", invocations[0])
	}
}

func TestNewProductionModelsDocsCommandsBuildsGeneratedCommands(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	operatorDefaults := &cliOperatorDefaultsOptions{}
	docs, models, err := newProductionModelsDocsCommands(globals, diagnostics, operatorDefaults, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newProductionModelsDocsCommands() error = %v", err)
	}
	if docs == nil || docs.RunE == nil {
		t.Fatal("generated docs command must attach handwritten RunE")
	}
	if models == nil || models.RunE != nil {
		t.Fatal("generated models parent must remain non-runnable")
	}
	if len(models.Commands()) != 4 {
		t.Fatalf("models child count = %d, want 4 generated leaves", len(models.Commands()))
	}
}

func TestRemainingModelsAccessorGettersReturnDelegates(t *testing.T) {
	if InspectModelAccessor() == nil || PullModelAccessor() == nil || InvokeModelAccessor() == nil {
		t.Fatal("models accessor getters must return non-nil delegates")
	}
}

func TestProductionModelsInspectAndPullHonorJSONFlag(t *testing.T) {
	originalInspect := InspectModelAccessor()
	originalPull := PullModelAccessor()
	defer func() {
		SetInspectModelAccessor(originalInspect)
		SetPullModelAccessor(originalPull)
	}()

	var inspectJSON, pullJSON bool
	SetInspectModelAccessor(func(cfg modelscli.InspectConfig) error {
		inspectJSON = cfg.JSON
		return nil
	})
	SetPullModelAccessor(func(cfg modelscli.PullConfig) error {
		pullJSON = cfg.JSON
		return nil
	})

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute inspect --json: %v", err)
	}
	root.SetArgs([]string{"--json", "models", "pull", "OMNIVOICE_Q4_K_M"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute pull --json: %v", err)
	}
	if !inspectJSON || !pullJSON {
		t.Fatalf("json bindings = inspect %t pull %t, want true", inspectJSON, pullJSON)
	}
}

func TestNewModelsDocsHandlerRegistryDefaultsNilOperatorDefaults(t *testing.T) {
	registry, invokeFlags, err := newModelsDocsHandlerRegistry(&cliGlobalOptions{}, &cliDiagnosticsOptions{}, nil, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newModelsDocsHandlerRegistry(nil operator defaults) error = %v", err)
	}
	if _, err := registry.Lookup("you.models.invoke"); err != nil {
		t.Fatalf("Lookup(you.models.invoke) error = %v", err)
	}
	if invokeFlags.Operation == nil || *invokeFlags.Operation != "TTS" {
		t.Fatalf("invoke operation binding = %#v, want default TTS", invokeFlags.Operation)
	}
}

func TestProductionDocsExecutesTopicWithVerboseDiagnostics(t *testing.T) {
	var stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--verbose", "docs", "models"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute verbose docs models: %v", err)
	}
	if !strings.Contains(stderr.String(), "docs request topic=models") {
		t.Fatalf("stderr = %q, want verbose docs diagnostics", stderr.String())
	}
}

func TestModelsDelegateAccessorsRoundTrip(t *testing.T) {
	originalList := ListModelsAccessor()
	defer SetListModelsAccessor(originalList)

	called := false
	SetListModelsAccessor(func(modelscli.ListConfig) error {
		called = true
		return nil
	})
	if ListModelsAccessor() == nil {
		t.Fatal("ListModelsAccessor() = nil after setter")
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list: %v", err)
	}
	if !called {
		t.Fatal("ListModelsAccessor replacement was not invoked")
	}
}

func TestModelsDelegateAccessorsRouteGeneratedCutoverCommands(t *testing.T) {
	originalList := ListModelsAccessor()
	originalInspect := InspectModelAccessor()
	originalPull := PullModelAccessor()
	originalInvoke := InvokeModelAccessor()
	defer func() {
		SetListModelsAccessor(originalList)
		SetInspectModelAccessor(originalInspect)
		SetPullModelAccessor(originalPull)
		SetInvokeModelAccessor(originalInvoke)
	}()

	var listed bool
	var inspected, pulled string
	var invocations []modelscli.InvokeConfig
	SetListModelsAccessor(func(modelscli.ListConfig) error { listed = true; return nil })
	SetInspectModelAccessor(func(cfg modelscli.InspectConfig) error {
		inspected = cfg.ModelName
		return nil
	})
	SetPullModelAccessor(func(cfg modelscli.PullConfig) error {
		pulled = cfg.ModelName
		return nil
	})
	SetInvokeModelAccessor(func(cfg modelscli.InvokeConfig) error {
		invocations = append(invocations, cfg)
		return nil
	})

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	for _, args := range [][]string{
		{"models", "list"},
		{"models", "inspect", "OMNIVOICE_Q4_K_M"},
		{"models", "pull", "OMNIVOICE_Q4_K_M"},
		{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello"},
	} {
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
	}
	if !listed || inspected != "OMNIVOICE_Q4_K_M" || pulled != "OMNIVOICE_Q4_K_M" || len(invocations) != 1 {
		t.Fatalf("delegate routing = listed %t inspected %q pulled %q invocations %d", listed, inspected, pulled, len(invocations))
	}
	if invocations[0].Operation != "TTS" || invocations[0].Text != "hello" {
		t.Fatalf("invoke config = %#v, want operation/text bindings", invocations[0])
	}
}

func TestProductionRootUsesGeneratedModelsDocsFamilyCutover(t *testing.T) {
	if !useGeneratedModelsDocsFamily {
		t.Fatal("useGeneratedModelsDocsFamily = false, want production cutover enabled")
	}

	root := NewRootCommand()
	docs, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("Find(docs) error = %v", err)
	}
	if docs.RunE == nil {
		t.Fatal("you docs must attach handwritten RunE through generated cutover")
	}

	models, _, err := root.Find([]string{"models"})
	if err != nil {
		t.Fatalf("Find(models) error = %v", err)
	}
	if models.RunE != nil {
		t.Fatal("you models must remain non-runnable")
	}
	list, _, err := root.Find([]string{"models", "list"})
	if err != nil {
		t.Fatalf("Find(models list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("you models list must attach handwritten RunE through generated cutover")
	}
}

func TestRootCommand_HelpDocumentsGlobalJSONFlag(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"--json", "structured JSON on stdout"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommand_HelpDocumentsGlobalServerFlag(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute root --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"--server",
		"factory API base URI",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("root help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("root help must not advertise --port:\n%s", help)
	}
}

func TestFactoryQueryCommand_HelpUsesGlobalFlags(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"factory", "query", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{
		"global --json",
		"global --server",
		"you --server http://localhost:9090 --json factory query",
		"you --json factory query",
	} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("factory query help missing %q:\n%s", want, help)
		}
	}
	if bytes.Contains([]byte(help), []byte("--port")) {
		t.Fatalf("factory query help must not advertise --port:\n%s", help)
	}
}

func TestSupportedCommands_DoNotRegisterLocalJSONFlag(t *testing.T) {
	root := NewRootCommand()
	for _, path := range [][]string{
		{"factory", "query"},
		{"work", "list"},
		{"models", "list"},
		{"models", "inspect"},
		{"models", "invoke"},
		{"models", "pull"},
	} {
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		if cmd.LocalFlags().Lookup("json") != nil {
			t.Fatalf("%s registers a local --json flag; use root persistent --json", cmd.CommandPath())
		}
	}
}

func TestFactoryQueryCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalQueryFactory := queryFactory
	defer func() {
		queryFactory = originalQueryFactory
	}()

	var got factorycli.QueryConfig
	queryFactory = func(cfg factorycli.QueryConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "query"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory query with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to QueryConfig.JSON")
	}
}

func TestFactoryReplaceCurrentCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalReplaceFactoryCurrent := replaceFactoryCurrent
	defer func() {
		replaceFactoryCurrent = originalReplaceFactoryCurrent
	}()

	var got factorycli.ReplaceCurrentConfig
	replaceFactoryCurrent = func(cfg factorycli.ReplaceCurrentConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "replace-current"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory replace-current with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ReplaceCurrentConfig.JSON")
	}
}

func TestModelsInspectCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalInspectModel := inspectModel
	defer func() {
		inspectModel = originalInspectModel
	}()

	var got modelscli.InspectConfig
	inspectModel = func(cfg modelscli.InspectConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "inspect", "OMNIVOICE_Q4_K_M"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to InspectConfig.JSON")
	}
}

func TestWorkListCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalListWork := listWork
	defer func() {
		listWork = originalListWork
	}()

	var got workcli.ListConfig
	listWork = func(cfg workcli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "work", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute work list with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to ListConfig.JSON")
	}
}

func TestSubmitCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalSubmitWork := submitWork
	defer func() {
		submitWork = originalSubmitWork
	}()

	var got submitcli.SubmitConfig
	submitWork = func(cfg submitcli.SubmitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json",
		"submit",
		"--name", "request-name",
		"--work-type-name", "tasks",
		"--payload", "request.md",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute submit with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to SubmitConfig.JSON")
	}
}

func TestFactoryDeleteCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalDeleteFactory := deleteFactory
	defer func() {
		deleteFactory = originalDeleteFactory
	}()

	var got factorycli.DeleteConfig
	deleteFactory = func(cfg factorycli.DeleteConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "factory", "delete", "staging"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute factory delete with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to DeleteConfig.JSON")
	}
}

func TestInitCommand_GlobalJSONMapsToConfig(t *testing.T) {
	originalInitFactory := initFactory
	defer func() {
		initFactory = originalInitFactory
	}()

	var got initcmd.InitConfig
	initFactory = func(cfg initcmd.InitConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init with global --json: %v", err)
	}
	if !got.JSON {
		t.Fatal("expected global --json to map to InitConfig.JSON")
	}
}

func TestModelsListCommand_DefaultServerAndJSONFlagMapToConfig(t *testing.T) {
	originalListModels := listModels
	defer func() {
		listModels = originalListModels
	}()

	var got modelscli.ListConfig
	listModels = func(cfg modelscli.ListConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "models", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list: %v", err)
	}
	if got.Server != "http://localhost:7437" {
		t.Fatalf("server = %q, want http://localhost:7437", got.Server)
	}
	if !got.JSON {
		t.Fatal("expected --json to map to ListConfig.JSON")
	}
}

func TestModelsListCommand_JSONVerboseKeepsStdoutParseableAndDiagnosticsOnStderr(t *testing.T) {
	originalListModels := listModels
	defer func() {
		listModels = originalListModels
	}()

	listModels = func(cfg modelscli.ListConfig) error {
		if !cfg.Verbose {
			t.Fatal("expected verbose config")
		}
		if cfg.Diagnostics == nil {
			t.Fatal("expected diagnostics writer")
		}
		if _, err := fmt.Fprintln(cfg.Diagnostics, "diagnostic: models list"); err != nil {
			return err
		}
		_, err := fmt.Fprintln(cfg.Output, `{"results":[]}`)
		return err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "models", "list", "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models list --json --verbose: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, stdout.String())
	}
	if _, ok := payload["results"]; !ok {
		t.Fatalf("stdout JSON = %#v, want results key", payload)
	}
	if got := stderr.String(); !strings.Contains(got, "diagnostic: models list") {
		t.Fatalf("stderr = %q, want diagnostics", got)
	}
}

func TestModelsInspectCommand_MapsModelArgumentAndServer(t *testing.T) {
	originalInspectModel := inspectModel
	defer func() {
		inspectModel = originalInspectModel
	}()

	var got modelscli.InspectConfig
	inspectModel = func(cfg modelscli.InspectConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "inspect", "OMNIVOICE_Q4_K_M", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models inspect: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model name = %q, want OMNIVOICE_Q4_K_M", got.ModelName)
	}
	if got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("server = %q, want http://127.0.0.1:9090", got.Server)
	}
}

func TestModelsInvokeCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalInvokeModel := invokeModel
	defer func() {
		invokeModel = originalInvokeModel
	}()

	var got modelscli.InvokeConfig
	invokeModel = func(cfg modelscli.InvokeConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS", "--text", "hello", "--output", "speech.wav", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models invoke: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Operation != "TTS" || got.Text != "hello" || got.OutputPath != "speech.wav" || got.Server != "http://127.0.0.1:9090" {
		t.Fatalf("invoke config = %#v, want mapped invoke args and flags", got)
	}
}

func TestModelsPullCommand_MapsArgumentsAndFlags(t *testing.T) {
	originalPullModel := pullModel
	defer func() {
		pullModel = originalPullModel
	}()

	var got modelscli.PullConfig
	pullModel = func(cfg modelscli.PullConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--json", "--server", "http://127.0.0.1:9090", "models", "pull", "OMNIVOICE_Q4_K_M"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models pull: %v", err)
	}
	if got.ModelName != "OMNIVOICE_Q4_K_M" || got.Server != "http://127.0.0.1:9090" || !got.JSON {
		t.Fatalf("pull config = %#v, want mapped pull args and flags", got)
	}
}

func TestModelsCommand_HelpMentionsDiscoverySurface(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand()
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute models --help: %v", err)
	}
	help := out.String()
	for _, want := range []string{"Inspect discovered models", "list", "inspect", "invoke", "pull", "bootstrap"} {
		if !bytes.Contains([]byte(help), []byte(want)) {
			t.Fatalf("models help missing %q:\n%s", want, help)
		}
	}
}
