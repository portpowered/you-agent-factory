package completionprojection_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
)

func TestProjectFlagsPreservesEffectiveSchemaOrderAndAliases(t *testing.T) {
	schema := effectiveSchema(
		namedParameter("alpha", "first", []string{"a", "one"}),
		namedParameter("beta", "second", []string{"b"}),
	)

	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{Target: completionprojection.TargetFlags},
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}

	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		flagCandidate("alpha", "--first", "alpha description"),
		flagCandidate("alpha", "--a", "alpha description"),
		flagCandidate("alpha", "--one", "alpha description"),
		flagCandidate("beta", "--second", "beta description"),
		flagCandidate("beta", "--b", "beta description"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectFlagsDoesNotInventFlagsForPositionalOrStdinBindings(t *testing.T) {
	schema := effectiveSchema(
		climanifest.EffectiveFactoryParameter{
			BindingID:             "position",
			PreferredExternalName: "position",
			Aliases:               []string{"p"},
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind:     work.InvocationParameterBindingKindPositional,
				Position: 1,
			}},
		},
		climanifest.EffectiveFactoryParameter{
			BindingID:             "stdin",
			PreferredExternalName: "stdin",
			Aliases:               []string{"s"},
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind: work.InvocationParameterBindingKindStdin,
			}},
		},
		namedParameter("named", "query", []string{"q"}),
	)

	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{Target: completionprojection.TargetFlags},
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		flagCandidate("named", "--query", "named description"),
		flagCandidate("named", "--q", "named description"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectFlagsHasSelectionParityAndIsRepeatable(t *testing.T) {
	namedSelection := effectiveSchema(
		namedParameter("format", "format", []string{"f"}),
		namedParameter("query", "search", []string{"q", "find"}),
	)
	explicitFileSelection := cloneSchema(namedSelection)
	completionContext := completionprojection.Context{
		Target:        completionprojection.TargetFlags,
		EnteredPrefix: "--se",
	}

	named, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("named Project() error = %v", err)
	}
	fromFile, err := completionprojection.Project(context.Background(), explicitFileSelection, completionContext)
	if err != nil {
		t.Fatalf("explicit-file Project() error = %v", err)
	}
	again, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("repeated Project() error = %v", err)
	}
	if !reflect.DeepEqual(named, fromFile) || !reflect.DeepEqual(named, again) {
		t.Fatalf("equivalent projections differ: named=%#v file=%#v again=%#v", named, fromFile, again)
	}

	named.Candidates[0].Value = "--changed"
	if explicitFileSelection.FactoryParameters[0].PreferredExternalName != "format" {
		t.Fatal("projection shares mutable state with the effective schema")
	}
	repeatedAfterMutation, err := completionprojection.Project(context.Background(), namedSelection, completionContext)
	if err != nil {
		t.Fatalf("Project() after result mutation error = %v", err)
	}
	if !reflect.DeepEqual(repeatedAfterMutation, again) {
		t.Fatalf("result mutation changed repeated projection: got=%#v want=%#v", repeatedAfterMutation, again)
	}
}

func TestProjectValuesPreservesDeclaredChoiceOrder(t *testing.T) {
	schema := effectiveSchema(climanifest.EffectiveFactoryParameter{
		BindingID:   "format",
		Description: "output format",
		Choices:     []string{"json", "text", "markdown"},
	})

	got := projectValues(t, schema, "format")
	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		valueCandidate("format", "json", "output format"),
		valueCandidate("format", "text", "output format"),
		valueCandidate("format", "markdown", "output format"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectValuesUsesDocumentedBooleanForms(t *testing.T) {
	schema := effectiveSchema(climanifest.EffectiveFactoryParameter{
		BindingID:   "confirm",
		Description: "confirm execution",
		TypeHint:    work.InvocationParameterTypeHintBooleanString,
	})

	got := projectValues(t, schema, "confirm")
	want := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		valueCandidate("confirm", "true", "confirm execution"),
		valueCandidate("confirm", "false", "confirm execution"),
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectValuesRequestsFilesystemDelegationForFileInput(t *testing.T) {
	schema := effectiveSchema(climanifest.EffectiveFactoryParameter{
		BindingID:   "config",
		Description: "configuration file",
		TypeHint:    work.InvocationParameterTypeHintFilePath,
	})

	got := projectValues(t, schema, "config")
	want := completionprojection.Projection{Directives: []completionprojection.Directive{{
		Kind:               completionprojection.DirectiveKindFilesystemDelegation,
		ParameterBindingID: "config",
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Project() = %#v, want %#v", got, want)
	}
}

func TestProjectValuesInventsNoFreeTextCandidates(t *testing.T) {
	defaultValue := "write a release note"
	schema := effectiveSchema(climanifest.EffectiveFactoryParameter{
		BindingID:     "prompt",
		Description:   "request text",
		TypeHint:      work.InvocationParameterTypeHintString,
		DefaultValue:  &defaultValue,
		DefaultValues: nil,
	})

	got := projectValues(t, schema, "prompt")
	if !reflect.DeepEqual(got, completionprojection.Projection{}) {
		t.Fatalf("Project() = %#v, want empty projection", got)
	}
}

func TestProjectValuesAddsDefaultsOnlyToDescriptionMetadata(t *testing.T) {
	defaultValue := "json"
	schema := effectiveSchema(
		climanifest.EffectiveFactoryParameter{
			BindingID:    "format",
			Description:  "output format",
			Choices:      []string{"text", "json"},
			DefaultValue: &defaultValue,
		},
		climanifest.EffectiveFactoryParameter{
			BindingID:     "labels",
			Choices:       []string{"bug", "feature"},
			DefaultValues: []string{"bug", "feature"},
		},
	)

	scalar := projectValues(t, schema, "format")
	wantScalar := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		valueCandidate("format", "text", "output format Default: json."),
		valueCandidate("format", "json", "output format Default: json."),
	}}
	if !reflect.DeepEqual(scalar, wantScalar) {
		t.Fatalf("scalar Project() = %#v, want %#v", scalar, wantScalar)
	}

	repeated := projectValues(t, schema, "labels")
	wantRepeated := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		valueCandidate("labels", "bug", "Defaults: bug, feature."),
		valueCandidate("labels", "feature", "Defaults: bug, feature."),
	}}
	if !reflect.DeepEqual(repeated, wantRepeated) {
		t.Fatalf("repeated Project() = %#v, want %#v", repeated, wantRepeated)
	}
}

func TestProjectSensitiveParameterExposesOnlyItsAddressableFlag(t *testing.T) {
	const (
		sensitiveChoiceSentinel     = "sensitive-choice-sentinel"
		sensitiveDefaultSentinel    = "sensitive-default-sentinel"
		sensitivePrefixSentinel     = "sensitive-prefix-sentinel"
		sensitiveDiagnosticSentinel = "sensitive-diagnostic-sentinel"
	)
	defaultValue := sensitiveDefaultSentinel
	schema := effectiveSchema(climanifest.EffectiveFactoryParameter{
		BindingID:             "credential",
		PreferredExternalName: "credential",
		Aliases:               []string{"auth"},
		Description:           "credential input",
		Choices:               []string{sensitiveChoiceSentinel},
		DefaultValue:          &defaultValue,
		TypeHint:              work.InvocationParameterTypeHintFilePath,
		Sensitive:             true,
		Bindings: []work.InvocationParameterBindingConfig{{
			Kind: work.InvocationParameterBindingKindNamed,
		}},
	})

	flags, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{
			Target:        completionprojection.TargetFlags,
			EnteredPrefix: sensitivePrefixSentinel,
		},
	)
	if err != nil {
		t.Fatal("flag projection returned an error")
	}
	wantFlags := completionprojection.Projection{Candidates: []completionprojection.Candidate{
		flagCandidate("credential", "--credential", "credential input"),
		flagCandidate("credential", "--auth", "credential input"),
	}}
	if !reflect.DeepEqual(flags, wantFlags) {
		t.Fatal("sensitive parameter flag projection differs from addressable flag facts")
	}

	values, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{
			Target:             completionprojection.TargetValues,
			ParameterBindingID: "credential",
			EnteredPrefix:      sensitivePrefixSentinel,
		},
	)
	if err != nil {
		t.Fatal("value projection returned an error")
	}
	if !reflect.DeepEqual(values, completionprojection.Projection{}) {
		t.Fatal("sensitive parameter produced value or directive facts")
	}

	assertProjectionOmitsText(t, flags,
		sensitiveChoiceSentinel,
		sensitiveDefaultSentinel,
		sensitivePrefixSentinel,
		sensitiveDiagnosticSentinel,
	)
	assertProjectionOmitsText(t, values,
		sensitiveChoiceSentinel,
		sensitiveDefaultSentinel,
		sensitivePrefixSentinel,
		sensitiveDiagnosticSentinel,
	)
}

func TestProjectNoSignatureReturnsNoSignatureCompletionFacts(t *testing.T) {
	schema := climanifest.EffectiveInputSchema{
		CommandID:        "you.run",
		FactoryInputMode: climanifest.EffectiveFactoryInputModeCompatibility,
		StaticInputs: []climanifest.EffectiveStaticInput{{
			ID:              "you.run.arg.0",
			Kind:            "argument",
			ConsumesStdin:   true,
			PublicSpellings: []string{"input"},
		}},
	}

	for _, completionContext := range []completionprojection.Context{
		{Target: completionprojection.TargetFlags},
		{Target: completionprojection.TargetValues, ParameterBindingID: "input"},
	} {
		got, err := completionprojection.Project(context.Background(), schema, completionContext)
		if err != nil {
			t.Fatal("no-signature projection returned an error")
		}
		if !reflect.DeepEqual(got, completionprojection.Projection{}) {
			t.Fatal("no-signature Factory produced signature-only completion facts")
		}
	}
}

func TestProjectRejectsInvalidSchemaAtomicallyWithoutConfidentialErrorText(t *testing.T) {
	const (
		prefixSentinel     = "entered-prefix-sentinel"
		sensitiveSentinel  = "sensitive-value-sentinel"
		diagnosticSentinel = "sensitive-diagnostic-sentinel"
	)
	schema := effectiveSchema(
		namedParameter("valid", "valid", nil),
		climanifest.EffectiveFactoryParameter{
			BindingID:     "invalid",
			CanonicalName: "invalid",
			Sensitive:     true,
			Choices:       []string{sensitiveSentinel},
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind: diagnosticSentinel,
			}},
		},
	)

	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{
			Target:        completionprojection.TargetFlags,
			EnteredPrefix: prefixSentinel,
		},
	)
	if !errors.Is(err, completionprojection.ErrInvalidSchema) {
		t.Fatalf("Project() error = %v, want ErrInvalidSchema", err)
	}
	if !reflect.DeepEqual(got, completionprojection.Projection{}) {
		t.Fatalf("Project() = %#v, want atomic empty failure", got)
	}
	assertErrorOmitsText(t, err, prefixSentinel, sensitiveSentinel, diagnosticSentinel)
}

func TestProjectRejectsStaticDynamicAndDynamicAliasCollisionsAtomically(t *testing.T) {
	tests := map[string]climanifest.EffectiveInputSchema{
		"static dynamic": func() climanifest.EffectiveInputSchema {
			schema := effectiveSchema(
				namedParameter("first", "first", nil),
				namedParameter("second", "reserved", nil),
			)
			schema.StaticInputs = []climanifest.EffectiveStaticInput{{
				ID:              "you.run.flag.reserved",
				Kind:            "flag",
				PublicSpellings: []string{"reserved"},
			}}
			return schema
		}(),
		"dynamic alias": effectiveSchema(
			namedParameter("first", "first", []string{"shared"}),
			namedParameter("second", "second", []string{"shared"}),
		),
	}

	for name, schema := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := completionprojection.Project(
				context.Background(),
				schema,
				completionprojection.Context{Target: completionprojection.TargetFlags},
			)
			if !errors.Is(err, completionprojection.ErrSchemaCollision) {
				t.Fatalf("Project() error = %v, want ErrSchemaCollision", err)
			}
			if !reflect.DeepEqual(got, completionprojection.Projection{}) {
				t.Fatalf("Project() = %#v, want atomic empty failure", got)
			}
		})
	}
}

func TestProjectCancellationBeforeAndDuringProjectionIsAtomic(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	for name, ctx := range map[string]context.Context{
		"before": cancelled,
		"during": newCancelAfterErrContext(5),
	} {
		t.Run(name, func(t *testing.T) {
			schema := effectiveSchema(
				namedParameter("first", "first", nil),
				namedParameter("second", "second", nil),
			)
			got, err := completionprojection.Project(
				ctx,
				schema,
				completionprojection.Context{Target: completionprojection.TargetFlags},
			)
			if !errors.Is(err, completionprojection.ErrCancelled) ||
				!errors.Is(err, context.Canceled) {
				t.Fatalf("Project() error = %v, want documented cancellation error", err)
			}
			if !reflect.DeepEqual(got, completionprojection.Projection{}) {
				t.Fatalf("Project() = %#v, want atomic empty cancellation", got)
			}
		})
	}
}

func effectiveSchema(parameters ...climanifest.EffectiveFactoryParameter) climanifest.EffectiveInputSchema {
	for index := range parameters {
		if parameters[index].CanonicalName == "" {
			parameters[index].CanonicalName = parameters[index].BindingID
		}
	}
	return climanifest.EffectiveInputSchema{
		CommandID:                  "you.run",
		FactoryInputMode:           climanifest.EffectiveFactoryInputModeSignature,
		UnknownNamedArgumentPolicy: work.InvocationUnknownNamedArgumentPolicyReject,
		FactoryParameters:          parameters,
	}
}

func namedParameter(bindingID, externalName string, aliases []string) climanifest.EffectiveFactoryParameter {
	return climanifest.EffectiveFactoryParameter{
		BindingID:             bindingID,
		CanonicalName:         bindingID,
		PreferredExternalName: externalName,
		Aliases:               aliases,
		Description:           bindingID + " description",
		Bindings: []work.InvocationParameterBindingConfig{{
			Kind: work.InvocationParameterBindingKindNamed,
		}},
	}
}

func flagCandidate(bindingID, value, description string) completionprojection.Candidate {
	return completionprojection.Candidate{
		Kind:               completionprojection.CandidateKindFlag,
		ParameterBindingID: bindingID,
		Value:              value,
		Description:        description,
	}
}

func valueCandidate(bindingID, value, description string) completionprojection.Candidate {
	return completionprojection.Candidate{
		Kind:               completionprojection.CandidateKindValue,
		ParameterBindingID: bindingID,
		Value:              value,
		Description:        description,
	}
}

func projectValues(
	t *testing.T,
	schema climanifest.EffectiveInputSchema,
	bindingID string,
) completionprojection.Projection {
	t.Helper()
	got, err := completionprojection.Project(
		context.Background(),
		schema,
		completionprojection.Context{
			Target:             completionprojection.TargetValues,
			ParameterBindingID: bindingID,
		},
	)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	return got
}

func cloneSchema(schema climanifest.EffectiveInputSchema) climanifest.EffectiveInputSchema {
	cloned := schema
	cloned.FactoryParameters = make([]climanifest.EffectiveFactoryParameter, len(schema.FactoryParameters))
	for index, parameter := range schema.FactoryParameters {
		cloned.FactoryParameters[index] = parameter
		cloned.FactoryParameters[index].Aliases = append([]string(nil), parameter.Aliases...)
		cloned.FactoryParameters[index].Bindings = append(
			[]work.InvocationParameterBindingConfig(nil),
			parameter.Bindings...,
		)
	}
	return cloned
}

func assertProjectionOmitsText(
	t *testing.T,
	projection completionprojection.Projection,
	omitted ...string,
) {
	t.Helper()
	for _, candidate := range projection.Candidates {
		for _, text := range omitted {
			if strings.Contains(candidate.Value, text) ||
				strings.Contains(candidate.Description, text) ||
				strings.Contains(candidate.ParameterBindingID, text) {
				t.Fatal("projection retained confidential candidate text")
			}
		}
	}
	for _, directive := range projection.Directives {
		for _, text := range omitted {
			if strings.Contains(directive.Kind, text) ||
				strings.Contains(directive.ParameterBindingID, text) {
				t.Fatal("projection retained confidential directive text")
			}
		}
	}
}

func assertErrorOmitsText(t *testing.T, err error, omitted ...string) {
	t.Helper()
	for _, text := range omitted {
		if strings.Contains(err.Error(), text) {
			t.Fatal("projection error retained confidential input text")
		}
	}
}

type cancelAfterErrContext struct {
	remainingChecks int
	done            chan struct{}
	cancelled       bool
}

func newCancelAfterErrContext(remainingChecks int) *cancelAfterErrContext {
	return &cancelAfterErrContext{
		remainingChecks: remainingChecks,
		done:            make(chan struct{}),
	}
}

func (ctx *cancelAfterErrContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (ctx *cancelAfterErrContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *cancelAfterErrContext) Err() error {
	if ctx.cancelled {
		return context.Canceled
	}
	ctx.remainingChecks--
	if ctx.remainingChecks <= 0 {
		close(ctx.done)
		ctx.cancelled = true
		return context.Canceled
	}
	return nil
}

func (ctx *cancelAfterErrContext) Value(any) any {
	return nil
}
