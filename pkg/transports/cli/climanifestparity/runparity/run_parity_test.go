package runparity_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

func TestGeneratedVsLegacyParityMatrix_Run(t *testing.T) {
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(cli.RootCommandOptions{})
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacyRun, err := climanifestparity.FindCommandByPath(legacyRoot, "you run")
	if err != nil {
		t.Fatalf("find legacy run: %v", err)
	}
	generatedRun, err := climanifestparity.FindCommandByPath(generatedRoot, "you run")
	if err != nil {
		t.Fatalf("find generated run: %v", err)
	}
	if !legacyRun.DisableFlagParsing || !generatedRun.DisableFlagParsing {
		t.Fatalf("DisableFlagParsing legacy=%t generated=%t, want both true", legacyRun.DisableFlagParsing, generatedRun.DisableFlagParsing)
	}
	assertNoConstructorMismatches(t, climanifestparity.CompareConstructorIdentityParity("you.run", legacyRun, generatedRun))

	if mismatches, compareErr := climanifestparity.CompareConstructorHelpParity(
		"you.run", legacyRoot, generatedRoot, "you run",
	); compareErr != nil {
		t.Fatalf("CompareConstructorHelpParity() error = %v", compareErr)
	} else {
		assertNoConstructorMismatches(t, mismatches)
	}
	if mismatches, compareErr := climanifestparity.CompareConstructorCompletionInventoryParity(
		"you.run", "you run", legacyRoot, generatedRoot,
	); compareErr != nil {
		t.Fatalf("CompareConstructorCompletionInventoryParity() error = %v", compareErr)
	} else {
		assertNoConstructorMismatches(t, mismatches)
	}
}

func TestGeneratedVsLegacyRunExecutionParity(t *testing.T) {
	factoryPath := testutil.MustRepoPath(t, "examples/basic/factory/factory.json")
	tests := []struct {
		name  string
		argv  []string
		stdin string
	}{
		{name: "no input", argv: []string{"run"}},
		{name: "work file", argv: []string{"run", "--work", "batch.json"}},
		{name: "explicit stdin", argv: []string{"run", "--factory", factoryPath, "-"}, stdin: "from stdin\n"},
		{name: "implicit stdin", argv: []string{"run", "--factory", factoryPath}, stdin: "implicit stdin\n"},
		{
			name: "inline flags no-option flags and double dash",
			argv: []string{
				"--server=http://127.0.0.1:9090", "run", "--factory=" + factoryPath,
				"--output=response-stream", "--with-mock-workers", "--skip-permissions", "--", "literal prompt",
			},
		},
		{
			name: "separated values and global output flags",
			argv: []string{
				"--json", "--verbose", "run", "--factory", factoryPath, "--output", "primary",
				"--with-mock-workers", "mock-workers.json", "ship", "it",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			legacy, generated := executeRunOnParityRoots(t, tc.argv, tc.stdin, nil)
			assertRunExecutionParity(t, legacy, generated)
			if !legacy.called || !generated.called {
				t.Fatalf("run service call legacy=%t generated=%t, want both called", legacy.called, generated.called)
			}
		})
	}
}

func TestGeneratedVsLegacyRunRejectionAndFailureParity(t *testing.T) {
	factoryPath := testutil.MustRepoPath(t, "examples/basic/factory/factory.json")
	tests := []struct {
		name        string
		argv        []string
		stdin       string
		errContains string
	}{
		{name: "unknown prompt-like flag", argv: []string{"run", "--unknown-prompt"}, errContains: "unknown flag"},
		{name: "positional requires factory", argv: []string{"run", "prompt"}, errContains: "require --factory or --named"},
		{name: "selector conflict", argv: []string{"run", "--factory", factoryPath, "--dir", "factory"}, errContains: "--factory cannot be used with --dir"},
		{name: "work and prompt conflict", argv: []string{"run", "--factory", factoryPath, "--work", "batch.json", "prompt"}, errContains: "cannot be used with --work"},
		{name: "positional and stdin conflict", argv: []string{"run", "--factory", factoryPath, "prompt"}, stdin: "stdin prompt\n", errContains: "INVOCATION_INPUT_SOURCE_CONFLICT"},
		{name: "invalid output mode", argv: []string{"run", "--output", "yaml"}, errContains: "unsupported --output value"},
		{name: "missing separated value", argv: []string{"run", "--factory"}, errContains: "flag needs an argument"},
		{name: "deprecated port", argv: []string{"run", "--port", "9090"}, errContains: "--port is no longer supported"},
		{name: "remote server", argv: []string{"--server", "https://example.com", "run"}, errContains: "is not a local bind target"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			legacy, generated := executeRunOnParityRoots(t, tc.argv, tc.stdin, nil)
			assertRunExecutionParity(t, legacy, generated)
			if legacy.err == nil || generated.err == nil {
				t.Fatalf("rejection errors legacy=%v generated=%v, want both non-nil", legacy.err, generated.err)
			}
			if !strings.Contains(legacy.err.Error(), tc.errContains) {
				t.Fatalf("legacy error = %q, want substring %q", legacy.err, tc.errContains)
			}
			if legacy.called || generated.called {
				t.Fatalf("validation must precede service call: legacy=%t generated=%t", legacy.called, generated.called)
			}
		})
	}

	t.Run("service failure human and JSON", func(t *testing.T) {
		for _, argv := range [][]string{{"run"}, {"--json", "run"}} {
			legacy, generated := executeRunOnParityRoots(t, argv, "", errors.New("run service unavailable"))
			assertRunExecutionParity(t, legacy, generated)
			if !legacy.called || legacy.err == nil || !strings.Contains(legacy.stderr, "run service unavailable") {
				t.Fatalf("legacy failure observation = %+v", legacy)
			}
		}
	})
}

type runConfigObservation struct {
	Continuously            bool
	WorkFile                string
	Dir                     string
	FactoryConfigPath       string
	InvocationPositional    string
	InvocationStdin         string
	RecordPath              string
	ReplayPath              string
	DisableDefaultRecording bool
	MockWorkersEnabled      bool
	MockWorkersConfigPath   string
	Verbose                 bool
	SuppressDashboard       bool
	CleanInvocation         bool
	JSON                    bool
	JSONOutput              bool
	InvocationOutputMode    string
	SkipPermissions         string
	BindHost                string
	Port                    int
	AutoPort                bool
}

type runExecutionObservation struct {
	config runConfigObservation
	called bool
	stdout string
	stderr string
	err    error
}

func executeRunOnParityRoots(
	t *testing.T,
	argv []string,
	stdin string,
	runErr error,
) (runExecutionObservation, runExecutionObservation) {
	t.Helper()
	home := t.TempDir()
	observations := make([]runConfigObservation, 0, 2)
	options := cli.RootCommandOptions{
		HomeDir:   func() (string, error) { return home, nil },
		LookupEnv: func(string) (string, bool) { return "", false },
		RunFactory: func(_ context.Context, cfg runcli.RunConfig) error {
			observations = append(observations, observeRunConfig(cfg))
			if runErr != nil {
				return runErr
			}
			if cfg.Output != nil {
				if cfg.JSON {
					_, _ = io.WriteString(cfg.Output, "{\"result\":\"ok\"}\n")
				} else {
					_, _ = io.WriteString(cfg.Output, "result: ok\n")
				}
			}
			return nil
		},
	}
	legacyRoot, generatedRoot, err := cli.NewRunSubmitFamilyParityRoots(options)
	if err != nil {
		t.Fatalf("NewRunSubmitFamilyParityRoots() error = %v", err)
	}
	legacy := executeRunParityRoot(legacyRoot, argv, stdin)
	if len(observations) == 1 {
		legacy.called = true
		legacy.config = observations[0]
	}
	generated := executeRunParityRoot(generatedRoot, argv, stdin)
	if len(observations) == 2 {
		generated.called = true
		generated.config = observations[1]
	}
	return legacy, generated
}

func executeRunParityRoot(root *cobra.Command, argv []string, stdin string) runExecutionObservation {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return runExecutionObservation{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func observeRunConfig(cfg runcli.RunConfig) runConfigObservation {
	return runConfigObservation{
		Continuously:            cfg.Continuously,
		WorkFile:                cfg.WorkFile,
		Dir:                     cfg.Dir,
		FactoryConfigPath:       cfg.FactoryConfigPath,
		InvocationPositional:    stringValue(cfg.InvocationPositionalText),
		InvocationStdin:         stringValue(cfg.InvocationStdinText),
		RecordPath:              cfg.RecordPath,
		ReplayPath:              cfg.ReplayPath,
		DisableDefaultRecording: cfg.DisableDefaultRecording,
		MockWorkersEnabled:      cfg.MockWorkersEnabled,
		MockWorkersConfigPath:   cfg.MockWorkersConfigPath,
		Verbose:                 cfg.Verbose,
		SuppressDashboard:       cfg.SuppressDashboardRendering,
		CleanInvocation:         cfg.CleanInvocation,
		JSON:                    cfg.JSON,
		JSONOutput:              cfg.JSONOutput,
		InvocationOutputMode:    cfg.InvocationOutputMode,
		SkipPermissions:         boolValue(cfg.InvocationSkipPermissionsOverride),
		BindHost:                cfg.BindHost,
		Port:                    cfg.Port,
		AutoPort:                cfg.AutoPort,
	}
}

func stringValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func boolValue(value *bool) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%t", *value)
}

func assertRunExecutionParity(t *testing.T, legacy, generated runExecutionObservation) {
	t.Helper()
	if legacy.called != generated.called {
		t.Fatalf("service-call parity legacy=%t generated=%t", legacy.called, generated.called)
	}
	if !reflect.DeepEqual(legacy.config, generated.config) {
		t.Fatalf("run config mismatch:\nlegacy=%+v\ngenerated=%+v", legacy.config, generated.config)
	}
	if legacy.stdout != generated.stdout {
		t.Fatalf("stdout mismatch:\nlegacy=%q\ngenerated=%q", legacy.stdout, generated.stdout)
	}
	if legacy.stderr != generated.stderr {
		t.Fatalf("stderr mismatch:\nlegacy=%q\ngenerated=%q", legacy.stderr, generated.stderr)
	}
	if errorText(legacy.err) != errorText(generated.err) {
		t.Fatalf("error mismatch:\nlegacy=%q\ngenerated=%q", legacy.err, generated.err)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func assertNoConstructorMismatches(t *testing.T, mismatches []climanifestparity.Mismatch) {
	t.Helper()
	if len(mismatches) > 0 {
		t.Fatalf("generated vs legacy parity drift detected:\n%s", climanifestparity.FormatMismatchReport(mismatches))
	}
}
