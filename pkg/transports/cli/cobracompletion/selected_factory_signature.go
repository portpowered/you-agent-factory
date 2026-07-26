package cobracompletion

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
	"github.com/spf13/cobra"
)

const (
	signatureTargetFlags  = "flags"
	signatureTargetValues = "values"
)

// SelectedFactorySignatureRequest identifies one selected effective Factory
// and the completion target requested by Cobra.
type SelectedFactorySignatureRequest struct {
	ProjectRoot       string
	GlobalRoot        string
	FactoryName       string
	Target            string
	ParameterSpelling string
	CompletionPrefix  string
	EnteredPrefix     string
}

// SelectedFactorySignatureResult is atomic. UseFallback preserves Cobra's
// existing static behavior when the Factory has no signature or a value target
// is not owned by the selected signature.
type SelectedFactorySignatureResult struct {
	Completions []cobra.Completion
	Directive   cobra.ShellCompDirective
	UseFallback bool
}

// SelectedFactorySignatureOperation resolves completion from the production
// effective Factory catalog and normalized run-input schema.
type SelectedFactorySignatureOperation func(
	context.Context,
	SelectedFactorySignatureRequest,
) SelectedFactorySignatureResult

// NewSelectedFactorySignature binds the Factory Definitions owner to the
// detached selected-Factory signature projection.
func NewSelectedFactorySignature(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
	manifest climanifest.Manifest,
) SelectedFactorySignatureOperation {
	return func(
		ctx context.Context,
		request SelectedFactorySignatureRequest,
	) SelectedFactorySignatureResult {
		if completionCancelled(ctx) || catalog == nil {
			return selectedFactorySignatureFailure()
		}
		result, err := catalog(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: request.ProjectRoot,
			GlobalRoot:  request.GlobalRoot,
		})
		if err != nil || completionCancelled(ctx) {
			return selectedFactorySignatureFailure()
		}
		entry, found := selectedFactoryEntry(result.Entries, request.FactoryName)
		if !found {
			return selectedFactorySignatureFailure()
		}
		schema, diagnostics, err := climanifest.ComposeRunInputs(
			manifest,
			"you.run",
			entry.InvocationSignature,
		)
		if err != nil || len(diagnostics) != 0 || completionCancelled(ctx) {
			return selectedFactorySignatureFailure()
		}
		if schema.FactoryInputMode != climanifest.EffectiveFactoryInputModeSignature {
			return SelectedFactorySignatureResult{UseFallback: true}
		}

		completionContext, recognized := signatureProjectionContext(schema, request)
		if !recognized {
			return SelectedFactorySignatureResult{UseFallback: true}
		}
		projection, err := completionprojection.Project(ctx, schema, completionContext)
		if err != nil {
			return selectedFactorySignatureFailure()
		}
		return projectSelectedFactorySignature(projection, request)
	}
}

// RegisterSelectedFactorySignature bridges dynamic selected-Factory inputs
// through ValidArgsFunction because the canonical run command disables Cobra
// flag parsing.
func RegisterSelectedFactorySignature(
	run *cobra.Command,
	complete SelectedFactorySignatureOperation,
	resolve FactoryNamesRequestResolver,
) error {
	if run == nil || run.Flags().Lookup(SelectedFactoryFlagName) == nil {
		return fmt.Errorf("register selected-Factory signature completion: canonical --named flag is required")
	}
	fallback := run.ValidArgsFunction
	run.ValidArgsFunction = func(
		cmd *cobra.Command,
		args []string,
		toComplete string,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		request, requested := selectedFactorySignatureRequest(args, toComplete)
		if !requested {
			return callCompletionFallback(fallback, cmd, args, toComplete)
		}
		roots, ok := resolveSelectedFactoryRoots(cmd, resolve)
		if !ok || complete == nil {
			return factoryNameFailure()
		}
		request.ProjectRoot = roots.ProjectRoot
		request.GlobalRoot = roots.GlobalRoot
		result := complete(cmd.Context(), request)
		if result.UseFallback {
			return callCompletionFallback(fallback, cmd, args, toComplete)
		}
		return result.Completions, result.Directive
	}
	return nil
}

func selectedFactoryEntry(
	entries []factorydefinitions.EffectiveFactoryCatalogEntry,
	name string,
) (factorydefinitions.EffectiveFactoryCatalogEntry, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return factorydefinitions.EffectiveFactoryCatalogEntry{}, false
}

func signatureProjectionContext(
	schema climanifest.EffectiveInputSchema,
	request SelectedFactorySignatureRequest,
) (completionprojection.Context, bool) {
	if request.Target == signatureTargetFlags {
		return completionprojection.Context{
			Target:        completionprojection.TargetFlags,
			EnteredPrefix: request.EnteredPrefix,
		}, true
	}
	for _, parameter := range schema.FactoryParameters {
		if parameter.PreferredExternalName == request.ParameterSpelling ||
			containsString(parameter.Aliases, request.ParameterSpelling) {
			return completionprojection.Context{
				Target:             completionprojection.TargetValues,
				ParameterBindingID: parameter.BindingID,
				EnteredPrefix:      request.EnteredPrefix,
			}, true
		}
	}
	return completionprojection.Context{}, false
}

func projectSelectedFactorySignature(
	projection completionprojection.Projection,
	request SelectedFactorySignatureRequest,
) SelectedFactorySignatureResult {
	completions := make([]cobra.Completion, 0, len(projection.Candidates))
	for _, candidate := range projection.Candidates {
		if !strings.HasPrefix(candidate.Value, request.EnteredPrefix) {
			continue
		}
		value := candidate.Value
		if request.Target == signatureTargetValues {
			value = request.CompletionPrefix + value
		}
		if candidate.Description != "" {
			value = cobra.CompletionWithDesc(value, candidate.Description)
		}
		completions = append(completions, value)
	}
	directive := cobra.ShellCompDirectiveNoFileComp
	for _, directiveFact := range projection.Directives {
		if directiveFact.Kind == completionprojection.DirectiveKindFilesystemDelegation {
			directive = cobra.ShellCompDirectiveDefault
		}
	}
	return SelectedFactorySignatureResult{
		Completions: completions,
		Directive:   directive,
	}
}

func selectedFactorySignatureRequest(
	args []string,
	toComplete string,
) (SelectedFactorySignatureRequest, bool) {
	factoryName := selectedFactoryName(args)
	if factoryName == "" {
		return SelectedFactorySignatureRequest{}, false
	}
	if spelling, prefix, ok := inlineSignatureValueRequest(toComplete); ok {
		return SelectedFactorySignatureRequest{
			FactoryName:       factoryName,
			Target:            signatureTargetValues,
			ParameterSpelling: spelling,
			CompletionPrefix:  "--" + spelling + "=",
			EnteredPrefix:     prefix,
		}, true
	}
	if len(args) > 0 {
		if spelling, ok := longFlagSpelling(args[len(args)-1]); ok &&
			spelling != SelectedFactoryFlagName {
			return SelectedFactorySignatureRequest{
				FactoryName:       factoryName,
				Target:            signatureTargetValues,
				ParameterSpelling: spelling,
				EnteredPrefix:     toComplete,
			}, true
		}
	}
	if strings.HasPrefix(toComplete, "--") {
		return SelectedFactorySignatureRequest{
			FactoryName:   factoryName,
			Target:        signatureTargetFlags,
			EnteredPrefix: toComplete,
		}, true
	}
	return SelectedFactorySignatureRequest{}, false
}

func selectedFactoryName(args []string) string {
	name := ""
	for index := 0; index < len(args); index++ {
		token := args[index]
		if strings.HasPrefix(token, "--"+SelectedFactoryFlagName+"=") {
			name = strings.TrimPrefix(token, "--"+SelectedFactoryFlagName+"=")
			continue
		}
		if token == "--"+SelectedFactoryFlagName && index+1 < len(args) {
			name = args[index+1]
			index++
		}
	}
	return strings.TrimSpace(name)
}

func inlineSignatureValueRequest(value string) (string, string, bool) {
	flag, entered, ok := strings.Cut(value, "=")
	if !ok {
		return "", "", false
	}
	spelling, ok := longFlagSpelling(flag)
	if !ok || spelling == SelectedFactoryFlagName {
		return "", "", false
	}
	return spelling, entered, true
}

func longFlagSpelling(value string) (string, bool) {
	if !strings.HasPrefix(value, "--") || len(value) == 2 {
		return "", false
	}
	return strings.TrimPrefix(value, "--"), true
}

func resolveSelectedFactoryRoots(
	cmd *cobra.Command,
	resolve FactoryNamesRequestResolver,
) (FactoryNamesRequest, bool) {
	if cmd == nil || resolve == nil {
		return FactoryNamesRequest{}, false
	}
	return resolve(cmd, "")
}

func callCompletionFallback(
	fallback cobra.CompletionFunc,
	cmd *cobra.Command,
	args []string,
	toComplete string,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	if fallback == nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	return fallback(cmd, args, toComplete)
}

func selectedFactorySignatureFailure() SelectedFactorySignatureResult {
	return SelectedFactorySignatureResult{
		Directive: factoryNameFailureDirective(),
	}
}

func factoryNameFailureDirective() cobra.ShellCompDirective {
	return cobra.ShellCompDirectiveError | cobra.ShellCompDirectiveNoFileComp
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
