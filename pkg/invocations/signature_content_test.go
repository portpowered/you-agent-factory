package invocations

import (
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestContentFromNormalizedArguments_MaterializesTypedSignatureInputs(t *testing.T) {
	directory := t.TempDir()
	wavPath := filepath.Join(directory, "reference.wav")
	textPath := filepath.Join(directory, "reference.txt")
	if err := os.WriteFile(wavPath, []byte("RIFF"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(textPath, []byte("reference transcript"), 0o644); err != nil {
		t.Fatal(err)
	}
	signature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
		{Name: "text", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1}}},
		{Name: "reference_audio", TypeHint: string(factoryapi.FactoryInvocationParameterTypeHintFilePath), Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}}},
		{Name: "reference_text", ValueMode: string(factoryapi.FactoryInvocationParameterValueModeFileContents), Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}}},
	}}
	normalized, err := NormalizeArguments(NormalizeArgumentsInput{Signature: signature, PositionalArgs: []string{"hello"}, NamedArgs: []NamedArgumentInput{{Key: "reference_audio", Values: []string{wavPath}}, {Key: "reference_text", Values: []string{textPath}}}})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	content, err := ContentFromNormalizedArguments(signature, &normalized)
	if err != nil {
		t.Fatalf("ContentFromNormalizedArguments: %v", err)
	}
	if len(content) != 3 {
		t.Fatalf("content = %#v, want three parts", content)
	}
	if content[0].Type.Normalized() != interfaces.WorkContentPartTypeText || content[0].Label != "text" || content[0].Text != "hello" {
		t.Fatalf("text content = %#v", content[0])
	}
	if content[1].Type.Normalized() != interfaces.WorkContentPartTypeAudio || content[1].Label != "reference_audio" || content[1].File != wavPath {
		t.Fatalf("audio content = %#v", content[1])
	}
	if content[2].Type.Normalized() != interfaces.WorkContentPartTypeText || content[2].Label != "reference_text" || content[2].Text != "reference transcript" {
		t.Fatalf("reference text content = %#v", content[2])
	}
}
