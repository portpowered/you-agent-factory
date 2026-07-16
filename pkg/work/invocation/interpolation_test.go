package invocation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestInterpolateWorkstationConfig_RejectsUnsafeWorktree(t *testing.T) {
	for _, worktree := range []string{"/absolute", "C:\\absolute", "../outside", "..\\outside", ".."} {
		t.Run(worktree, func(t *testing.T) {
			_, err := InterpolateWorkstationConfig(interfaces.FactoryWorkstationConfig{
				Worktree: "${worktree}",
			}, &interfaces.InvocationArguments{Arguments: map[string]interfaces.InvocationArgument{
				"worktree": {Values: []string{worktree}},
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), "workstation.worktree") {
				t.Fatalf("InterpolateWorkstationConfig error = %v, want actionable worktree validation error", err)
			}
		})
	}
}

func TestInterpolateWorkstationConfig_AcceptsRelativeWorktree(t *testing.T) {
	workstation, err := InterpolateWorkstationConfig(interfaces.FactoryWorkstationConfig{
		Worktree: "${worktree}",
	}, &interfaces.InvocationArguments{Arguments: map[string]interfaces.InvocationArgument{
		"worktree": {Values: []string{"release/dashboard"}},
	}}, nil)
	if err != nil {
		t.Fatalf("InterpolateWorkstationConfig: %v", err)
	}
	if workstation.Worktree != "release/dashboard" {
		t.Fatalf("worktree = %q, want relative worktree", workstation.Worktree)
	}
}

func TestInterpolateWorkerConfig_OmitsExactOptionalParameter(t *testing.T) {
	worker, err := InterpolateWorkerConfig(interfaces.WorkerConfig{
		Model: "${model}",
		Body:  "Use ${input}",
	}, &interfaces.InvocationArguments{
		Arguments: map[string]interfaces.InvocationArgument{
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
	_, err := InterpolateWorkstationConfig(interfaces.FactoryWorkstationConfig{
		PromptTemplate: "Use ${missing} now",
	}, &interfaces.InvocationArguments{}, nil)
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
	const path = "prompt.txt"
	readFile := func(got string) ([]byte, error) {
		if got != path {
			return nil, fmt.Errorf("path = %q, want %q", got, path)
		}
		return []byte("from-file"), nil
	}

	worker, err := InterpolateWorkerConfig(interfaces.WorkerConfig{
		Body: "${input}",
	}, &interfaces.InvocationArguments{
		Arguments: map[string]interfaces.InvocationArgument{
			"input": {
				Values:    []string{path},
				ValueMode: valueModeFileContents,
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

func TestInvocationDiagnostic_RedactsSensitiveValuesAndPreservesSources(t *testing.T) {
	diagnostic := InvocationDiagnostic(&interfaces.InvocationSignatureConfig{
		Parameters: []interfaces.InvocationParameterConfig{{Name: "apiKey"}},
	}, &interfaces.InvocationArguments{
		Arguments: map[string]interfaces.InvocationArgument{
			"apiKey": {
				Values:    []string{"secret"},
				Sensitive: true,
				Sources: []interfaces.InvocationArgumentSource{{
					Kind:   "NAMED",
					Name:   "api-key",
					Redact: true,
				}},
			},
		},
	})
	if diagnostic == nil {
		t.Fatal("InvocationDiagnostic = nil, want summary")
	}
	if diagnostic.SignatureHash == "" {
		t.Fatal("SignatureHash = empty, want stable digest")
	}
	if len(diagnostic.Parameters) != 1 {
		t.Fatalf("parameter count = %d, want 1", len(diagnostic.Parameters))
	}
	if diagnostic.Parameters[0].Redacted != true {
		t.Fatalf("redacted = %v, want true", diagnostic.Parameters[0].Redacted)
	}
	if diagnostic.Parameters[0].ValueCount != 1 {
		t.Fatalf("value count = %d, want 1", diagnostic.Parameters[0].ValueCount)
	}
	if len(diagnostic.Parameters[0].SourceKinds) != 1 || diagnostic.Parameters[0].SourceKinds[0] != "NAMED" {
		t.Fatalf("source kinds = %#v, want [NAMED]", diagnostic.Parameters[0].SourceKinds)
	}
}
