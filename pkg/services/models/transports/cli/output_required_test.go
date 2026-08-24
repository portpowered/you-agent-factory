package cli

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestModelCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"list": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background()})
		},
		"inspect": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background()})
		},
		"invoke": func() error { return invokeForTest(t, InvokeConfig{Context: context.Background()}) },
		"pull": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background()})
		},
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}

func TestParseGenericCLIInputsPreservesOrderAndInputForms(t *testing.T) {
	t.Parallel()

	readPaths := make([]string, 0, 2)
	inputs, err := parseGenericCLIInputs(context.Background(), []string{
		"text=Find similar work",
		"parameters=json:{\"normalize\":true}",
		"text=@notes.md",
		"parameters=@params.json",
	}, func(path string) ([]byte, error) {
		readPaths = append(readPaths, path)
		switch path {
		case "notes.md":
			return []byte("file-backed text"), nil
		case "params.json":
			return []byte(`{"dimensions":4}`), nil
		default:
			return nil, errors.New("unexpected path")
		}
	})
	if err != nil {
		t.Fatalf("parseGenericCLIInputs() error = %v", err)
	}
	want := []modelinference.InferenceInput{
		{Name: "text", Modality: modelinference.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "Find similar work"},
		{Name: "parameters", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"normalize":true}`},
		{Name: "text", Modality: modelinference.ModalityText, ContentType: "text/plain", MediaType: "text/plain", Content: "file-backed text"},
		{Name: "parameters", Modality: modelinference.ModalityJSON, ContentType: "application/json", MediaType: "application/json", Content: `{"dimensions":4}`},
	}
	if !reflect.DeepEqual(inputs, want) {
		t.Fatalf("parsed inputs = %#v, want %#v", inputs, want)
	}
	if !reflect.DeepEqual(readPaths, []string{"notes.md", "params.json"}) {
		t.Fatalf("read paths = %#v, want notes.md then params.json", readPaths)
	}
}

func TestParseGenericCLIInputsRejectsMalformedBeforeFileRead(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		binding string
		want    string
	}{
		{name: "missing equals", binding: "text", want: "expected slot=value"},
		{name: "empty slot", binding: "=hello", want: "slot name is required"},
		{name: "empty value", binding: "text=", want: "value is required"},
		{name: "invalid JSON", binding: `parameters=json:{"broken"}`, want: "invalid JSON input"},
		{name: "multiple JSON values", binding: "parameters=json:1 2", want: "multiple JSON values"},
		{name: "empty file path", binding: "text=@", want: "path is required after @"},
	} {
		t.Run(test.name, func(t *testing.T) {
			readCalled := false
			_, err := parseGenericCLIInputs(context.Background(), []string{test.binding}, func(string) ([]byte, error) {
				readCalled = true
				return nil, nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			if readCalled {
				t.Fatal("file reader called for malformed binding")
			}
		})
	}
}

func TestParseGenericCLIInputsChecksCancellationAndSize(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readCalled := false
	_, err := parseGenericCLIInputs(ctx, []string{"text=@input.txt"}, func(string) ([]byte, error) {
		readCalled = true
		return []byte("unreachable"), nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled parse error = %v, want context.Canceled", err)
	}
	if readCalled {
		t.Fatal("cancelled parse called file reader")
	}

	_, err = parseGenericCLIInputs(context.Background(), []string{"text=@large.txt"}, func(string) ([]byte, error) {
		return bytes.Repeat([]byte("x"), maxGenericCLIInputBytes+1), nil
	})
	if err == nil || !strings.Contains(err.Error(), "16777216-byte limit") {
		t.Fatalf("oversized parse error = %v, want bounded input error", err)
	}
}

func TestReadModelsInvokeInputsMapsRepeatableGenericBindingsAndInfersOperation(t *testing.T) {
	t.Parallel()

	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceManifestDefault}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("embed")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("TTS")},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{
				"text=Find similar work", `parameters=json:{"normalize":true}`,
			})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("")},
		},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	parsed, err := readModelsInvokeInputs(inputs)
	if err != nil {
		t.Fatalf("readModelsInvokeInputs() error = %v", err)
	}
	if parsed.modelName != "embed" || parsed.operation != "" || parsed.text != "" || parsed.outputPath != "" {
		t.Fatalf("parsed generic invoke inputs = %#v, want embed with inferred operation and no legacy text/output", parsed)
	}
	if !reflect.DeepEqual(parsed.inputBindings, []string{"text=Find similar work", `parameters=json:{"normalize":true}`}) {
		t.Fatalf("parsed input bindings = %#v, want authored order", parsed.inputBindings)
	}
}
