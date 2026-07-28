package invocationinterpolation

import (
	"fmt"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestInterpolateWorkerConfig_OmitsExactOptionalParameter(t *testing.T) {
	worker, err := InterpolateWorkerConfig(workerconfig.Config{
		Model: "${model}",
		Body:  "Use ${input}",
	}, &work.InvocationArguments{
		Arguments: map[string]work.InvocationArgument{
			"input": {Values: []string{"draft"}},
		},
	}, nil)
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
	_, err := InterpolateWorkstationConfig(factorydefinitions.FactoryWorkstationConfig{
		PromptTemplate: "Use ${missing} now",
	}, &work.InvocationArguments{}, nil)
	if err == nil {
		t.Fatal("InterpolateWorkstationConfig error = nil, want invalid interpolation")
	}
	argumentErr, ok := err.(*work.ArgumentError)
	if !ok {
		t.Fatalf("error = %T, want *ArgumentError", err)
	}
	if argumentErr.Code != factorydefinitions.ArgumentErrorCodeInvalidInterpolation {
		t.Fatalf("code = %q, want %q", argumentErr.Code, factorydefinitions.ArgumentErrorCodeInvalidInterpolation)
	}
}

func TestInterpolateWorkerConfig_ResolvesFileContentsValueMode(t *testing.T) {
	const path = "prompt.txt"
	readFile := func(got string) ([]byte, error) {
		if got != path {
			return nil, fmt.Errorf("path = %q, want %q", got, path)
		}
		return []byte("from-file"), nil
	}

	worker, err := InterpolateWorkerConfig(workerconfig.Config{
		Body: "${input}",
	}, &work.InvocationArguments{
		Arguments: map[string]work.InvocationArgument{
			"input": {
				Values:    []string{path},
				ValueMode: work.InvocationParameterValueModeFileContents,
			},
		},
	}, readFile)
	if err != nil {
		t.Fatalf("InterpolateWorkerConfig: %v", err)
	}
	if worker.Body != "from-file" {
		t.Fatalf("body = %q, want file contents", worker.Body)
	}
}
