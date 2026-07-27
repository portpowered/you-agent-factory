package climanifest

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestComposeRunInputsProducesCompleteEffectiveSchema(t *testing.T) {
	manifest, signature := completeCompositionFixture()
	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
	}
	if len(effective.StaticInputs) != 3 {
		t.Fatalf("static inputs = %#v, want all global and run inputs", effective.StaticInputs)
	}
	if effective.UnknownNamedArgumentPolicy != work.InvocationUnknownNamedArgumentPolicyAllow {
		t.Fatalf("unknown named argument policy = %q", effective.UnknownNamedArgumentPolicy)
	}
	want := completeEffectiveParameters()
	if !reflect.DeepEqual(effective.FactoryParameters, want) {
		t.Fatalf("Factory parameters = %#v, want %#v", effective.FactoryParameters, want)
	}

	again, againDiagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(againDiagnostics) != 0 || !reflect.DeepEqual(again, effective) {
		t.Fatalf("repeated composition differs: first=%#v second=%#v diagnostics=%#v err=%v", effective, again, againDiagnostics, err)
	}
}

func TestComposeRunInputsDetachesCallerAndResultCollections(t *testing.T) {
	manifest, signature := completeCompositionFixture()
	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
	}
	again, againDiagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(againDiagnostics) != 0 {
		t.Fatalf("second ComposeRunInputs() err=%v diagnostics=%#v", err, againDiagnostics)
	}

	effective.StaticInputs[0].PublicSpellings[0] = "changed"
	effective.FactoryParameters[0].Aliases[0] = "changed"
	effective.FactoryParameters[0].Choices[0] = "changed"
	effective.FactoryParameters[0].Bindings[0].Kind = "changed"
	effective.FactoryParameters[1].DefaultValues[0] = "changed"
	if manifest.Commands["you.run"].Flags["global"].Long != "verbose" {
		t.Fatal("composition mutated the static manifest")
	}
	if signature.Parameters[0].Aliases[0] != "exact-alias" || signature.Parameters[0].Choices[0] != "true" ||
		signature.Parameters[0].Bindings[0].Kind != work.InvocationParameterBindingKindNamed ||
		signature.Parameters[1].DefaultValues[0] != "one" {
		t.Fatal("composition mutated the Factory invocation signature")
	}
	if !reflect.DeepEqual(again.FactoryParameters, completeEffectiveParameters()) || again.StaticInputs[0].PublicSpellings[0] == "changed" {
		t.Fatalf("returned schema shares caller-owned or prior result collections: %#v", again)
	}

	signature.Parameters[0].Aliases[0] = "caller-changed"
	signature.Parameters[0].Choices[0] = "caller-changed"
	signature.Parameters[0].Bindings[0].Kind = "caller-changed"
	signature.Parameters[1].DefaultValues[0] = "caller-changed"
	staticCommand := manifest.Commands["you.run"]
	outputFlag := staticCommand.Flags["output"]
	outputFlag.Aliases[0] = "caller-changed"
	staticCommand.Flags["output"] = outputFlag
	manifest.Commands["you.run"] = staticCommand
	if !reflect.DeepEqual(again.FactoryParameters, completeEffectiveParameters()) || staticInputByID(t, again.StaticInputs, "you.run.flag.output").PublicSpellings[1] != "emit" {
		t.Fatalf("caller-owned mutations changed a resolved schema: %#v", again)
	}
}

func TestComposeRunInputsPreservesSupportedTypeHints(t *testing.T) {
	supported := []string{
		work.InvocationParameterTypeHintString,
		work.InvocationParameterTypeHintPath,
		work.InvocationParameterTypeHintFilePath,
		work.InvocationParameterTypeHintDirectoryPath,
		work.InvocationParameterTypeHintNumberString,
		work.InvocationParameterTypeHintBooleanString,
		work.InvocationParameterTypeHintJSON,
	}
	for _, typeHint := range supported {
		t.Run(typeHint, func(t *testing.T) {
			signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
				Name: "value", TypeHint: typeHint,
				Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
			}}}
			effective, diagnostics, err := ComposeRunInputs(compositionManifest(), "you.run", &signature)
			if err != nil || len(diagnostics) != 0 {
				t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
			}
			if got := effective.FactoryParameters[0].TypeHint; got != typeHint {
				t.Fatalf("TypeHint = %q, want %q", got, typeHint)
			}
		})
	}
}

func TestComposeRunInputsRejectsEveryReservedStaticNamespace(t *testing.T) {
	manifest := compositionManifest()
	root := manifest.Commands["you"]
	root.Aliases = []string{"execute"}
	manifest.Commands["you"] = root
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{
		{Name: " you.run.binding.output ", ExternalName: " output ", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "run", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 2}}},
		{Name: "command-alias", ExternalName: "execute", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "alias", Aliases: []string{" emit "}, Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "short", ExternalName: "v", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "position", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 1}}},
		{Name: "stdin", Bindings: []work.InvocationParameterBindingConfig{{Kind: " STDIN "}}},
		{Name: "you.run.flag.named", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 2}}},
	}}

	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil {
		t.Fatalf("ComposeRunInputs() error = %v", err)
	}
	if !reflect.DeepEqual(effective, EffectiveInputSchema{}) {
		t.Fatalf("effective schema = %#v, want rejected composition to produce no schema", effective)
	}
	want := []CompositionDiagnostic{
		{Code: CompositionCollisionAlias, Path: "/invocationSignature/parameters/3/aliases/0", StaticOwner: "you.run.flag.output", FactoryOwner: "alias"},
		{Code: CompositionCollisionBindingID, Path: "/invocationSignature/parameters/0/name", StaticOwner: "you.run.flag.output", FactoryOwner: "you.run.binding.output"},
		{Code: CompositionCollisionBindingID, Path: "/invocationSignature/parameters/7/name", StaticOwner: "you.run.flag.named", FactoryOwner: "you.run.flag.named"},
		{Code: CompositionCollisionCommandName, Path: "/invocationSignature/parameters/1/name", StaticOwner: "you.run", FactoryOwner: "run"},
		{Code: CompositionCollisionCommandName, Path: "/invocationSignature/parameters/2/externalName", StaticOwner: "you", FactoryOwner: "command-alias"},
		{Code: CompositionCollisionLongName, Path: "/invocationSignature/parameters/0/externalName", StaticOwner: "you.run.flag.output", FactoryOwner: "you.run.binding.output"},
		{Code: CompositionCollisionShorthand, Path: "/invocationSignature/parameters/4/externalName", StaticOwner: "you.flag.verbose", FactoryOwner: "short"},
	}
	if len(diagnostics) != len(want) {
		t.Fatalf("diagnostics = %#v, want %#v", diagnostics, want)
	}
	for index := range want {
		got := diagnostics[index]
		if got.Code != want[index].Code || got.Path != want[index].Path || got.StaticOwner != want[index].StaticOwner || got.FactoryOwner != want[index].FactoryOwner {
			t.Fatalf("diagnostic[%d] = %#v, want %#v", index, got, want[index])
		}
		if got.Message == "" {
			t.Fatalf("diagnostic[%d] has no owner-aware message", index)
		}
	}
}

func TestComposeRunInputsCollisionDiagnosticsAreDeterministic(t *testing.T) {
	manifest := compositionManifest()
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{
		{Name: "output", Aliases: []string{"v", "emit"}, Bindings: []work.InvocationParameterBindingConfig{
			{Kind: work.InvocationParameterBindingKindPositional, Position: 1},
			{Kind: work.InvocationParameterBindingKindStdin},
		}},
	}}

	_, want, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(want) == 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, want)
	}
	for iteration := 0; iteration < 25; iteration++ {
		_, got, repeatErr := ComposeRunInputs(manifest, "you.run", &signature)
		if repeatErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d diagnostics=%#v err=%v, want %#v", iteration, got, repeatErr, want)
		}
	}
}

func TestComposeRunInputsPreservesRunFlagsAndReplacesCompatibilityInput(t *testing.T) {
	manifest := compositionManifest()
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "search", Aliases: []string{"q"},
		Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}

	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", &signature)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
	}
	want := projectStaticInputs(commandWithoutCompatibilityInvocationInput(manifest.Commands["you.run"]))
	if !reflect.DeepEqual(effective.StaticInputs, want) {
		t.Fatalf("effective static inputs = %#v, want run flags without compatibility carrier %#v", effective.StaticInputs, want)
	}
}

func TestComposeRunInputsNamedAndFileSelectionsAreEquivalent(t *testing.T) {
	manifest := compositionManifest()
	namedFactorySignature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "query", DefaultValue: "all", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}
	fileFactorySignature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "query", DefaultValue: "all", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}

	named, namedDiagnostics, err := ComposeRunInputs(manifest, "you.run", &namedFactorySignature)
	if err != nil {
		t.Fatalf("named composition error = %v", err)
	}
	file, fileDiagnostics, err := ComposeRunInputs(manifest, "you.run", &fileFactorySignature)
	if err != nil {
		t.Fatalf("file composition error = %v", err)
	}
	if !reflect.DeepEqual(named, file) || !reflect.DeepEqual(namedDiagnostics, fileDiagnostics) {
		t.Fatalf("equivalent selections differ: named=%#v/%#v file=%#v/%#v", named, namedDiagnostics, file, fileDiagnostics)
	}

	namedColliding := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "output", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}
	fileColliding := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "output", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}
	_, namedCollisionDiagnostics, _ := ComposeRunInputs(manifest, "you.run", &namedColliding)
	_, fileCollisionDiagnostics, _ := ComposeRunInputs(manifest, "you.run", &fileColliding)
	if !reflect.DeepEqual(namedCollisionDiagnostics, fileCollisionDiagnostics) {
		t.Fatalf("equivalent selection diagnostics differ: named=%#v file=%#v", namedCollisionDiagnostics, fileCollisionDiagnostics)
	}
}

func TestComposeRunInputsRequiresStaticCommand(t *testing.T) {
	if _, _, err := ComposeRunInputs(Manifest{}, "you.run", nil); err == nil {
		t.Fatal("ComposeRunInputs() error = nil, want missing static command failure")
	}
}

func TestComposeRunInputsKeepsNoSignatureCompatibilityDistinct(t *testing.T) {
	manifest := compositionManifest()
	compatibility, diagnostics, err := ComposeRunInputs(manifest, "you.run", nil)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("compatibility composition err=%v diagnostics=%#v", err, diagnostics)
	}
	if compatibility.FactoryInputMode != EffectiveFactoryInputModeCompatibility {
		t.Fatalf("FactoryInputMode = %q, want compatibility", compatibility.FactoryInputMode)
	}
	if compatibility.UnknownNamedArgumentPolicy != "" || len(compatibility.FactoryParameters) != 0 {
		t.Fatalf("no-signature schema synthesized signature facts: %#v", compatibility)
	}
	if !reflect.DeepEqual(compatibility.StaticInputs, projectStaticInputs(manifest.Commands["you.run"])) {
		t.Fatalf("compatibility static inputs = %#v", compatibility.StaticInputs)
	}

	emptySignature := work.InvocationSignatureConfig{}
	signature, signatureDiagnostics, err := ComposeRunInputs(manifest, "you.run", &emptySignature)
	if err != nil || len(signatureDiagnostics) != 0 {
		t.Fatalf("empty signature composition err=%v diagnostics=%#v", err, signatureDiagnostics)
	}
	if signature.FactoryInputMode != EffectiveFactoryInputModeSignature ||
		signature.UnknownNamedArgumentPolicy != work.InvocationUnknownNamedArgumentPolicyReject {
		t.Fatalf("empty active signature facts = %#v", signature)
	}
}

func compositionManifest() Manifest {
	return Manifest{Commands: map[string]Command{
		"you": {ID: "you", Name: "you"},
		"you.run": {
			ID:   "you.run",
			Name: "run",
			Arguments: map[string]Argument{
				"input": {ID: "you.run.arg.0", Name: "input", Position: 0, Channels: []string{SourceCLI, SourceStdin}},
			},
			Flags: map[string]Flag{
				"global": {ID: "you.flag.verbose", Long: "verbose", Shorthand: "v", Scope: "inherited"},
				"output": {ID: "you.run.flag.output", Long: "output", Aliases: []string{"emit"}, Scope: "local", HandlerBindingID: "you.run.binding.output"},
				"named":  {ID: "you.run.flag.named", Long: "named", Scope: "local"},
			},
		},
	}}
}

func completeCompositionFixture() (Manifest, work.InvocationSignatureConfig) {
	manifest := compositionManifest()
	command := manifest.Commands["you.run"]
	delete(command.Arguments, "input")
	manifest.Commands["you.run"] = command
	signature := work.InvocationSignatureConfig{
		UnknownNamedArgumentPolicy: work.InvocationUnknownNamedArgumentPolicyAllow,
		Parameters: []work.InvocationParameterConfig{
			{
				Name: "exact", Description: "one exact string", ExternalName: "exact-value",
				Aliases: []string{"exact-alias"}, TypeHint: work.InvocationParameterTypeHintBooleanString,
				Required: true, Sensitive: true, Choices: []string{"true", "false"}, DefaultValue: "true",
				Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
			},
			{
				Name: "files", TypeHint: work.InvocationParameterTypeHintFilePath,
				ValueMode: work.InvocationParameterValueModeRepeated, DefaultValues: []string{"one", "two"},
				Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
			},
			{
				Name: "input", ValueMode: work.InvocationParameterValueModeFileContents,
				TypeHint: work.InvocationParameterTypeHintPath,
				Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindStdin}},
			},
			{
				Name: "rest", ValueMode: work.InvocationParameterValueModeVariadic,
				TypeHint: work.InvocationParameterTypeHintDirectoryPath,
				Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 1}},
			},
		},
	}
	return manifest, signature
}

func completeEffectiveParameters() []EffectiveFactoryParameter {
	return []EffectiveFactoryParameter{
		{
			BindingID: "exact", CanonicalName: "exact", PreferredExternalName: "exact-value",
			Aliases: []string{"exact-alias"}, Description: "one exact string", Required: true,
			Choices: []string{"true", "false"}, DefaultValue: stringPointer("true"),
			ValueMode: work.InvocationParameterValueModeExact, ValueConsumption: EffectiveValueConsumptionSingle,
			MinimumValues: 1, MaximumValues: 1, TypeHint: work.InvocationParameterTypeHintBooleanString,
			Sensitive: true, Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
		},
		{
			BindingID: "files", CanonicalName: "files", PreferredExternalName: "files",
			DefaultValues: []string{"one", "two"},
			ValueMode:     work.InvocationParameterValueModeRepeated, ValueConsumption: EffectiveValueConsumptionRepeated,
			MinimumValues: 0, MaximumValues: EffectiveUnboundedCardinality, TypeHint: work.InvocationParameterTypeHintFilePath,
			Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
		},
		{
			BindingID: "input", CanonicalName: "input", PreferredExternalName: "input",
			ValueMode: work.InvocationParameterValueModeFileContents, ValueConsumption: EffectiveValueConsumptionFileContents,
			MinimumValues: 0, MaximumValues: 1, TypeHint: work.InvocationParameterTypeHintPath,
			Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindStdin}},
		},
		{
			BindingID: "rest", CanonicalName: "rest", PreferredExternalName: "rest",
			ValueMode: work.InvocationParameterValueModeVariadic, ValueConsumption: EffectiveValueConsumptionRemainingPositionals,
			MinimumValues: 0, MaximumValues: EffectiveUnboundedCardinality, TypeHint: work.InvocationParameterTypeHintDirectoryPath,
			Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 1}},
		},
	}
}

func stringPointer(value string) *string {
	return &value
}

func staticInputByID(t *testing.T, inputs []EffectiveStaticInput, id string) EffectiveStaticInput {
	t.Helper()
	for _, input := range inputs {
		if input.ID == id {
			return input
		}
	}
	t.Fatalf("static input %q not found in %#v", id, inputs)
	return EffectiveStaticInput{}
}
