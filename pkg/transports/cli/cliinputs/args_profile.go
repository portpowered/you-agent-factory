package cliinputs

import (
	"fmt"

	"github.com/spf13/cobra"
)

type argsProfile struct {
	minCount         int
	maxCount         int
	variadic         bool
	variadicPosition int
}

func (p argsProfile) fixedSlots() int {
	if p.maxCount == 0 {
		return 0
	}
	if p.variadic {
		return variadicFixedSlots(p.minCount)
	}
	return p.maxCount
}

func variadicStartPosition(minCount int) int {
	return variadicFixedSlots(minCount)
}

func variadicFixedSlots(minCount int) int {
	if minCount <= 1 {
		return 0
	}
	return minCount - 1
}

func (p argsProfile) requiredAt(position int) bool {
	return position < p.minCount
}

func probeArgsProfile(cmd *cobra.Command) argsProfile {
	validator := effectiveArgsValidator(cmd)

	validCounts := make([]int, 0, maxArgsProbeCount+1)
	for count := 0; count <= maxArgsProbeCount; count++ {
		if argsValidatorAccepts(validator, cmd, makeProbeArgs(cmd, count)) {
			validCounts = append(validCounts, count)
		}
	}

	if len(validCounts) == 0 {
		return argsProfile{maxCount: 0}
	}

	minCount := validCounts[0]
	maxCount := validCounts[len(validCounts)-1]
	variadic := false
	variadicPosition := -1

	if maxCount == maxArgsProbeCount && argsValidatorAccepts(validator, cmd, makeProbeArgs(cmd, maxArgsProbeCount+1)) {
		variadic = true
		variadicPosition = variadicStartPosition(minCount)
		maxCount = -1
	}

	if !variadic && minCount == maxCount && minCount == 0 {
		return argsProfile{minCount: 0, maxCount: 0}
	}

	return argsProfile{
		minCount:         minCount,
		maxCount:         maxCount,
		variadic:         variadic,
		variadicPosition: variadicPosition,
	}
}

func effectiveArgsValidator(cmd *cobra.Command) cobra.PositionalArgs {
	if cmd.Args != nil {
		return cmd.Args
	}
	return legacyArgsValidator
}

func legacyArgsValidator(cmd *cobra.Command, args []string) error {
	if !cmd.HasSubCommands() {
		return nil
	}
	if !cmd.HasParent() && len(args) > 0 {
		return fmt.Errorf("legacy args reject positional input on grouped root")
	}
	return nil
}

func argsValidatorAccepts(validator cobra.PositionalArgs, cmd *cobra.Command, args []string) bool {
	if validator == nil {
		return true
	}
	return validator(cmd, args) == nil
}

func makeProbeArgs(cmd *cobra.Command, count int) []string {
	if count == 0 {
		return nil
	}

	validArgs := normalizedValidArgs(cmd)
	args := make([]string, count)
	for i := range args {
		if len(validArgs) > 0 {
			args[i] = validArgs[i%len(validArgs)]
			continue
		}
		args[i] = fmt.Sprintf("_cliinputs_probe_%d", i)
	}
	return args
}
