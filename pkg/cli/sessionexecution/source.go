package sessionexecution

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/invocations"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

type sourceSelector struct {
	kind  workflowsource.Kind
	label string
}

func resolveExecutionSource(cfg StartConfig) (factorysessionexecution.Source, error) {
	selectors := collectExplicitSourceSelectors(cfg)
	if len(selectors) > 1 {
		labels := make([]string, 0, len(selectors))
		for _, selector := range selectors {
			labels = append(labels, selector.label)
		}
		return factorysessionexecution.Source{}, newExecutionError(
			ErrorCodeSourceConflict,
			fmt.Sprintf("session execution source selectors conflict: %s", strings.Join(labels, ", ")),
			"source",
		)
	}
	if len(selectors) == 1 {
		return sourceFromSelector(cfg, selectors[0])
	}
	return resolveImplicitExecutionSource(cfg)
}

func collectExplicitSourceSelectors(cfg StartConfig) []sourceSelector {
	selectors := make([]sourceSelector, 0, 3)
	if cfg.FactoryID != "" {
		selectors = append(selectors, sourceSelector{
			kind:  workflowsource.KindFactoryID,
			label: "--factory",
		})
	}
	if cfg.WorkflowName != "" {
		selectors = append(selectors, sourceSelector{
			kind:  workflowsource.KindWorkflowName,
			label: "--workflow",
		})
	}
	if cfg.WorkflowFile != "" {
		selectors = append(selectors, sourceSelector{
			kind:  workflowsource.KindWorkflowFile,
			label: "--workflow-file",
		})
	}
	return selectors
}

func sourceFromSelector(cfg StartConfig, selector sourceSelector) (factorysessionexecution.Source, error) {
	switch selector.kind {
	case workflowsource.KindFactoryID:
		return factorysessionexecution.Source{
			Kind:      selector.kind,
			FactoryID: strings.TrimSpace(cfg.FactoryID),
		}, nil
	case workflowsource.KindWorkflowName:
		return factorysessionexecution.Source{
			Kind:         selector.kind,
			WorkflowName: strings.TrimSpace(cfg.WorkflowName),
		}, nil
	case workflowsource.KindWorkflowFile:
		return factorysessionexecution.Source{
			Kind:         selector.kind,
			WorkflowFile: strings.TrimSpace(cfg.WorkflowFile),
		}, nil
	default:
		return factorysessionexecution.Source{}, newExecutionError(
			ErrorCodeMissingSource,
			"session execution source is required",
			"source",
		)
	}
}

func resolveImplicitExecutionSource(cfg StartConfig) (factorysessionexecution.Source, error) {
	switch len(cfg.PositionalArgs) {
	case 0:
		inlineSource, err := resolveInlineWorkflowText(cfg)
		if err != nil {
			return factorysessionexecution.Source{}, err
		}
		if inlineSource == "" {
			return factorysessionexecution.Source{}, newExecutionError(
				ErrorCodeMissingSource,
				"session execution source is required: provide --factory, --workflow, --workflow-file, inline workflow text, or a workflow file path",
				"source",
			)
		}
		return inlineWorkflowSource(inlineSource), nil
	case 1:
		arg := strings.TrimSpace(cfg.PositionalArgs[0])
		if arg == "-" {
			inlineSource, err := resolveInlineWorkflowText(cfg)
			if err != nil {
				return factorysessionexecution.Source{}, err
			}
			return inlineWorkflowSource(inlineSource), nil
		}
		if _, err := os.Stat(arg); err == nil {
			return factorysessionexecution.Source{
				Kind:         workflowsource.KindWorkflowFile,
				WorkflowFile: arg,
			}, nil
		}
		inlineSource, err := resolveInlineWorkflowText(cfg)
		if err != nil {
			return factorysessionexecution.Source{}, err
		}
		return inlineWorkflowSource(inlineSource), nil
	default:
		inlineSource, err := resolveInlineWorkflowText(cfg)
		if err != nil {
			return factorysessionexecution.Source{}, err
		}
		return inlineWorkflowSource(inlineSource), nil
	}
}

func inlineWorkflowSource(inlineSource string) factorysessionexecution.Source {
	return factorysessionexecution.Source{
		Kind: workflowsource.KindInlineWorkflow,
		InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
			InlineSource: inlineSource,
		},
	}
}

func resolveInlineWorkflowText(cfg StartConfig) (string, error) {
	positionalPrompt, explicitStdin, hasPositional := splitInlineWorkflowArgs(cfg.PositionalArgs)
	sources := invocations.TextInputSources{}
	if hasPositional {
		sources.PositionalText = &positionalPrompt
	}

	stdinPayload, hasStdin, err := readInlineWorkflowStdin(cfg, explicitStdin)
	if err != nil {
		return "", err
	}
	if hasStdin {
		sources.StdinText = &stdinPayload
	}
	if sources.PositionalText == nil && sources.StdinText == nil {
		return "", nil
	}

	resolved, err := invocations.ResolveTextInput(sources)
	if err != nil {
		var inputErr *invocations.InputError
		if errors.As(err, &inputErr) && inputErr.Code == invocations.InputErrorCodeSourceConflict {
			return "", newExecutionError(ErrorCodeSourceConflict, inputErr.Message, "source")
		}
		return "", err
	}
	return resolved.Text, nil
}

func splitInlineWorkflowArgs(args []string) (prompt string, explicitStdin bool, hasPositional bool) {
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "-" {
			explicitStdin = true
			continue
		}
		hasPositional = true
		positional = append(positional, arg)
	}
	return strings.Join(positional, " "), explicitStdin, hasPositional
}

func readInlineWorkflowStdin(cfg StartConfig, explicitStdin bool) (string, bool, error) {
	if !explicitStdin && executionStdinIsTTY(cfg) {
		return "", false, nil
	}
	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", false, fmt.Errorf("read session execution stdin: %w", err)
	}
	payload := string(data)
	if payload == "" {
		if explicitStdin {
			return "", true, nil
		}
		return "", false, nil
	}
	return payload, true, nil
}

func executionStdinIsTTY(cfg StartConfig) bool {
	if cfg.StdinIsTTY != nil {
		return cfg.StdinIsTTY()
	}
	if cfg.Stdin != nil && cfg.Stdin != os.Stdin {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
