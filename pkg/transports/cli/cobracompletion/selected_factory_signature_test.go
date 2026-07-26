package cobracompletion_test

import (
	"context"
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cobracompletion"
	"github.com/spf13/cobra"
)

func TestSelectedFactorySignatureProjectsFlagsAliasesAndTypedValues(t *testing.T) {
	defaultFormat := "json"
	operation := selectedFactorySignatureOperation(t, &factorydefinitions.InvocationSignatureConfig{
		Parameters: []work.InvocationParameterConfig{
			{
				Name:         "format",
				ExternalName: "output-format",
				Aliases:      []string{"f"},
				Description:  "output format",
				Choices:      []string{"text", "json"},
				DefaultValue: defaultFormat,
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind: work.InvocationParameterBindingKindNamed,
				}},
			},
			{
				Name:        "confirm",
				Description: "confirm execution",
				TypeHint:    work.InvocationParameterTypeHintBooleanString,
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind: work.InvocationParameterBindingKindNamed,
				}},
			},
			{
				Name:    "positional",
				Aliases: []string{"p"},
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind:     work.InvocationParameterBindingKindPositional,
					Position: 1,
				}},
			},
		},
	})

	flags := operation(t.Context(), cobracompletion.SelectedFactorySignatureRequest{
		ProjectRoot:   "project",
		GlobalRoot:    "global",
		FactoryName:   "alpha",
		Target:        "flags",
		EnteredPrefix: "--",
	})
	wantFlags := []cobra.Completion{
		cobra.CompletionWithDesc("--confirm", "confirm execution"),
		cobra.CompletionWithDesc("--output-format", "output format"),
		cobra.CompletionWithDesc("--f", "output format"),
	}
	if !reflect.DeepEqual(flags.Completions, wantFlags) ||
		flags.Directive != cobra.ShellCompDirectiveNoFileComp ||
		flags.UseFallback {
		t.Fatalf("flag completion = %#v, want %#v", flags, wantFlags)
	}

	values := operation(t.Context(), cobracompletion.SelectedFactorySignatureRequest{
		ProjectRoot:       "project",
		GlobalRoot:        "global",
		FactoryName:       "alpha",
		Target:            "values",
		ParameterSpelling: "f",
		CompletionPrefix:  "--f=",
		EnteredPrefix:     "j",
	})
	wantValues := []cobra.Completion{
		cobra.CompletionWithDesc("--f=json", "output format Default: json."),
	}
	if !reflect.DeepEqual(values.Completions, wantValues) ||
		values.Directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("value completion = %#v, want %#v", values, wantValues)
	}

	boolean := operation(t.Context(), cobracompletion.SelectedFactorySignatureRequest{
		ProjectRoot:       "project",
		GlobalRoot:        "global",
		FactoryName:       "alpha",
		Target:            "values",
		ParameterSpelling: "confirm",
		EnteredPrefix:     "f",
	})
	if !reflect.DeepEqual(boolean.Completions, []cobra.Completion{
		cobra.CompletionWithDesc("false", "confirm execution"),
	}) {
		t.Fatalf("boolean completion = %#v", boolean.Completions)
	}
}

func TestSelectedFactorySignatureMapsFilesystemAndSuppressesInventedValues(t *testing.T) {
	operation := selectedFactorySignatureOperation(t, &factorydefinitions.InvocationSignatureConfig{
		Parameters: []work.InvocationParameterConfig{
			{
				Name:     "config",
				TypeHint: work.InvocationParameterTypeHintFilePath,
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind: work.InvocationParameterBindingKindNamed,
				}},
			},
			{
				Name:         "prompt",
				DefaultValue: "must-not-complete",
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind: work.InvocationParameterBindingKindNamed,
				}},
			},
			{
				Name:         "token",
				Sensitive:    true,
				Choices:      []string{"must-not-complete"},
				DefaultValue: "must-not-complete",
				Bindings: []work.InvocationParameterBindingConfig{{
					Kind: work.InvocationParameterBindingKindNamed,
				}},
			},
		},
	})

	file := operation(t.Context(), valueRequest("config"))
	if len(file.Completions) != 0 || file.Directive != cobra.ShellCompDirectiveDefault {
		t.Fatalf("file completion = %#v, want filesystem delegation", file)
	}
	for _, spelling := range []string{"prompt", "token"} {
		result := operation(t.Context(), valueRequest(spelling))
		if len(result.Completions) != 0 ||
			result.Directive != cobra.ShellCompDirectiveNoFileComp ||
			result.UseFallback {
			t.Fatalf("%s completion = %#v, want handled empty result", spelling, result)
		}
	}
}

func TestSelectedFactorySignaturePreservesFallbackAndReturnsAtomicFailure(t *testing.T) {
	noSignature := selectedFactorySignatureOperation(t, nil)
	result := noSignature(t.Context(), valueRequest("format"))
	if !result.UseFallback {
		t.Fatalf("no-signature result = %#v, want static fallback", result)
	}

	invalid := selectedFactorySignatureOperation(t, &factorydefinitions.InvocationSignatureConfig{
		Parameters: []work.InvocationParameterConfig{{
			Name: "named",
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind: work.InvocationParameterBindingKindNamed,
			}},
		}},
	})
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	failed := invalid(cancelled, cobracompletion.SelectedFactorySignatureRequest{
		ProjectRoot: "project",
		GlobalRoot:  "global",
		FactoryName: "alpha",
		Target:      "flags",
	})
	wantDirective := cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	if len(failed.Completions) != 0 || failed.Directive != wantDirective ||
		failed.UseFallback {
		t.Fatalf("cancelled completion = %#v, want atomic failure", failed)
	}
}

func TestSelectedFactorySignatureFailuresAreAtomicAndCancellationSkipsDiscovery(t *testing.T) {
	calls := 0
	manifest := selectedFactoryRunManifest()
	complete := cobracompletion.NewSelectedFactorySignature(
		func(
			ctx context.Context,
			_ factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			calls++
			switch calls {
			case 1:
				return factorydefinitions.ListEffectiveFactoriesResult{}, context.Canceled
			case 2:
				cancelContext(t, ctx)
				return factorydefinitions.ListEffectiveFactoriesResult{
					Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
						Name: "alpha",
					}},
				}, nil
			case 3:
				return factorydefinitions.ListEffectiveFactoriesResult{}, nil
			default:
				return factorydefinitions.ListEffectiveFactoriesResult{
					Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
						Name: "alpha",
						InvocationSignature: &factorydefinitions.InvocationSignatureConfig{
							Parameters: []work.InvocationParameterConfig{
								{
									Name: "secret-diagnostic-must-not-escape",
									Bindings: []work.InvocationParameterBindingConfig{{
										Kind: work.InvocationParameterBindingKindNamed,
									}},
								},
								{
									Name: "secret-diagnostic-must-not-escape",
									Bindings: []work.InvocationParameterBindingConfig{{
										Kind: work.InvocationParameterBindingKindNamed,
									}},
								},
							},
						},
					}},
				}, nil
			}
		},
		manifest,
	)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	contexts := []context.Context{
		cancelled,
		t.Context(),
		withCancelHook(t.Context()),
		t.Context(),
		t.Context(),
	}
	for index, ctx := range contexts {
		result := complete(ctx, cobracompletion.SelectedFactorySignatureRequest{
			FactoryName: "alpha",
			Target:      "flags",
		})
		assertAtomicSignatureFailure(t, index, result)
	}
	if calls != 4 {
		t.Fatalf("catalog calls = %d, want pre-cancellation to skip discovery", calls)
	}
}

func TestSelectedFactorySignatureIsRepeatableAndDoesNotMutateSignature(t *testing.T) {
	defaultValue := "json"
	signature := &factorydefinitions.InvocationSignatureConfig{
		Parameters: []work.InvocationParameterConfig{{
			Name:         "format",
			Choices:      []string{"text", "json"},
			DefaultValue: defaultValue,
			Bindings: []work.InvocationParameterBindingConfig{{
				Kind: work.InvocationParameterBindingKindNamed,
			}},
		}},
	}
	before := *signature
	before.Parameters = append([]work.InvocationParameterConfig(nil), signature.Parameters...)
	before.Parameters[0].Choices = append([]string(nil), signature.Parameters[0].Choices...)
	before.Parameters[0].Bindings = append(
		[]work.InvocationParameterBindingConfig(nil),
		signature.Parameters[0].Bindings...,
	)
	complete := selectedFactorySignatureOperation(t, signature)
	request := valueRequest("format")

	first := complete(t.Context(), request)
	again := complete(t.Context(), request)
	if !reflect.DeepEqual(first, again) {
		t.Fatalf("repeated completion differs: first=%#v again=%#v", first, again)
	}
	if !reflect.DeepEqual(*signature, before) {
		t.Fatalf("completion mutated signature: got=%#v want=%#v", *signature, before)
	}
}

func assertAtomicSignatureFailure(
	t *testing.T,
	index int,
	result cobracompletion.SelectedFactorySignatureResult,
) {
	t.Helper()
	wantDirective := cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
	if len(result.Completions) != 0 || result.Directive != wantDirective ||
		result.UseFallback {
		t.Fatalf("failure %d = %#v, want atomic sensitive-safe failure", index, result)
	}
}

func withCancelHook(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	return context.WithValue(ctx, cancelContextKey{}, context.CancelFunc(cancel))
}

func TestRegisterSelectedFactorySignatureBridgesDisabledFlagParsing(t *testing.T) {
	run := &cobra.Command{Use: "run", DisableFlagParsing: true}
	run.Flags().String("named", "", "")
	run.ValidArgsFunction = func(
		*cobra.Command,
		[]string,
		string,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		return []cobra.Completion{"static"}, cobra.ShellCompDirectiveDefault
	}
	var requests []cobracompletion.SelectedFactorySignatureRequest
	err := cobracompletion.RegisterSelectedFactorySignature(
		run,
		func(
			_ context.Context,
			request cobracompletion.SelectedFactorySignatureRequest,
		) cobracompletion.SelectedFactorySignatureResult {
			requests = append(requests, request)
			return cobracompletion.SelectedFactorySignatureResult{
				Completions: []cobra.Completion{"dynamic"},
				Directive:   cobra.ShellCompDirectiveNoFileComp,
			}
		},
		func(*cobra.Command, string) (cobracompletion.FactoryNamesRequest, bool) {
			return cobracompletion.FactoryNamesRequest{
				ProjectRoot: "project",
				GlobalRoot:  "global",
			}, true
		},
	)
	if err != nil {
		t.Fatalf("RegisterSelectedFactorySignature() error = %v", err)
	}

	got, directive := run.ValidArgsFunction(
		run,
		[]string{"--named", "alpha"},
		"--out",
	)
	if !reflect.DeepEqual(got, []cobra.Completion{"dynamic"}) ||
		directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("flag bridge = (%#v, %v)", got, directive)
	}
	if len(requests) != 1 ||
		requests[0].FactoryName != "alpha" ||
		requests[0].Target != "flags" ||
		requests[0].EnteredPrefix != "--out" {
		t.Fatalf("flag request = %#v", requests)
	}

	got, directive = run.ValidArgsFunction(
		run,
		[]string{"--named=alpha"},
		"--output-format=j",
	)
	if !reflect.DeepEqual(got, []cobra.Completion{"dynamic"}) ||
		directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("value bridge = (%#v, %v)", got, directive)
	}
	if len(requests) != 2 ||
		requests[1].ParameterSpelling != "output-format" ||
		requests[1].CompletionPrefix != "--output-format=" ||
		requests[1].EnteredPrefix != "j" {
		t.Fatalf("value request = %#v", requests)
	}

	got, directive = run.ValidArgsFunction(run, nil, "")
	assertCompletionResult(t, "fallback", got, directive, []cobra.Completion{"static"}, cobra.ShellCompDirectiveDefault)
}

func selectedFactorySignatureOperation(
	t *testing.T,
	signature *factorydefinitions.InvocationSignatureConfig,
) cobracompletion.SelectedFactorySignatureOperation {
	t.Helper()
	manifest := selectedFactoryRunManifest()
	return cobracompletion.NewSelectedFactorySignature(
		func(
			context.Context,
			factorydefinitions.ListEffectiveFactoriesRequest,
		) (factorydefinitions.ListEffectiveFactoriesResult, error) {
			return factorydefinitions.ListEffectiveFactoriesResult{
				Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{
					Name:                "alpha",
					InvocationSignature: signature,
				}},
			}, nil
		},
		manifest,
	)
}

func selectedFactoryRunManifest() climanifest.Manifest {
	return climanifest.Manifest{
		Commands: map[string]climanifest.Command{
			"you.run": {
				ID:    "you.run",
				Name:  "run",
				Flags: map[string]climanifest.Flag{},
			},
		},
	}
}

func valueRequest(spelling string) cobracompletion.SelectedFactorySignatureRequest {
	return cobracompletion.SelectedFactorySignatureRequest{
		ProjectRoot:       "project",
		GlobalRoot:        "global",
		FactoryName:       "alpha",
		Target:            "values",
		ParameterSpelling: spelling,
	}
}

func assertCompletionResult(
	t *testing.T,
	label string,
	got []cobra.Completion,
	directive cobra.ShellCompDirective,
	want []cobra.Completion,
	wantDirective cobra.ShellCompDirective,
) {
	t.Helper()
	if !reflect.DeepEqual(got, want) || directive != wantDirective {
		t.Fatalf("%s = (%#v, %v), want (%#v, %v)", label, got, directive, want, wantDirective)
	}
}
