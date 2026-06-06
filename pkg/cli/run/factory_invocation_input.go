package run

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	InvocationErrorCodeAmbiguousInput = "RUN_INVOCATION_AMBIGUOUS_INPUT"
)

type InvocationInputSource string

const (
	InvocationInputSourcePositional InvocationInputSource = "positional prompt"
	InvocationInputSourceStdin      InvocationInputSource = "stdin"
)

type FactoryInvocationInputConfig struct {
	PromptArgs []string
	Stdin      io.Reader
	StdinIsTTY func() bool
}

type FactoryInvocationInput struct {
	Source  InvocationInputSource
	Payload string
}

// ResolveFactoryInvocationInput resolves the one-shot invocation payload from
// positional prompt args, explicit "-" stdin, or piped non-TTY stdin.
func ResolveFactoryInvocationInput(cfg FactoryInvocationInputConfig) (FactoryInvocationInput, error) {
	positionalPrompt, explicitStdin := splitInvocationPromptArgs(cfg.PromptArgs)
	stdinPayload, hasStdin, err := resolveInvocationStdin(cfg, explicitStdin)
	if err != nil {
		return FactoryInvocationInput{}, err
	}

	var sources []InvocationInputSource
	if positionalPrompt != "" {
		sources = append(sources, InvocationInputSourcePositional)
	}
	if hasStdin {
		sources = append(sources, InvocationInputSourceStdin)
	}
	if len(sources) > 1 {
		return FactoryInvocationInput{}, ambiguousInvocationInputError(sources)
	}
	if positionalPrompt != "" {
		return FactoryInvocationInput{Source: InvocationInputSourcePositional, Payload: positionalPrompt}, nil
	}
	if hasStdin {
		return FactoryInvocationInput{Source: InvocationInputSourceStdin, Payload: stdinPayload}, nil
	}
	return FactoryInvocationInput{}, nil
}

func splitInvocationPromptArgs(args []string) (prompt string, explicitStdin bool) {
	positional := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "-" {
			explicitStdin = true
			continue
		}
		positional = append(positional, arg)
	}
	return strings.TrimSpace(strings.Join(positional, " ")), explicitStdin
}

func resolveInvocationStdin(cfg FactoryInvocationInputConfig, explicitStdin bool) (string, bool, error) {
	if !explicitStdin && invocationStdinIsTTY(cfg) {
		return "", false, nil
	}

	stdin := cfg.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", false, fmt.Errorf("read invocation stdin: %w", err)
	}
	payload := strings.TrimSpace(string(data))
	if payload == "" {
		if explicitStdin {
			return "", false, fmt.Errorf("stdin input is empty")
		}
		return "", false, nil
	}
	return payload, true, nil
}

func invocationStdinIsTTY(cfg FactoryInvocationInputConfig) bool {
	if cfg.StdinIsTTY != nil {
		return cfg.StdinIsTTY()
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func ambiguousInvocationInputError(sources []InvocationInputSource) error {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		labels = append(labels, string(source))
	}
	return fmt.Errorf("%s: ambiguous invocation input sources: %s", InvocationErrorCodeAmbiguousInput, strings.Join(labels, " and "))
}
