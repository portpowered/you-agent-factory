package invocations

import (
	"os"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestInterpolateWorkerConfig_OmitsExactOptionalParameter(t *testing.T) {
	worker, err := InterpolateWorkerConfig(interfaces.WorkerConfig{
		Model: "${model}",
		Body:  "Use ${input}",
	}, &interfaces.InvocationArguments{
		Arguments: map[string]interfaces.InvocationArgument{
			"input": {Values: []string{"draft"}},
		},
	})
	if err != nil {
		t.Fatalf("InterpolateWorkerConfig: %v", err)
	}
	if worker.Model != "" {
		t.Fatalf("model = %q, want omitted exact-field interpolation to stay unset", worker.Model)
	}
	if worker.Body != "Use draft" {
		t.Fatalf("body = %q, want mixed interpolation to resolve provided value", worker.Body)
	}
}

func TestInterpolateWorkstationConfig_RejectsMissingEmbeddedParameter(t *testing.T) {
	_, err := InterpolateWorkstationConfig(interfaces.FactoryWorkstationConfig{
		PromptTemplate: "Use ${missing} now",
	}, &interfaces.InvocationArguments{})
	if err == nil {
		t.Fatal("InterpolateWorkstationConfig error = nil, want invalid interpolation")
	}
	argumentErr, ok := err.(*ArgumentError)
	if !ok {
		t.Fatalf("error = %T, want *ArgumentError", err)
	}
	if argumentErr.Code != ArgumentErrorCodeInvalidInterpolation {
		t.Fatalf("code = %q, want %q", argumentErr.Code, ArgumentErrorCodeInvalidInterpolation)
	}
}

func TestInterpolateWorkerConfig_ResolvesFileContentsValueMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "prompt.txt")
	if err := os.WriteFile(path, []byte("from-file"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	worker, err := InterpolateWorkerConfig(interfaces.WorkerConfig{
		Body: "${input}",
	}, &interfaces.InvocationArguments{
		Arguments: map[string]interfaces.InvocationArgument{
			"input": {
				Values:    []string{path},
				ValueMode: string(factoryapi.FactoryInvocationParameterValueModeFileContents),
			},
		},
	})
	if err != nil {
		t.Fatalf("InterpolateWorkerConfig: %v", err)
	}
	if worker.Body != "from-file" {
		t.Fatalf("body = %q, want file contents", worker.Body)
	}
}
