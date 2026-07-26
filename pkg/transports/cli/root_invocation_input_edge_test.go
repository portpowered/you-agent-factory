package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

func TestResolveFactoryInvocationInput_RequiresProcessTerminalMetadata(t *testing.T) {
	_, err := collectRunInvocationStdin(nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "process terminal metadata is required") {
		t.Fatalf("error = %v, want missing terminal metadata", err)
	}
}

func TestPrepareRunInvocationInputRequiresInjectedWorkRole(t *testing.T) {
	_, err := prepareRunInvocationInput(&cobra.Command{}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "Work invocation-input preparation is required") {
		t.Fatalf("error = %v, want required Work role", err)
	}
}

func TestResolveFactoryInvocationInput_RequiresProcessStdinForExplicitDash(t *testing.T) {
	_, err := collectRunInvocationStdin([]string{"-"}, nil, func() bool { return true })
	if err == nil || !strings.Contains(err.Error(), "process stdin is required") {
		t.Fatalf("error = %v, want missing stdin", err)
	}
}

func TestCollectRunInvocationStdinPreservesReaderFailure(t *testing.T) {
	want := errors.New("reader failed")
	_, err := collectRunInvocationStdin([]string{"-"}, failingInvocationReader{err: want}, func() bool { return true })
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want wrapped reader failure", err)
	}
}

type failingInvocationReader struct{ err error }

func (reader failingInvocationReader) Read([]byte) (int, error) { return 0, reader.err }

func TestResolveRunFactoryPromptNoSignatureSelectionsPreserveCompatibilityInput(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, factoryDir)
	stdinText := "from stdin\n"
	tests := []struct {
		name       string
		arguments  []string
		stdin      *string
		wantSource work.InputSourceLabel
		wantText   string
	}{
		{
			name: "positional text", arguments: []string{"--mode", "fast"},
			wantSource: work.InputSourcePositionalText, wantText: "--mode fast",
		},
		{
			name: "stdin text", arguments: []string{"-"}, stdin: &stdinText,
			wantSource: work.InputSourceStdinText, wantText: "from stdin\n",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			named := resolveNoSignaturePromptForTest(
				t, factoryDir, factoryPath, "named", "legacy",
				test.arguments, test.stdin, test.wantSource, test.wantText,
			)
			file := resolveNoSignaturePromptForTest(
				t, factoryDir, factoryPath, "factory", factoryPath,
				test.arguments, test.stdin, test.wantSource, test.wantText,
			)
			if named.request.Signature != nil || file.request.Signature != nil {
				t.Fatalf("no-signature selections activated signature policy: named=%#v file=%#v", named.request.Signature, file.request.Signature)
			}
			if !reflect.DeepEqual(named.request, file.request) {
				t.Fatalf("compatibility preparation requests differ: named=%#v file=%#v", named.request, file.request)
			}
			for selection, cfg := range map[string]runcli.RunConfig{"named": named.config, "file": file.config} {
				if cfg.PreparedInvocationInput == nil ||
					cfg.PreparedInvocationInput.ResolvedInput == nil ||
					cfg.PreparedInvocationInput.ResolvedInput.Text != test.wantText {
					t.Fatalf("%s prepared input = %#v, want compatibility text %q", selection, cfg.PreparedInvocationInput, test.wantText)
				}
				if cfg.InvocationNormalizedArguments != nil {
					t.Fatalf("%s normalized arguments = %#v, want nil", selection, cfg.InvocationNormalizedArguments)
				}
			}
		})
	}
}

type noSignaturePromptObservation struct {
	request work.InvocationInputPreparationRequest
	config  runcli.RunConfig
}

func resolveNoSignaturePromptForTest(
	t *testing.T,
	factoryDir string,
	factoryPath string,
	flag string,
	value string,
	arguments []string,
	stdin *string,
	wantSource work.InputSourceLabel,
	wantText string,
) noSignaturePromptObservation {
	t.Helper()
	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("named", "", "")
	cmd.Flags().String("factory", "", "")
	cmd.Flags().String("work", "", "")
	if err := cmd.Flags().Set(flag, value); err != nil {
		t.Fatalf("set %s: %v", flag, err)
	}
	if stdin != nil {
		cmd.SetIn(strings.NewReader(*stdin))
	} else {
		cmd.SetIn(strings.NewReader(""))
	}

	calls := 0
	var request work.InvocationInputPreparationRequest
	preparation := rootInvocationInputScript{prepare: func(
		_ context.Context,
		got work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		calls++
		request = got
		resolved := work.ResolvedInput{
			Source: wantSource,
			Text:   wantText,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: wantText,
			}},
		}
		return work.PreparedInvocationInput{Source: wantSource, ResolvedInput: &resolved}, nil
	}}
	cfg := runcli.RunConfig{Dir: factoryDir, LoadFactoryConfigFile: loadTestFactoryConfigFile}
	if flag == "factory" {
		cfg.FactoryConfigPath = factoryPath
	}
	if err := resolveRunFactoryPrompt(cmd, &cfg, arguments, preparation); err != nil {
		t.Fatalf("resolve %s Factory prompt: %v", flag, err)
	}
	if calls != 1 {
		t.Fatalf("%s preparation calls = %d, want exactly 1", flag, calls)
	}
	return noSignaturePromptObservation{request: request, config: cfg}
}

func TestResolveRunFactoryPromptPreservesCompleteEffectiveSignatureBehavior(t *testing.T) {
	t.Parallel()

	factoryDir, factoryPath := writeEffectiveSignatureFactory(t, completeInvocationSignaturePayload())
	cmd := newEffectiveSignatureCommand(t, factoryPath, "stdin body")
	cfg := effectiveSignatureRunConfig(factoryDir, factoryPath)
	var request work.InvocationInputPreparationRequest
	calls := 0
	preparation := rootInvocationInputScript{prepare: func(
		_ context.Context,
		got work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		calls++
		request = got
		return completePreparedInvocation(), nil
	}}

	err := resolveRunFactoryPrompt(cmd, &cfg, completeInvocationArguments(), preparation)
	if err != nil {
		t.Fatalf("resolve complete signature input: %v", err)
	}
	if calls != 1 {
		t.Fatalf("preparation calls = %d, want exactly one", calls)
	}
	assertCompletePreparedInvocation(t, cfg.PreparedInvocationInput)
	assertCompleteEffectiveValueModes(t, request.Signature)
	if !reflect.DeepEqual(request.Arguments, completeInvocationArguments()) ||
		request.StdinText == nil || *request.StdinText != "stdin body" {
		t.Fatalf("raw Work preparation request = %#v", request)
	}
}

func TestResolveRunFactoryPromptEmptySignatureInputStillUsesEffectiveSchema(t *testing.T) {
	t.Parallel()

	factoryDir, factoryPath := writeEffectiveSignatureFactory(t, defaultOnlyInvocationSignaturePayload())
	cmd := newEffectiveSignatureCommand(t, factoryPath, "")
	cfg := effectiveSignatureRunConfig(factoryDir, factoryPath)
	calls := 0
	preparation := rootInvocationInputScript{prepare: func(
		_ context.Context,
		request work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		calls++
		if request.Signature == nil || len(request.Signature.Parameters) != 1 ||
			request.Signature.Parameters[0].DefaultValue != "safe" {
			t.Fatalf("empty-input preparation signature = %#v, want selected Factory default", request.Signature)
		}
		return work.PreparedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"mode": {
					Values:  []string{"safe"},
					Sources: []work.ArgumentSource{{Kind: work.ArgumentSourceKindDefault, Name: "default"}},
				},
			},
		}}, nil
	}}

	if err := resolveRunFactoryPrompt(cmd, &cfg, nil, preparation); err != nil {
		t.Fatalf("resolve empty signature input: %v", err)
	}
	if calls != 1 {
		t.Fatalf("preparation calls = %d, want exactly one", calls)
	}
	if cfg.PreparedInvocationInput == nil ||
		cfg.PreparedInvocationInput.NormalizedArguments == nil ||
		cfg.PreparedInvocationInput.NormalizedArguments.Arguments["mode"].Values[0] != "safe" {
		t.Fatalf("prepared default-only input = %#v", cfg.PreparedInvocationInput)
	}
}

func TestResolveRunFactoryPromptEmptyInputStillRejectsSchemaCollision(t *testing.T) {
	t.Parallel()

	factoryDir, factoryPath := writeEffectiveSignatureFactory(t, collisionInvocationSignaturePayload())
	cmd := newEffectiveSignatureCommand(t, factoryPath, "")
	cfg := effectiveSignatureRunConfig(factoryDir, factoryPath)
	calls := 0
	preparation := rootInvocationInputScript{prepare: func(
		context.Context,
		work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		calls++
		return work.PreparedInvocationInput{}, nil
	}}

	err := resolveRunFactoryPrompt(cmd, &cfg, nil, preparation)
	if err == nil || !strings.Contains(err.Error(), climanifest.CompositionCollisionLongName) {
		t.Fatalf("error = %v, want empty-input composition collision", err)
	}
	if calls != 0 {
		t.Fatalf("preparation calls = %d, want collision before normalization", calls)
	}
	if cfg.PreparedInvocationInput != nil || cfg.InvocationNormalizedArguments != nil {
		t.Fatalf("collision left partial input: %#v", cfg)
	}
}

func TestResolveRunFactoryPromptSensitiveConstraintFailureIsStableAndDetached(t *testing.T) {
	t.Parallel()

	const sensitiveValue = "credential-that-must-not-leak"
	factoryDir, factoryPath := writeEffectiveSignatureFactory(t, sensitiveInvocationSignaturePayload())
	cmd := newEffectiveSignatureCommand(t, factoryPath, "")
	cfg := effectiveSignatureRunConfig(factoryDir, factoryPath)
	preparation := rootInvocationInputScript{prepare: func(
		_ context.Context,
		_ work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		return work.PreparedInvocationInput{}, &work.ArgumentError{
			Code:    work.ArgumentErrorCodeStringValidationMismatch,
			Message: `parameter "token" value <redacted> is not one of the declared choices`,
		}
	}}

	err := resolveRunFactoryPrompt(
		cmd,
		&cfg,
		[]string{"--token", sensitiveValue},
		preparation,
	)
	if err == nil || !strings.Contains(err.Error(), string(work.ArgumentErrorCodeStringValidationMismatch)) {
		t.Fatalf("error = %v, want stable validation mismatch", err)
	}
	if strings.Contains(err.Error(), sensitiveValue) {
		t.Fatalf("sensitive value leaked in error: %v", err)
	}
	if cfg.PreparedInvocationInput != nil || cfg.InvocationNormalizedArguments != nil {
		t.Fatalf("failed normalization left partial input: %#v", cfg)
	}
}

func TestResolveRunFactoryPromptCancellationReturnsNoPartialInput(t *testing.T) {
	t.Parallel()

	factoryDir, factoryPath := writeEffectiveSignatureFactory(t, completeInvocationSignaturePayload())
	cmd := newEffectiveSignatureCommand(t, factoryPath, "")
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	cfg := effectiveSignatureRunConfig(factoryDir, factoryPath)
	preparation := rootInvocationInputScript{prepare: func(
		_ context.Context,
		_ work.InvocationInputPreparationRequest,
	) (work.PreparedInvocationInput, error) {
		cancel()
		return work.PreparedInvocationInput{}, ctx.Err()
	}}

	err := resolveRunFactoryPrompt(cmd, &cfg, []string{"draft"}, preparation)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if cfg.PreparedInvocationInput != nil || cfg.InvocationNormalizedArguments != nil {
		t.Fatalf("canceled normalization left partial input: %#v", cfg)
	}
}

func newEffectiveSignatureCommand(t *testing.T, factoryPath, stdin string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{Use: "run"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("named", "", "")
	cmd.Flags().String("factory", "", "")
	cmd.Flags().String("work", "", "")
	if err := cmd.Flags().Set("factory", factoryPath); err != nil {
		t.Fatalf("set factory: %v", err)
	}
	cmd.SetIn(strings.NewReader(stdin))
	return cmd
}

func effectiveSignatureRunConfig(factoryDir, factoryPath string) runcli.RunConfig {
	return runcli.RunConfig{
		Dir:                   factoryDir,
		FactoryConfigPath:     factoryPath,
		LoadFactoryConfigFile: loadTestFactoryConfigFile,
	}
}

func writeEffectiveSignatureFactory(t *testing.T, payload []byte) (string, string) {
	t.Helper()

	factoryDir := t.TempDir()
	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, payload, 0o644); err != nil {
		t.Fatalf("write Factory fixture: %v", err)
	}
	return factoryDir, factoryPath
}

func completeInvocationArguments() []string {
	return []string{
		"schema", "one.md", "two.md",
		"--t", "alpha", "--tag", "beta",
		"--count", "2",
		"--file", "story.md",
		"-",
	}
}

func completePreparedInvocation() work.PreparedInvocationInput {
	return work.PreparedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"topic": {Values: []string{"schema"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindPositional, Name: "1"},
			}},
			"files": {Values: []string{"one.md", "two.md"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindPositional, Name: "2+"},
			}},
			"tags": {Values: []string{"alpha", "beta"}, Sensitive: true, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindNamed, Name: "t", Redact: true},
				{Kind: work.ArgumentSourceKindNamed, Name: "tag", Redact: true},
			}},
			"format": {Values: []string{"json"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindDefault, Name: "default"},
			}},
			"count": {Values: []string{"2"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindNamed, Name: "count"},
			}},
			"document": {Values: []string{"story.md"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindNamed, Name: "file"},
			}},
			"body": {Values: []string{"stdin body"}, Sources: []work.ArgumentSource{
				{Kind: work.ArgumentSourceKindStdin, Name: "stdin"},
			}},
		},
	}}
}

func parityPreparedInvocation() work.PreparedInvocationInput {
	return work.PreparedInvocationInput{NormalizedArguments: &work.NormalizedArguments{
		Arguments: map[string]work.NormalizedArgument{
			"input":    {Values: []string{"draft"}},
			"mode":     {Values: []string{"fast"}},
			"confirm":  {Values: []string{"true"}},
			"artifact": {Values: []string{"result.md"}},
		},
	}}
}

func setupNamedFactoryInvocationTest(t *testing.T) (string, func()) {
	t.Helper()

	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	t.Setenv("HOME", homeDirectory)
	t.Setenv("USERPROFILE", homeDirectory)

	globalRoot, err := defaultNamedFactoriesRootForTest()
	if err != nil {
		t.Fatalf("DefaultGlobalNamedFactoryRoot: %v", err)
	}
	factoryDir := filepath.Join(globalRoot, "alpha")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", factoryDir, err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, interfaces.FactoryConfigFile), portableFactoryPayloadWithInvocationSignature(), 0o644); err != nil {
		t.Fatalf("WriteFile(factory.json): %v", err)
	}
	return factoryDir, func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}
}

func assertCompletePreparedInvocation(t *testing.T, prepared *work.PreparedInvocationInput) {
	t.Helper()

	if prepared == nil || prepared.NormalizedArguments == nil || prepared.ResolvedInput != nil {
		t.Fatalf("prepared input = %#v, want structured canonical input", prepared)
	}
	arguments := prepared.NormalizedArguments.Arguments
	assertCanonicalInvocationArgument(t, arguments, "topic", []string{"schema"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindPositional, Name: "1"}})
	assertCanonicalInvocationArgument(t, arguments, "files", []string{"one.md", "two.md"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindPositional, Name: "2+"}})
	assertCanonicalInvocationArgument(t, arguments, "tags", []string{"alpha", "beta"}, true,
		[]work.ArgumentSource{
			{Kind: work.ArgumentSourceKindNamed, Name: "t", Redact: true},
			{Kind: work.ArgumentSourceKindNamed, Name: "tag", Redact: true},
		})
	assertCanonicalInvocationArgument(t, arguments, "format", []string{"json"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindDefault, Name: "default"}})
	assertCanonicalInvocationArgument(t, arguments, "count", []string{"2"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindNamed, Name: "count"}})
	assertCanonicalInvocationArgument(t, arguments, "document", []string{"story.md"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindNamed, Name: "file"}})
	assertCanonicalInvocationArgument(t, arguments, "body", []string{"stdin body"}, false,
		[]work.ArgumentSource{{Kind: work.ArgumentSourceKindStdin, Name: "stdin"}})
}

func assertCompleteEffectiveValueModes(t *testing.T, signature *work.InvocationSignatureConfig) {
	t.Helper()

	if signature == nil {
		t.Fatal("effective signature was not supplied to Work")
	}
	got := make(map[string]string, len(signature.Parameters))
	for _, parameter := range signature.Parameters {
		got[parameter.Name] = work.NormalizeInvocationValueMode(parameter.ValueMode)
	}
	want := map[string]string{
		"topic":    work.InvocationParameterValueModeExact,
		"files":    work.InvocationParameterValueModeVariadic,
		"tags":     work.InvocationParameterValueModeRepeated,
		"format":   work.InvocationParameterValueModeExact,
		"count":    work.InvocationParameterValueModeExact,
		"document": work.InvocationParameterValueModeFileContents,
		"body":     work.InvocationParameterValueModeExact,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("effective value modes = %#v, want %#v", got, want)
	}
}

func assertCanonicalInvocationArgument(
	t *testing.T,
	arguments map[string]work.NormalizedArgument,
	name string,
	wantValues []string,
	wantSensitive bool,
	wantSources []work.ArgumentSource,
) {
	t.Helper()

	got, ok := arguments[name]
	if !ok {
		t.Fatalf("canonical argument %q is missing from %#v", name, arguments)
	}
	if !reflect.DeepEqual(got.Values, wantValues) ||
		got.Sensitive != wantSensitive ||
		!reflect.DeepEqual(got.Sources, wantSources) {
		t.Fatalf(
			"canonical argument %q = %#v, want values=%#v sensitive=%t sources=%#v",
			name, got, wantValues, wantSensitive, wantSources,
		)
	}
}

func completeInvocationSignaturePayload() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [
      {"name": "topic", "required": true, "bindings": [{"kind": "POSITIONAL", "position": 1}]},
      {"name": "files", "valueMode": "VARIADIC", "bindings": [{"kind": "POSITIONAL", "position": 2}]},
      {
        "name": "tags",
        "externalName": "tag",
        "aliases": ["t"],
        "valueMode": "REPEATED",
        "sensitive": true,
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "format",
        "choices": ["json", "text"],
        "defaultValue": "json",
        "bindings": [{"kind": "NAMED"}]
      },
      {"name": "count", "typeHint": "NUMBER_STRING", "bindings": [{"kind": "NAMED"}]},
      {
        "name": "document",
        "externalName": "document",
        "aliases": ["file"],
        "typeHint": "FILE_PATH",
        "valueMode": "FILE_CONTENTS",
        "bindings": [{"kind": "NAMED"}]
      },
      {"name": "body", "bindings": [{"kind": "STDIN"}]}
    ]
  }
}`)
}

func sensitiveInvocationSignaturePayload() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [{
      "name": "token",
      "externalName": "token",
      "sensitive": true,
      "choices": ["allowed"],
      "bindings": [{"kind": "NAMED"}]
    }]
  }
}`)
}

func defaultOnlyInvocationSignaturePayload() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [{
      "name": "mode",
      "defaultValue": "safe",
      "bindings": [{"kind": "NAMED"}]
    }]
  }
}`)
}

func collisionInvocationSignaturePayload() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [{
      "name": "reserved",
      "externalName": "quiet",
      "bindings": [{"kind": "NAMED"}]
    }]
  }
}`)
}

func portableFactoryPayloadWithDefaultHandling() []byte {
	return []byte(`{
  "name": "portable",
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`)
}

func portableFactoryPayloadWithInvocationSignature() []byte {
	return []byte(`{
  "name": "portable",
  "invocationSignature": {
    "parameters": [
      {
        "name": "input",
        "description": "Primary text input for the portable factory.",
        "required": true,
        "bindings": [{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}]
      },
      {
        "name": "mode",
        "description": "Execution mode for the portable factory.",
        "choices": ["fast", "safe"],
        "defaultValue": "safe",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "confirm",
        "typeHint": "BOOLEAN_STRING",
        "description": "Request confirmation mode.",
        "bindings": [{"kind": "NAMED"}]
      },
      {
        "name": "artifact",
        "description": "Optional output file path.",
        "aliases": ["out"],
        "typeHint": "FILE_PATH",
        "bindings": [{"kind": "NAMED"}]
      }
    ],
    "outputContract": {
      "mode": "FILE",
      "pathParameter": "artifact",
      "contentType": "text/plain",
      "fileExtension": ".txt"
    },
    "examples": [
      {
        "name": "positional-input",
        "argv": ["Fix the lint issues", "--mode", "safe", "--artifact", "report.md"]
      },
      {
        "name": "stdin-input",
        "argv": ["--mode", "fast"],
        "stdin": "Fix the lint issues"
      }
    ]
  },
  "workTypes": [{
    "name": "story",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workstations": [{
    "name": "ws",
    "inputs": [{"workType": "story", "state": "init"}],
    "outputs": [{"workType": "story", "state": "complete"}],
    "onFailure": [{"workType": "story", "state": "failed"}]
  }]
}`)
}
