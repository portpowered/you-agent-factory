package climanifest

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestComposeRunInputsProducesImmutableEffectiveSchema(t *testing.T) {
	manifest := compositionManifest()
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{
		{
			Name:          "prompt",
			ExternalName:  "prompt",
			Aliases:       []string{"request"},
			DefaultValues: []string{"one", "two"},
			Bindings:      []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
		},
	}}

	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", signature)
	if err != nil || len(diagnostics) != 0 {
		t.Fatalf("ComposeRunInputs() err=%v diagnostics=%#v", err, diagnostics)
	}
	if len(effective.StaticInputs) != 4 {
		t.Fatalf("static inputs = %#v, want all global and run inputs", effective.StaticInputs)
	}
	if got := effective.FactoryParameters[0]; got.BindingID != "prompt" || !reflect.DeepEqual(got.Parameter.DefaultValues, []string{"one", "two"}) {
		t.Fatalf("Factory parameter = %#v, want signature parameter and defaults", got)
	}

	effective.StaticInputs[0].PublicSpellings[0] = "changed"
	effective.FactoryParameters[0].Parameter.Aliases[0] = "changed"
	effective.FactoryParameters[0].Parameter.DefaultValues[0] = "changed"
	effective.FactoryParameters[0].Parameter.Bindings[0].Kind = "changed"
	if manifest.Commands["you.run"].Flags["global"].Long != "verbose" {
		t.Fatal("composition mutated the static manifest")
	}
	if signature.Parameters[0].Aliases[0] != "request" || signature.Parameters[0].DefaultValues[0] != "one" || signature.Parameters[0].Bindings[0].Kind != work.InvocationParameterBindingKindNamed {
		t.Fatal("composition mutated the Factory invocation signature")
	}
}

func TestComposeRunInputsRejectsEveryReservedStaticNamespace(t *testing.T) {
	manifest := compositionManifest()
	signature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{
		{Name: "you.run.binding.output", ExternalName: "output", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "command", ExternalName: "run", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "alias", ExternalName: "emit", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "short", ExternalName: "v", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}}},
		{Name: "position", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindPositional, Position: 1}}},
		{Name: "stdin", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindStdin}}},
	}}

	effective, diagnostics, err := ComposeRunInputs(manifest, "you.run", signature)
	if err != nil {
		t.Fatalf("ComposeRunInputs() error = %v", err)
	}
	if !reflect.DeepEqual(effective, EffectiveInputSchema{}) {
		t.Fatalf("effective schema = %#v, want rejected composition to produce no schema", effective)
	}
	want := []CompositionDiagnostic{
		{Code: CompositionCollisionAlias, Path: "/invocationSignature/parameters/2/externalName", StaticOwner: "you.run.flag.output", FactoryOwner: "alias"},
		{Code: CompositionCollisionBindingID, Path: "/invocationSignature/parameters/0/name", StaticOwner: "you.run.flag.output", FactoryOwner: "you.run.binding.output"},
		{Code: CompositionCollisionCommandName, Path: "/invocationSignature/parameters/1/externalName", StaticOwner: "you.run", FactoryOwner: "command"},
		{Code: CompositionCollisionLongName, Path: "/invocationSignature/parameters/0/externalName", StaticOwner: "you.run.flag.output", FactoryOwner: "you.run.binding.output"},
		{Code: CompositionCollisionPosition, Path: "/invocationSignature/parameters/4/bindings/0/position", StaticOwner: "you.run.arg.0", FactoryOwner: "position"},
		{Code: CompositionCollisionShorthand, Path: "/invocationSignature/parameters/3/externalName", StaticOwner: "you.flag.verbose", FactoryOwner: "short"},
		{Code: CompositionCollisionStdin, Path: "/invocationSignature/parameters/5/bindings/0/kind", StaticOwner: "you.run.arg.0", FactoryOwner: "stdin"},
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

func TestComposeRunInputsNamedAndFileSelectionsAreEquivalent(t *testing.T) {
	manifest := compositionManifest()
	namedFactorySignature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "query", DefaultValue: "all", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}
	fileFactorySignature := work.InvocationSignatureConfig{Parameters: []work.InvocationParameterConfig{{
		Name: "query", ExternalName: "query", DefaultValue: "all", Bindings: []work.InvocationParameterBindingConfig{{Kind: work.InvocationParameterBindingKindNamed}},
	}}}

	named, namedDiagnostics, err := ComposeRunInputs(manifest, "you.run", namedFactorySignature)
	if err != nil {
		t.Fatalf("named composition error = %v", err)
	}
	file, fileDiagnostics, err := ComposeRunInputs(manifest, "you.run", fileFactorySignature)
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
	_, namedCollisionDiagnostics, _ := ComposeRunInputs(manifest, "you.run", namedColliding)
	_, fileCollisionDiagnostics, _ := ComposeRunInputs(manifest, "you.run", fileColliding)
	if !reflect.DeepEqual(namedCollisionDiagnostics, fileCollisionDiagnostics) {
		t.Fatalf("equivalent selection diagnostics differ: named=%#v file=%#v", namedCollisionDiagnostics, fileCollisionDiagnostics)
	}
}

func TestComposeRunInputsRequiresStaticCommand(t *testing.T) {
	if _, _, err := ComposeRunInputs(Manifest{}, "you.run", work.InvocationSignatureConfig{}); err == nil {
		t.Fatal("ComposeRunInputs() error = nil, want missing static command failure")
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
