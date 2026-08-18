// Package cobracompletion adapts detached CLI completion projections to Cobra.
package cobracompletion

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/completionprojection"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	"github.com/spf13/cobra"
)

const SelectedFactoryFlagName = "named"

// FactoryNamesRequest identifies one effective catalog and the prefix entered
// by the shell.
type FactoryNamesRequest struct {
	ProjectRoot   string
	GlobalRoot    string
	EnteredPrefix string
}

// FactoryNamesOperation returns safe, atomic Cobra completion results.
type FactoryNamesOperation func(
	context.Context,
	FactoryNamesRequest,
) ([]cobra.Completion, cobra.ShellCompDirective)

// FactoryNamesRequestResolver derives catalog roots from invocation-local
// process edges. False requests a sensitive-safe atomic failure.
type FactoryNamesRequestResolver func(
	*cobra.Command,
	string,
) (FactoryNamesRequest, bool)

// NewFactoryNames binds the Factory Definitions catalog owner to the detached
// selected-Factory projection.
func NewFactoryNames(
	catalog factorydefinitions.EffectiveFactoryCatalogOperation,
) FactoryNamesOperation {
	return func(
		ctx context.Context,
		request FactoryNamesRequest,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		if completionCancelled(ctx) || catalog == nil {
			return factoryNameFailure()
		}
		result, err := catalog(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
			ProjectRoot: request.ProjectRoot,
			GlobalRoot:  request.GlobalRoot,
		})
		if err != nil || completionCancelled(ctx) {
			return factoryNameFailure()
		}
		projection, err := completionprojection.ProjectFactoryNames(ctx, result)
		if err != nil {
			return factoryNameFailure()
		}

		completions := make([]cobra.Completion, 0, len(projection.Candidates))
		for _, candidate := range projection.Candidates {
			if !strings.HasPrefix(candidate.Value, request.EnteredPrefix) {
				continue
			}
			if candidate.Description == "" {
				completions = append(completions, candidate.Value)
				continue
			}
			completions = append(
				completions,
				cobra.CompletionWithDesc(candidate.Value, candidate.Description),
			)
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	}
}

// RegisterFactoryNames attaches the existing selected-Factory flag callback.
// Run commands that own flag parsing receive the equivalent argument callback
// required by Cobra's completion protocol.
func RegisterFactoryNames(
	run *cobra.Command,
	complete FactoryNamesOperation,
	resolve FactoryNamesRequestResolver,
) error {
	if run == nil || run.Flags().Lookup(SelectedFactoryFlagName) == nil {
		return fmt.Errorf("register selected-Factory completion: canonical --named flag is required")
	}
	callback := func(
		cmd *cobra.Command,
		_ []string,
		toComplete string,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		return completeResolvedFactoryNames(cmd, toComplete, complete, resolve)
	}
	if err := run.RegisterFlagCompletionFunc(SelectedFactoryFlagName, callback); err != nil {
		return err
	}

	fallback := run.ValidArgsFunction
	run.ValidArgsFunction = func(
		cmd *cobra.Command,
		args []string,
		toComplete string,
	) ([]cobra.Completion, cobra.ShellCompDirective) {
		prefix, completionPrefix, selected := factoryNameValueRequest(args, toComplete)
		if selected {
			completions, directive := completeResolvedFactoryNames(
				cmd,
				prefix,
				complete,
				resolve,
			)
			for index := range completions {
				completions[index] = completionPrefix + completions[index]
			}
			return completions, directive
		}
		if fallback != nil {
			return fallback(cmd, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveDefault
	}
	return nil
}

func completeResolvedFactoryNames(
	cmd *cobra.Command,
	prefix string,
	complete FactoryNamesOperation,
	resolve FactoryNamesRequestResolver,
) ([]cobra.Completion, cobra.ShellCompDirective) {
	if cmd == nil || complete == nil || resolve == nil {
		return factoryNameFailure()
	}
	request, ok := resolve(cmd, prefix)
	if !ok {
		return factoryNameFailure()
	}
	return complete(cmd.Context(), request)
}

func factoryNameValueRequest(
	args []string,
	toComplete string,
) (prefix string, completionPrefix string, selected bool) {
	if _, _, terminated := runcli.SplitFlagTerminator(args); terminated {
		return "", "", false
	}
	longPrefix := "--" + SelectedFactoryFlagName + "="
	if strings.HasPrefix(toComplete, longPrefix) {
		return strings.TrimPrefix(toComplete, longPrefix), longPrefix, true
	}
	if len(args) > 0 && args[len(args)-1] == "--"+SelectedFactoryFlagName {
		return toComplete, "", true
	}
	return "", "", false
}

func factoryNameFailure() ([]cobra.Completion, cobra.ShellCompDirective) {
	return nil, factoryNameFailureDirective()
}

func completionCancelled(ctx context.Context) bool {
	return ctx == nil || ctx.Err() != nil
}
