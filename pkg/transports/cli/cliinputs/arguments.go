package cliinputs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	valueTypeString = "string"

	completionKindNone    = "none"
	completionKindStatic  = "static"
	completionKindDynamic = "dynamic"

	inputChannelCLI = "cli"

	doubleDashNone            = "none"
	doubleDashTerminatesFlags = "terminates-flags"

	maxArgsProbeCount = 8
)

func collectArgumentRecords(root *cobra.Command) []ArgumentRecord {
	records := []ArgumentRecord{}
	collectCommandArguments(root, &records)
	return records
}

func collectCommandArguments(cmd *cobra.Command, records *[]ArgumentRecord) {
	*records = append(*records, argumentRecordsForCommand(cmd)...)

	children := append([]*cobra.Command(nil), cmd.Commands()...)
	sort.Slice(children, func(i, j int) bool {
		return children[i].CommandPath() < children[j].CommandPath()
	})
	for _, child := range children {
		collectCommandArguments(child, records)
	}
}

func argumentRecordsForCommand(cmd *cobra.Command) []ArgumentRecord {
	profile := probeArgsProfile(cmd)
	if profile.maxCount == 0 {
		return nil
	}

	useNames := parseUseArgNames(cmd.Use)
	join := commandJoin(cmd)
	enum := normalizedValidArgs(cmd)
	completionKind := argumentCompletionKind(cmd)
	doubleDash := argumentDoubleDashHandling(profile)
	inputChannels := []string{inputChannelCLI}

	records := make([]ArgumentRecord, 0, profile.fixedSlots()+1)
	for position := 0; position < profile.fixedSlots(); position++ {
		minCard := 0
		if profile.requiredAt(position) {
			minCard = 1
		}
		records = append(records, newArgumentRecord(
			join,
			position,
			useNames,
			profile.requiredAt(position),
			minCard,
			1,
			false,
			enum,
			completionKind,
			inputChannels,
			doubleDash,
			cmd,
		))
	}

	if profile.variadic {
		position := profile.variadicPosition
		minCard := profile.minCount - profile.fixedSlots()
		if minCard < 0 {
			minCard = 0
		}
		records = append(records, newArgumentRecord(
			join,
			position,
			useNames,
			minCard > 0,
			minCard,
			-1,
			true,
			enum,
			completionKind,
			inputChannels,
			doubleDash,
			cmd,
		))
	}

	return records
}

func newArgumentRecord(
	join CommandJoin,
	position int,
	useNames []useArgName,
	required bool,
	minCardinality int,
	maxCardinality int,
	variadic bool,
	enum []string,
	completionKind string,
	inputChannels []string,
	doubleDash string,
	cmd *cobra.Command,
) ArgumentRecord {
	name := argumentName(useNames, position, variadic)
	enumCopy := enum
	if enumCopy == nil {
		enumCopy = []string{}
	}
	channelsCopy := inputChannels
	if channelsCopy == nil {
		channelsCopy = []string{}
	}

	return ArgumentRecord{
		CommandJoin:        join,
		IDCandidate:        argumentIDCandidate(join.CommandPath, position),
		Name:               name,
		DocIDCandidate:     argumentDocIDCandidate(cmd, position, name),
		Position:           position,
		Kind:               argumentKindPositional,
		ValueType:          valueTypeString,
		Required:           required,
		MinCardinality:     minCardinality,
		MaxCardinality:     maxCardinality,
		Variadic:           variadic,
		Enum:               enumCopy,
		Pattern:            "",
		CompletionKind:     completionKind,
		InputChannels:      channelsCopy,
		DoubleDashHandling: doubleDash,
	}
}

func commandJoin(cmd *cobra.Command) CommandJoin {
	path := cmd.CommandPath()
	return CommandJoin{
		CommandPath:        path,
		CommandIDCandidate: commandIDCandidate(path),
	}
}

func commandIDCandidate(path string) string {
	return strings.ReplaceAll(path, " ", ".")
}

func argumentIDCandidate(commandPath string, position int) string {
	return fmt.Sprintf("%s.arg.%d", commandIDCandidate(commandPath), position)
}

func argumentDocIDCandidate(cmd *cobra.Command, position int, fallbackName string) string {
	if cmd.Annotations != nil {
		if docID, ok := cmd.Annotations[fmt.Sprintf("docId.arg.%d", position)]; ok {
			return docID
		}
		if docID, ok := cmd.Annotations["docId.arg."+fallbackName]; ok {
			return docID
		}
	}
	return ""
}

func argumentName(useNames []useArgName, position int, variadic bool) string {
	for _, candidate := range useNames {
		if candidate.position != position {
			continue
		}
		if variadic && candidate.variadic {
			return candidate.name
		}
		if !variadic && !candidate.variadic {
			return candidate.name
		}
	}
	return fmt.Sprintf("arg%d", position)
}

func argumentCompletionKind(cmd *cobra.Command) string {
	if len(normalizedValidArgs(cmd)) > 0 {
		return completionKindStatic
	}
	if cmd.ValidArgsFunction != nil {
		return completionKindDynamic
	}
	return completionKindNone
}

func argumentDoubleDashHandling(profile argsProfile) string {
	if profile.maxCount == 0 {
		return doubleDashNone
	}
	return doubleDashTerminatesFlags
}

func normalizedValidArgs(cmd *cobra.Command) []string {
	if len(cmd.ValidArgs) == 0 {
		return nil
	}
	validArgs := make([]string, 0, len(cmd.ValidArgs))
	for _, candidate := range cmd.ValidArgs {
		validArgs = append(validArgs, strings.SplitN(string(candidate), "\t", 2)[0])
	}
	return validArgs
}

type useArgName struct {
	position int
	name     string
	variadic bool
}

func parseUseArgNames(use string) []useArgName {
	fields := strings.Fields(use)
	if len(fields) <= 1 {
		return nil
	}

	names := make([]useArgName, 0, len(fields)-1)
	position := 0
	for _, field := range fields[1:] {
		name, variadic, recognized := parseUseArgToken(field)
		if !recognized {
			continue
		}
		names = append(names, useArgName{
			position: position,
			name:     name,
			variadic: variadic,
		})
		if !variadic {
			position++
		}
	}
	return names
}

func parseUseArgToken(token string) (name string, variadic bool, recognized bool) {
	if strings.HasSuffix(token, "...") {
		base := strings.TrimSuffix(token, "...")
		if strings.HasPrefix(base, "<") && strings.HasSuffix(base, ">") {
			return strings.Trim(base, "<>"), true, true
		}
		return base, true, true
	}
	switch {
	case strings.HasPrefix(token, "<") && strings.HasSuffix(token, ">"):
		return strings.Trim(token, "<>"), false, true
	case strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]"):
		return strings.Trim(token, "[]"), false, true
	default:
		if token == strings.ToUpper(token) && strings.ContainsAny(token, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
			return strings.ToLower(token), false, true
		}
	}
	return "", false, false
}
