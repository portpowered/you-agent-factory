package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolveFactoryInvocationInputSchemaNamedAndFileSelectionsAreEquivalent(t *testing.T) {
	t.Parallel()

	fixtureSignature := interfaces.InvocationSignatureConfig{
		Parameters: []interfaces.InvocationParameterConfig{{
			Name:         "query",
			ExternalName: "search",
			Aliases:      []string{"q"},
			DefaultValue: "all",
			Bindings: []interfaces.InvocationParameterBindingConfig{{
				Kind: work.InvocationParameterBindingKindNamed,
			}},
		}},
	}
	namedPath := filepath.Join("project", "factory", "named-fixture", interfaces.FactoryConfigFile)
	filePath := filepath.Join("fixtures", "portable-factory.yaml")
	selectedConfigs := map[string]*interfaces.FactoryConfig{
		namedPath: {
			Name:                "named-catalog-identity",
			InvocationSignature: cloneInvocationSignatureFixture(fixtureSignature),
		},
		filePath: {
			Name:                "portable-file-identity",
			InvocationSignature: cloneInvocationSignatureFixture(fixtureSignature),
		},
	}
	expectedNamed := interfaces.FactoryConfig{
		Name:                selectedConfigs[namedPath].Name,
		InvocationSignature: cloneInvocationSignatureFixture(*selectedConfigs[namedPath].InvocationSignature),
	}
	expectedFile := interfaces.FactoryConfig{
		Name:                selectedConfigs[filePath].Name,
		InvocationSignature: cloneInvocationSignatureFixture(*selectedConfigs[filePath].InvocationSignature),
	}
	loadedPaths := make([]string, 0, 2)
	load := interfaces.FactoryConfigFileLoader(func(path string) (*interfaces.FactoryConfig, error) {
		loadedPaths = append(loadedPaths, path)
		return selectedConfigs[path], nil
	})

	manifest := runSchemaFixtureManifest()
	named, namedDiagnostics, err := ResolveFactoryInvocationInputSchema(
		context.Background(), manifest, "you.run", load, namedPath,
	)
	if err != nil {
		t.Fatalf("named selection: %v", err)
	}
	file, fileDiagnostics, err := ResolveFactoryInvocationInputSchema(
		context.Background(), manifest, "you.run", load, filePath,
	)
	if err != nil {
		t.Fatalf("file selection: %v", err)
	}
	if !reflect.DeepEqual(named, file) || !reflect.DeepEqual(namedDiagnostics, fileDiagnostics) {
		t.Fatalf("equivalent selections differ: named=%#v/%#v file=%#v/%#v", named, namedDiagnostics, file, fileDiagnostics)
	}
	if !reflect.DeepEqual(loadedPaths, []string{namedPath, filePath}) {
		t.Fatalf("read-only loader paths = %#v", loadedPaths)
	}
	if !reflect.DeepEqual(*selectedConfigs[namedPath], expectedNamed) ||
		!reflect.DeepEqual(*selectedConfigs[filePath], expectedFile) {
		t.Fatalf("schema resolution mutated a selected Factory config")
	}
}

func TestResolveFactoryInvocationInputSchemaHonorsCancellationWithoutPartialResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cancelLoad bool
	}{
		{name: "before lookup"},
		{name: "during lookup", cancelLoad: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			loadCalls := 0
			load := interfaces.FactoryConfigFileLoader(func(string) (*interfaces.FactoryConfig, error) {
				loadCalls++
				if test.cancelLoad {
					cancel()
				}
				return &interfaces.FactoryConfig{InvocationSignature: &interfaces.InvocationSignatureConfig{}}, nil
			})
			if !test.cancelLoad {
				cancel()
			}

			schema, diagnostics, err := ResolveFactoryInvocationInputSchema(
				ctx, runSchemaFixtureManifest(), "you.run", load, "fixture/factory.json",
			)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v, want context cancellation", err)
			}
			if !reflect.DeepEqual(schema, climanifest.EffectiveInputSchema{}) || diagnostics != nil {
				t.Fatalf("canceled lookup returned partial result: schema=%#v diagnostics=%#v", schema, diagnostics)
			}
			wantCalls := 0
			if test.cancelLoad {
				wantCalls = 1
			}
			if loadCalls != wantCalls {
				t.Fatalf("loader calls = %d, want %d", loadCalls, wantCalls)
			}
		})
	}
}

func TestMapInvocationInputErrorPreservesWrappedStableSensitiveFailure(t *testing.T) {
	t.Parallel()

	const sensitiveValue = "credential-that-must-not-leak"
	wrapped := fmt.Errorf("%w: %w", work.ErrInvalidInvocationInput, &work.ArgumentError{
		Code:    work.ArgumentErrorCodeStringValidationMismatch,
		Message: `parameter "token" value <redacted> is not one of the declared choices`,
	})

	err := MapInvocationInputError(wrapped)
	if err == nil || !strings.Contains(err.Error(), string(work.ArgumentErrorCodeStringValidationMismatch)) {
		t.Fatalf("error = %v, want stable validation code", err)
	}
	if strings.Contains(err.Error(), sensitiveValue) {
		t.Fatalf("mapped error leaked sensitive value: %v", err)
	}
}

func TestResolveFactoryInvocationInputSchemaNoSignatureNamedAndFileSelectionsAreEquivalent(t *testing.T) {
	t.Parallel()

	namedPath := filepath.Join("project", "factory", "legacy", interfaces.FactoryConfigFile)
	filePath := filepath.Join("fixtures", "legacy-factory.yaml")
	load := interfaces.FactoryConfigFileLoader(func(path string) (*interfaces.FactoryConfig, error) {
		switch path {
		case namedPath, filePath:
			return &interfaces.FactoryConfig{Name: filepath.Base(filepath.Dir(path))}, nil
		default:
			return nil, errors.New("unexpected Factory path")
		}
	})

	manifest := runSchemaFixtureManifest()
	named, namedDiagnostics, err := ResolveFactoryInvocationInputSchema(
		context.Background(), manifest, "you.run", load, namedPath,
	)
	if err != nil {
		t.Fatalf("named selection: %v", err)
	}
	file, fileDiagnostics, err := ResolveFactoryInvocationInputSchema(
		context.Background(), manifest, "you.run", load, filePath,
	)
	if err != nil {
		t.Fatalf("file selection: %v", err)
	}
	if !reflect.DeepEqual(named, file) || !reflect.DeepEqual(namedDiagnostics, fileDiagnostics) {
		t.Fatalf("no-signature selections differ: named=%#v/%#v file=%#v/%#v", named, namedDiagnostics, file, fileDiagnostics)
	}
	if named.FactoryInputMode != climanifest.EffectiveFactoryInputModeCompatibility {
		t.Fatalf("FactoryInputMode = %q, want compatibility", named.FactoryInputMode)
	}
	if named.UnknownNamedArgumentPolicy != "" || len(named.FactoryParameters) != 0 {
		t.Fatalf("no-signature selection synthesized signature facts: %#v", named)
	}
}

func cloneInvocationSignatureFixture(signature interfaces.InvocationSignatureConfig) *interfaces.InvocationSignatureConfig {
	cloned := signature
	cloned.Parameters = append([]interfaces.InvocationParameterConfig(nil), signature.Parameters...)
	for index := range cloned.Parameters {
		cloned.Parameters[index].Aliases = append([]string(nil), signature.Parameters[index].Aliases...)
		cloned.Parameters[index].Bindings = append(
			[]interfaces.InvocationParameterBindingConfig(nil),
			signature.Parameters[index].Bindings...,
		)
	}
	return &cloned
}

func runSchemaFixtureManifest() climanifest.Manifest {
	return climanifest.Manifest{Commands: map[string]climanifest.Command{
		"you": {ID: "you", Name: "you"},
		"you.run": {
			ID:   "you.run",
			Name: "run",
			Flags: map[string]climanifest.Flag{
				"factory": {ID: "you.run.flag.factory", Long: "factory"},
				"named":   {ID: "you.run.flag.named", Long: "named"},
			},
		},
	}}
}

func TestJavaScriptWorkflowPathRecognizesSupportedExtensions(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"workflow.js", "WORKFLOW.MJS", " workflow.cjs "} {
		if !javascriptWorkflowPath(path) {
			t.Fatalf("javascriptWorkflowPath(%q) = false", path)
		}
	}
	if javascriptWorkflowPath("factory.json") {
		t.Fatal("javascriptWorkflowPath accepted a factory config")
	}
	data, err := loadFactoryInvocationHelpData("you", RunConfig{FactoryConfigPath: "workflow.mjs"})
	if err != nil || data != nil {
		t.Fatalf("loadFactoryInvocationHelpData(JavaScript) = (%#v, %v)", data, err)
	}
}

func TestFormatFactoryInvocationHelp_RendersTopLevelStructuredExamples(t *testing.T) {
	data := factoryInvocationHelpData{
		factoryName:   "example-factory",
		selectionText: "named factory example-factory",
		commandPrefix: "you run --named example-factory",
		signature: &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
			{Name: "input", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}}},
			{Name: "tag", ExternalName: "tag", ValueMode: "REPEATED", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
		}},
		examples: []interfaces.InvocationExampleConfig{{
			Name: "tagged",
			Description: interfaces.NameValueConfig{
				Type: interfaces.NameValueTypeLocalizableAsset, Value: "Run with two tags.",
			},
			Args: interfaces.InvocationExampleArguments{"input": "hello world", "tag": []string{"alpha", "beta"}},
		}},
	}

	output := formatFactoryInvocationHelp(data)
	for _, want := range []string{
		"# Run with two tags.",
		"you run --named example-factory 'hello world' --tag alpha --tag beta",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatInvocationExampleRendersStructuredStdin(t *testing.T) {
	t.Parallel()

	signature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
		{Name: "body", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "STDIN"}}},
	}}
	example := interfaces.InvocationExampleConfig{
		Name: "stdin",
		Args: interfaces.InvocationExampleArguments{"body": "first line\nsecond line"},
	}
	output := formatInvocationExample("you run --factory factory.json", signature, example)
	if !strings.Contains(output, "printf '%s\\n' 'first line\nsecond line' | you run --factory factory.json") {
		t.Fatalf("stdin example output = %q", output)
	}
}

func TestFormatInvocationExampleResolvesAliasAndExternalNameBindings(t *testing.T) {
	t.Parallel()

	signature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
		{Name: "input", ExternalName: "prompt", Aliases: []string{"p"}, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}}},
		{Name: "body", ExternalName: "content", Aliases: []string{"c"}, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "STDIN"}}},
	}}
	tests := []struct {
		name string
		args interfaces.InvocationExampleArguments
		want string
	}{
		{name: "alias follows positional binding", args: interfaces.InvocationExampleArguments{"p": "hello"}, want: "you run --factory factory.json hello\n"},
		{name: "external name follows stdin binding", args: interfaces.InvocationExampleArguments{"content": "from stdin"}, want: "printf '%s\\n' 'from stdin' | you run --factory factory.json\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			example := interfaces.InvocationExampleConfig{Name: test.name, Args: test.args}
			if got := formatInvocationExample("you run --factory factory.json", signature, example); got != "  "+test.want {
				t.Fatalf("formatInvocationExample() = %q, want %q", got, "  "+test.want)
			}
		})
	}
}

// Work owns ambiguity detection. This transport test verifies only the stable
// CLI representation and observability of the injected role's typed failure.
func TestObserveInvocationRejection_AmbiguousInputRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()
	err := MapInvocationInputError(&work.InputError{
		Code:    work.InputErrorCodeSourceConflict,
		Message: "invocation input sources conflict: positional_text, stdin_text",
		ConflictingSources: []work.InputSourceLabel{
			work.InputSourcePositionalText,
			work.InputSourceStdinText,
		},
	})

	core, observed := observer.New(zap.InfoLevel)
	ObserveInvocationRejection(zap.New(core), err)

	entry := observed.FilterMessage(cleanInvocationLogMessageRejected).AllUntimed()
	if len(entry) != 1 {
		t.Fatalf("rejected logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["mode"] != cleanInvocationModeLabel || fields["reason"] != cleanInvocationRejectReason {
		t.Fatalf("fields = %#v", fields)
	}
	conflictingAny, ok := fields["conflictingSources"].([]interface{})
	if !ok || len(conflictingAny) != 2 || conflictingAny[0] != "positional_prompt" || conflictingAny[1] != "stdin" {
		t.Fatalf("conflictingSources = %#v", fields["conflictingSources"])
	}
	if got := snapshotCleanInvocationMetrics(); got != (CleanInvocationMetricsSnapshot{Attempts: 1, AmbiguityRejected: 1}) {
		t.Fatalf("metrics = %#v", got)
	}
}
