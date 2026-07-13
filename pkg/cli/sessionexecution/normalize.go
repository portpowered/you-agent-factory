package sessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
)

// NormalizeStartRequest resolves CLI flags, positional values, and stdin into the
// shared durable Factory Session execution request shape.
func NormalizeStartRequest(cfg StartConfig) (factorysessionexecution.StartRequest, ExecutionMode, error) {
	mode, err := normalizeExecutionMode(cfg.Mode)
	if err != nil {
		return factorysessionexecution.StartRequest{}, "", err
	}

	source, err := resolveExecutionSource(cfg)
	if err != nil {
		return factorysessionexecution.StartRequest{}, "", err
	}

	args, err := parseArgsJSON(cfg.ArgsJSON)
	if err != nil {
		return factorysessionexecution.StartRequest{}, "", err
	}
	policy, err := parseRequestedPolicy(cfg.PolicyJSON, cfg.PolicyHash)
	if err != nil {
		return factorysessionexecution.StartRequest{}, "", err
	}

	input := factorysession.CLIStartInput{
		RequestID:       strings.TrimSpace(cfg.RequestID),
		Source:          source,
		Args:            args,
		RequestedPolicy: policy,
	}
	if trimmed := strings.TrimSpace(cfg.ChildExecutorMode); trimmed != "" {
		input.Runtime = &factorysessionexecution.RuntimeOptions{
			ChildExecutorMode: trimmed,
		}
	}
	if mode == ExecutionModeSync {
		input.Wait = &factorysessionexecution.WaitOptions{
			TimeoutMillis:   cfg.WaitTimeoutMillis,
			CancelOnTimeout: cfg.CancelOnTimeout,
		}
	}

	normalized, err := factorysession.StartRequestFromCLI(input)
	if err != nil {
		return factorysessionexecution.StartRequest{}, "", err
	}
	return normalized, mode, nil
}

func normalizeExecutionMode(mode ExecutionMode) (ExecutionMode, error) {
	switch mode {
	case ExecutionModeSync, ExecutionModeAsync:
		return mode, nil
	case "":
		return "", newExecutionError(
			ErrorCodeUnsupportedMode,
			"session execution mode is required: use sync or async",
			"mode",
		)
	default:
		return "", newExecutionError(
			ErrorCodeUnsupportedMode,
			fmt.Sprintf("session execution mode %q is unsupported: use sync or async", mode),
			"mode",
		)
	}
}

func parseArgsJSON(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(trimmed), &args); err != nil {
		return nil, newExecutionError(
			ErrorCodeInvalidArgs,
			"session execution --args must be a JSON object",
			"args",
		)
	}
	if args == nil {
		return nil, newExecutionError(
			ErrorCodeInvalidArgs,
			"session execution --args must be a JSON object",
			"args",
		)
	}
	return args, nil
}

func parseRequestedPolicy(policyJSON, policyHash string) (map[string]any, error) {
	trimmedJSON := strings.TrimSpace(policyJSON)
	trimmedHash := strings.TrimSpace(policyHash)
	if trimmedJSON == "" && trimmedHash == "" {
		return nil, nil
	}
	if trimmedJSON != "" && trimmedHash != "" {
		return nil, newExecutionError(
			ErrorCodeSourceConflict,
			"session execution requested policy selectors conflict: --policy and --policy-hash",
			"requestedPolicy",
		)
	}
	if trimmedHash != "" {
		return map[string]any{"policyHash": trimmedHash}, nil
	}
	var policy map[string]any
	if err := json.Unmarshal([]byte(trimmedJSON), &policy); err != nil {
		return nil, newExecutionError(
			ErrorCodeInvalidPolicy,
			"session execution --policy must be a JSON object",
			"requestedPolicy",
		)
	}
	if policy == nil {
		return nil, newExecutionError(
			ErrorCodeInvalidPolicy,
			"session execution --policy must be a JSON object",
			"requestedPolicy",
		)
	}
	return policy, nil
}
