package cli

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestReadModelsInvokeInputsClearsManifestOperationForNamedInput(t *testing.T) {
	t.Parallel()

	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("llm")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(modelinference.OperationTTS)},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{"prompt=Write a haiku"})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringArrayValue(nil)},
		},
	)
	if err != nil {
		t.Fatalf("resolve invoke inputs: %v", err)
	}

	got, err := readModelsInvokeInputs(inputs)
	if err != nil {
		t.Fatalf("readModelsInvokeInputs: %v", err)
	}
	if got.operation != "" {
		t.Fatalf("operation = %q, want empty so the built-in alias can infer OMNI", got.operation)
	}
	if !reflect.DeepEqual(got.inputMappings, []string{"prompt=Write a haiku"}) {
		t.Fatalf("input mappings = %#v, want named prompt", got.inputMappings)
	}
}

func TestInferGenericCLIModelOperationUsesBuiltInAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "llm", model: "  LLM ", want: modelinference.OperationOMNI},
		{name: "asr", model: modelinference.BuiltInModelNameASR, want: modelinference.OperationASR},
		{name: "tts", model: modelinference.BuiltInModelNameTTS, want: modelinference.OperationTTS},
		{name: "embed", model: modelinference.BuiltInModelNameEmbed, want: modelinference.OperationEMBED},
		{name: "authored", model: "custom-model", want: ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := inferGenericCLIModelOperation(test.model); got != test.want {
				t.Fatalf("inferGenericCLIModelOperation(%q) = %q, want %q", test.model, got, test.want)
			}
		})
	}
}

func TestReadModelsInvokeOutputsSupportsScalarAndNamedForms(t *testing.T) {
	t.Parallel()

	resolve := func(t *testing.T, outputKind resolvedinput.ValueKind, output resolvedinput.Value, legacy []string) resolvedinput.Inputs {
		t.Helper()
		definitions := []resolvedinput.Definition{{ID: modelsInvokeOutputID, Kind: outputKind, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}}}
		candidates := []resolvedinput.Candidate{{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceCLIFlag, Value: output}}
		if legacy != nil {
			definitions = append(definitions, resolvedinput.Definition{ID: modelsInvokeOutputMapID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}})
			candidates = append(candidates, resolvedinput.Candidate{InputID: modelsInvokeOutputMapID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue(legacy)})
		}
		inputs, err := resolvedinput.Resolve(definitions, candidates)
		if err != nil {
			t.Fatalf("resolve output inputs: %v", err)
		}
		return inputs
	}

	t.Run("scalar path remains supported", func(t *testing.T) {
		path, mappings, err := readModelsInvokeOutputs(resolve(t, resolvedinput.ValueKindString, resolvedinput.StringValue("answer.txt"), nil))
		if err != nil || path != "answer.txt" || len(mappings) != 0 {
			t.Fatalf("scalar output = path:%q mappings:%#v error:%v, want path", path, mappings, err)
		}
	})
	t.Run("named repeatable values", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"text=answer.txt"}), []string{"usage=usage.json"})
		path, mappings, err := readModelsInvokeOutputs(inputs)
		if err != nil || path != "" || !reflect.DeepEqual(mappings, []string{"text=answer.txt", "usage=usage.json"}) {
			t.Fatalf("named outputs = path:%q mappings:%#v error:%v", path, mappings, err)
		}
	})
	t.Run("path and mapping conflict", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"answer.txt", "text=answer.txt"}), nil)
		if _, _, err := readModelsInvokeOutputs(inputs); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("path/mapping conflict = %v, want conflict", err)
		}
	})
	t.Run("second unqualified path is rejected", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"one.txt", "two.txt"}), nil)
		if _, _, err := readModelsInvokeOutputs(inputs); err == nil || !strings.Contains(err.Error(), "after the first unqualified path") {
			t.Fatalf("multiple paths = %v, want repeatable path error", err)
		}
	})
}

func TestCommandHandlerTransformsRemoveArguments(t *testing.T) {
	server := "http://127.0.0.1:7437"
	called := false
	handler := NewCommandHandler(
		commandServiceFake{remove: func(cfg RemoveConfig) error {
			called = true
			if cfg.ModelName != "model-c" || cfg.Server != server || cfg.Context.Err() != context.Canceled {
				t.Fatalf("RemoveConfig = %#v", cfg)
			}
			return nil
		}},
		func(*cobra.Command) io.Writer { return io.Discard },
		nil,
		nil,
		nil,
	)
	cmd := &cobra.Command{Use: "models"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	_, _, inherited := resolvedModelsHandlerInputs(t, server)
	removeInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{ID: modelsRemoveNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}}},
		[]resolvedinput.Candidate{{InputID: modelsRemoveNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("model-c")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Remove(cmd, removeInputs, inherited); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("remove service operation was not called")
	}
}
