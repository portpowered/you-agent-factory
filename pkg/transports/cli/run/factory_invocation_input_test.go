package run

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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
