package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	modelscli "github.com/portpowered/infinite-you/pkg/services/models/transports/cli"
	"github.com/spf13/cobra"
)

// TestProductionModelsCLICharacterizationValidation pins the exact validation
// messages returned by the public command/handler composition. The operation
// double is only reached by the JSON success case, so missing-input assertions
// cannot accidentally depend on a runtime, HTTP server, or model asset.
func TestProductionModelsCLICharacterizationValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing text",
			args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--json"},
			want: "--text is required",
		},
		{
			name: "missing output without JSON",
			args: []string{"models", "invoke", "OMNIVOICE_Q4_K_M", "--text", "hello"},
			want: "--output is required unless --json is set",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			operation := &modelsCLICharacterizationInvocation{}
			root, _ := newModelsCLICharacterizationRoot(t, operation)
			root.SetArgs(testCase.args)

			err := root.Execute()
			if err == nil || err.Error() != testCase.want {
				t.Fatalf("execute %v error = %v, want exactly %q", testCase.args, err, testCase.want)
			}
			if operation.calls != 0 {
				t.Fatalf("validation invoked model operation %d time(s), want none", operation.calls)
			}
		})
	}
}

func TestProductionModelsCLICharacterizationJSONBypassesOutputRequirement(t *testing.T) {
	operation := &modelsCLICharacterizationInvocation{}
	root, stdout := newModelsCLICharacterizationRoot(t, operation)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M", "--json", "--text", "hello",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute JSON invoke without --output: %v", err)
	}
	var response map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON response %q: %v", stdout.String(), err)
	}
	if operation.calls != 1 {
		t.Fatalf("model operation calls = %d, want 1", operation.calls)
	}
	if operation.request.Operation != "TTS" {
		t.Fatalf("default operation = %q, want TTS", operation.request.Operation)
	}
	if response["operation"] != "TTS" {
		t.Fatalf("JSON operation = %#v, want TTS", response["operation"])
	}
}

// TestProductionModelsCLICharacterizationSuccessExit pins the successful
// command boundary for the audio projection. The current zero-error exit is
// characterized, not endorsed: artifact production is supplied by the
// deterministic Models service test and this test only proves Cobra preserves
// its success through the public command composition.
func TestProductionModelsCLICharacterizationSuccessExit(t *testing.T) {
	var got modelscli.InvokeConfig
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: modelsCLIServiceFunctions{
		invoke: func(cfg modelscli.InvokeConfig) error {
			got = cfg
			_, err := io.WriteString(cfg.Output, "Wrote audio: speech.wav\n")
			return err
		},
	}}))
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"models", "invoke", "OMNIVOICE_Q4_K_M", "--operation", "TTS",
		"--text", "hello world", "--output", "speech.wav",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute successful Models invoke: %v", err)
	}
	if got.OutputPath != "speech.wav" || got.Operation != "TTS" || got.Text != "hello world" {
		t.Fatalf("successful invoke config = %#v, want output/operation/text bindings", got)
	}
	if gotOutput, wantOutput := stdout.String(), "Wrote audio: speech.wav\n"; gotOutput != wantOutput {
		t.Fatalf("successful invoke stdout = %q, want %q", gotOutput, wantOutput)
	}
}

func TestProductionModelsCLICharacterizationRejectsInvalidInputsBeforeService(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing model name",
			args: []string{"models", "invoke"},
			want: "accepts 1 arg(s), received 0",
		},
		{
			name: "extra model name",
			args: []string{"models", "invoke", "model-a", "model-b"},
			want: "accepts 1 arg(s), received 2",
		},
		{
			name: "unknown flag",
			args: []string{"models", "invoke", "model-a", "--unknown"},
			want: "unknown flag: --unknown",
		},
		{
			name: "unsupported operation",
			args: []string{"models", "invoke", "model-a", "--operation", "INVALID"},
			want: "not one of the declared choices",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			called := false
			root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
				invoke: func(modelscli.InvokeConfig) error {
					called = true
					return nil
				},
			}}).NewCommand(nil, nil, nil)
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs(testCase.args)

			err := root.Execute()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("execute %v error = %v, want %q", testCase.args, err, testCase.want)
			}
			if called {
				t.Fatal("invalid Models input invoked the service")
			}
		})
	}
}

func TestProductionModelsCLICharacterizationRejectsChangedLegacyPort(t *testing.T) {
	called := false
	root := (CommandFactory{ModelsCLI: modelsCLIServiceFunctions{
		list: func(modelscli.ListConfig) error {
			called = true
			return nil
		},
	}}).NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"models", "list", "--port", "7437"})

	err := root.Execute()
	want := "--port is no longer supported; use --server instead (for example, --server http://localhost:7437)"
	if err == nil || err.Error() != want {
		t.Fatalf("execute changed --port error = %v, want exactly %q", err, want)
	}
	if called {
		t.Fatal("changed legacy --port invoked Models service")
	}
}

type modelsCLICharacterizationInvocation struct {
	calls   int
	request modelinference.Request
}

func (operation *modelsCLICharacterizationInvocation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	return explicit, nil
}

func (*modelsCLICharacterizationInvocation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (operation *modelsCLICharacterizationInvocation) InvokeModel(
	_ context.Context,
	_ modelscli.InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	operation.calls++
	operation.request = request
	return modelinference.Result{
		ModelName: modelName,
		Operation: operation.request.Operation,
	}, nil
}

func newModelsCLICharacterizationRoot(
	t *testing.T,
	operation *modelsCLICharacterizationInvocation,
) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	service := modelscli.New(rootTestHTTPProtocol(), operation)
	factory := withTestInjectedPlatformRoles(NewCommandFactory(CommandOperations{ModelsCLI: service}))
	root := factory.NewCommand(
		func() (string, error) { return t.TempDir(), nil },
		func(string) (string, bool) { return "", false },
		nil,
	)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	return root, &stdout
}
