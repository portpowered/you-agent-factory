package climanifestcobra

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// PersistentFlagBindings supplies live variables for root persistent flags declared
// in generated representative-family metadata.
type PersistentFlagBindings struct {
	Verbose                    *bool
	Debug                      *bool
	Server                     *string
	JSON                       *bool
	DefaultWorkerModelProvider *string
	DefaultWorkerModel         *string
	// FlagUsages supplies Cobra help text for persistent flags when the manifest
	// does not yet carry per-flag usage descriptions.
	FlagUsages map[string]string
}

const genericInputAnnotationPrefix = "infinite-you/input-id/"
const genericArgumentAnnotationPrefix = "infinite-you/argument-value/"

type encodedArgumentValue struct {
	ValueType string          `json:"valueType"`
	Present   bool            `json:"present"`
	Value     json.RawMessage `json:"value"`
}

// InputValues returns the parsed flag and positional inputs declared for cmd,
// keyed by stable manifest input ID. Returned repeated values are detached copies.
func InputValues(cmd *cobra.Command) (map[string]any, error) {
	if cmd == nil {
		return nil, fmt.Errorf("read generic command inputs: command is required")
	}
	values := make(map[string]any)
	for key, longName := range cmd.Annotations {
		if strings.HasPrefix(key, genericArgumentAnnotationPrefix) {
			inputID := strings.TrimPrefix(key, genericArgumentAnnotationPrefix)
			value, err := decodeArgumentValue(longName)
			if err != nil {
				return nil, fmt.Errorf("read generic command argument %q: %w", inputID, err)
			}
			values[inputID] = value
			continue
		}
		if !strings.HasPrefix(key, genericInputAnnotationPrefix) {
			continue
		}
		inputID := strings.TrimPrefix(key, genericInputAnnotationPrefix)
		flag := lookupCommandFlag(cmd, longName)
		if flag == nil {
			return nil, fmt.Errorf("read generic command input %q: flag %q is unavailable", inputID, longName)
		}
		getter, ok := flag.Value.(interface{ Get() any })
		if !ok {
			return nil, fmt.Errorf("read generic command input %q: flag %q has no typed value", inputID, longName)
		}
		values[inputID] = cloneGenericInputValue(getter.Get())
	}
	return values, nil
}

func decodeArgumentValue(encoded string) (any, error) {
	var record encodedArgumentValue
	if err := json.Unmarshal([]byte(encoded), &record); err != nil {
		return nil, fmt.Errorf("decode stored value: %w", err)
	}
	if !record.Present && string(record.Value) == "null" {
		return nil, nil
	}
	target, err := argumentDecodeTarget(record.ValueType)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(record.Value, target); err != nil {
		return nil, fmt.Errorf("decode %s value: %w", record.ValueType, err)
	}
	return dereferenceArgumentValue(target)
}

func argumentDecodeTarget(valueType string) (any, error) {
	switch valueType {
	case "bool":
		return new(bool), nil
	case "string":
		return new(string), nil
	case "int":
		return new(int), nil
	case "int64":
		return new(int64), nil
	case "boolArray":
		return new([]bool), nil
	case "stringArray":
		return new([]string), nil
	case "intArray":
		return new([]int), nil
	case "int64Array":
		return new([]int64), nil
	default:
		return nil, fmt.Errorf("unsupported stored value type %q", valueType)
	}
}

func dereferenceArgumentValue(target any) (any, error) {
	switch typed := target.(type) {
	case *bool:
		return *typed, nil
	case *string:
		return *typed, nil
	case *int:
		return *typed, nil
	case *int64:
		return *typed, nil
	case *[]bool:
		return append([]bool(nil), (*typed)...), nil
	case *[]string:
		return append([]string(nil), (*typed)...), nil
	case *[]int:
		return append([]int(nil), (*typed)...), nil
	case *[]int64:
		return append([]int64(nil), (*typed)...), nil
	default:
		return nil, fmt.Errorf("unsupported decoded value")
	}
}

func lookupCommandFlag(cmd *cobra.Command, name string) *pflag.Flag {
	for current := cmd; current != nil; current = current.Parent() {
		if flag := current.Flags().Lookup(name); flag != nil {
			return flag
		}
		if flag := current.PersistentFlags().Lookup(name); flag != nil {
			return flag
		}
	}
	return nil
}

type genericFlagValue struct {
	valueType     string
	normalization string
	enum          map[string]struct{}
	value         any
	arrayChanged  bool
}

func newGenericFlagValue(flag climanifest.Flag) (*genericFlagValue, error) {
	value, err := genericFlagDefault(flag)
	if err != nil {
		return nil, err
	}
	choices := make(map[string]struct{}, len(flag.Enum))
	for _, choice := range flag.Enum {
		choices[choice] = struct{}{}
	}
	return &genericFlagValue{
		valueType:     flag.ValueType,
		normalization: flag.Normalization,
		enum:          choices,
		value:         cloneGenericInputValue(normalizeGenericInput(value, flag.Normalization)),
	}, nil
}

func (v *genericFlagValue) Set(raw string) error {
	value, err := parseGenericFlagString(v.valueType, raw)
	if err != nil {
		return err
	}
	value = normalizeGenericInput(value, v.normalization)
	if err := validateGenericChoice(v.enum, value); err != nil {
		return err
	}
	if v.valueType != "stringArray" {
		v.value = value
		return nil
	}
	item := value.([]string)[0]
	if !v.arrayChanged {
		v.value = []string{item}
		v.arrayChanged = true
		return nil
	}
	v.value = append(v.value.([]string), item)
	return nil
}

func (v *genericFlagValue) Type() string {
	return v.valueType
}

func (v *genericFlagValue) String() string {
	return genericFlagString(v.value)
}

func (v *genericFlagValue) Get() any {
	return cloneGenericInputValue(v.value)
}

func (v *genericFlagValue) Append(raw string) error {
	if v.valueType != "stringArray" {
		return fmt.Errorf("value type %q is not a repeated string", v.valueType)
	}
	value, err := parseGenericFlagString(v.valueType, raw)
	if err != nil {
		return err
	}
	item := normalizeGenericInput(value, v.normalization).([]string)[0]
	if err := validateGenericChoice(v.enum, item); err != nil {
		return err
	}
	v.value = append(v.value.([]string), item)
	return nil
}

func (v *genericFlagValue) Replace(values []string) error {
	replacement := make([]string, 0, len(values))
	for _, raw := range values {
		value := normalizeGenericInput([]string{raw}, v.normalization).([]string)[0]
		if err := validateGenericChoice(v.enum, value); err != nil {
			return err
		}
		replacement = append(replacement, value)
	}
	v.value = replacement
	v.arrayChanged = true
	return nil
}

func (v *genericFlagValue) GetSlice() []string {
	if values, ok := v.value.([]string); ok {
		return append([]string(nil), values...)
	}
	return nil
}

func genericFlagDefault(flag climanifest.Flag) (any, error) {
	if flag.DefaultValue != nil {
		return typedGenericInputValue(flag.ValueType, flag.DefaultValue)
	}
	if flag.ValueType == "stringArray" {
		if flag.Default != "" {
			return nil, fmt.Errorf("repeated-string default must use defaultValue.stringArray")
		}
		return []string{}, nil
	}
	return parseGenericFlagString(flag.ValueType, flag.Default)
}

func hasGenericFlagDefault(flag climanifest.Flag) bool {
	if flag.DefaultValue != nil || flag.Default != "" {
		return true
	}
	for _, source := range flag.AcceptedSources {
		if source == "manifest-default" {
			return true
		}
	}
	return false
}

func genericNoOptionValue(flag climanifest.Flag) (any, error) {
	if flag.NoOptionValue != nil {
		return typedGenericInputValue(flag.ValueType, flag.NoOptionValue)
	}
	return parseGenericFlagString(flag.ValueType, flag.NoOptionDefault)
}

func typedGenericInputValue(valueType string, authored *climanifest.InputValue) (any, error) {
	count := 0
	var value any
	if authored.Boolean != nil {
		count++
		value = *authored.Boolean
	}
	if authored.String != nil {
		count++
		value = *authored.String
	}
	if authored.Int != nil {
		count++
		value = *authored.Int
	}
	if authored.Int64 != nil {
		count++
		value = *authored.Int64
	}
	if authored.StringArray != nil {
		count++
		value = append([]string(nil), (*authored.StringArray)...)
	}
	if count != 1 {
		return nil, fmt.Errorf("typed value must author exactly one member")
	}
	matches := map[string]bool{
		"bool":        authored.Boolean != nil,
		"string":      authored.String != nil,
		"int":         authored.Int != nil,
		"int64":       authored.Int64 != nil,
		"stringArray": authored.StringArray != nil,
	}
	if !matches[valueType] {
		return nil, fmt.Errorf("typed value member does not match value type %q", valueType)
	}
	return value, nil
}

func parseGenericFlagString(valueType, raw string) (any, error) {
	switch valueType {
	case "bool":
		if raw == "" {
			return false, nil
		}
		return strconv.ParseBool(raw)
	case "string":
		return raw, nil
	case "int":
		if raw == "" {
			return 0, nil
		}
		return strconv.Atoi(raw)
	case "int64":
		if raw == "" {
			return int64(0), nil
		}
		return strconv.ParseInt(raw, 10, 64)
	case "stringArray":
		return []string{raw}, nil
	default:
		return nil, fmt.Errorf("unsupported value type %q", valueType)
	}
}

func normalizeGenericInput(value any, mode string) any {
	if mode != "trim" {
		return value
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		normalized := make([]string, len(typed))
		for index := range typed {
			normalized[index] = strings.TrimSpace(typed[index])
		}
		return normalized
	default:
		return value
	}
}

func validateEnumValue(flag climanifest.Flag, value any) error {
	choices := make(map[string]struct{}, len(flag.Enum))
	for _, choice := range flag.Enum {
		choices[choice] = struct{}{}
	}
	return validateGenericChoice(choices, normalizeGenericInput(value, flag.Normalization))
}

func validateGenericChoice(choices map[string]struct{}, value any) error {
	if len(choices) == 0 {
		return nil
	}
	values, ok := value.([]string)
	if !ok {
		values = []string{value.(string)}
	}
	for _, candidate := range values {
		if _, exists := choices[candidate]; !exists {
			return fmt.Errorf("value %q is not one of the declared choices", candidate)
		}
	}
	return nil
}

func genericFlagString(value any) string {
	switch typed := value.(type) {
	case []string:
		return "[" + strings.Join(typed, ",") + "]"
	default:
		return fmt.Sprint(value)
	}
}

func cloneGenericInputValue(value any) any {
	switch values := value.(type) {
	case []bool:
		return append([]bool(nil), values...)
	case []string:
		return append([]string(nil), values...)
	case []int:
		return append([]int(nil), values...)
	case []int64:
		return append([]int64(nil), values...)
	}
	return value
}

type plannedRelationship struct {
	record       climanifest.Relationship
	participants []plannedParticipant
	when         *plannedParticipant
}

type plannedParticipant struct {
	id        string
	kind      string
	public    string
	flagNames []string
}

func planCommandArgumentsAndRelationships(plan []plannedCommand) error {
	inputOwners := make(map[string]string)
	for _, item := range plan {
		for _, flag := range item.record.Flags {
			inputOwners[flag.ID] = item.record.ID
		}
	}
	for index := range plan {
		arguments, err := validateAndSortArguments(plan[index].record, inputOwners)
		if err != nil {
			return err
		}
		plan[index].arguments = arguments
		relationships, err := planRelationships(plan, index)
		if err != nil {
			return err
		}
		plan[index].relationships = relationships
	}
	return nil
}

func validateAndSortArguments(record climanifest.Command, owners map[string]string) ([]climanifest.Argument, error) {
	keys := make([]string, 0, len(record.Arguments))
	for key := range record.Arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	arguments := make([]climanifest.Argument, 0, len(keys))
	for _, key := range keys {
		argument := record.Arguments[key]
		if key != argument.ID {
			return nil, genericArgumentError(record.ID, argument.ID, "map key %q does not match input id", key)
		}
		if owner, exists := owners[argument.ID]; exists {
			return nil, genericArgumentError(record.ID, argument.ID, "stable input ID is already declared by command %q", owner)
		}
		owners[argument.ID] = record.ID
		if err := validateArgumentShape(record.ID, argument); err != nil {
			return nil, err
		}
		arguments = append(arguments, argument)
	}
	sort.Slice(arguments, func(i, j int) bool { return arguments[i].Position < arguments[j].Position })
	for position, argument := range arguments {
		if argument.Position != position {
			return nil, genericArgumentError(record.ID, argument.ID, "position %d leaves an impossible positional layout; want %d", argument.Position, position)
		}
		if argument.Variadic && position != len(arguments)-1 {
			return nil, genericArgumentError(record.ID, argument.ID, "variadic argument must be last")
		}
	}
	return arguments, nil
}

func validateArgumentShape(commandID string, argument climanifest.Argument) error {
	if strings.TrimSpace(argument.ID) == "" || strings.TrimSpace(argument.Name) == "" {
		return genericArgumentError(commandID, argument.ID, "stable input ID and public name are required")
	}
	if argument.Kind != "positional" {
		return genericArgumentError(commandID, argument.ID, "unsupported argument kind %q", argument.Kind)
	}
	if argument.Scope != "" && argument.Scope != "local" {
		return genericArgumentError(commandID, argument.ID, "unsupported scope %q", argument.Scope)
	}
	switch argument.ValueType {
	case "bool", "string", "int", "int64", "stringArray":
	default:
		return genericArgumentError(commandID, argument.ID, "unsupported value type %q", argument.ValueType)
	}
	if err := validateArgumentCardinality(commandID, argument); err != nil {
		return err
	}
	if err := validateArgumentRecordShape(commandID, argument); err != nil {
		return err
	}
	if err := validateArgumentDoubleDash(commandID, argument); err != nil {
		return err
	}
	return validateArgumentValueContract(commandID, argument)
}

func validateArgumentCardinality(commandID string, argument climanifest.Argument) error {
	if argument.MinCardinality < 0 || argument.MaxCardinality < -1 ||
		(argument.MaxCardinality >= 0 && argument.MinCardinality > argument.MaxCardinality) {
		return genericArgumentError(commandID, argument.ID, "invalid cardinality %d..%d", argument.MinCardinality, argument.MaxCardinality)
	}
	if argument.Required != (argument.MinCardinality > 0) {
		return genericArgumentError(commandID, argument.ID, "required=%t is inconsistent with minimum cardinality %d", argument.Required, argument.MinCardinality)
	}
	if argument.Variadic != (argument.MaxCardinality == -1) {
		return genericArgumentError(commandID, argument.ID, "variadic=%t is inconsistent with maximum cardinality %d", argument.Variadic, argument.MaxCardinality)
	}
	if argument.MaxCardinality == 0 {
		return genericArgumentError(commandID, argument.ID, "maximum cardinality must accept at least one value")
	}
	return nil
}

func validateArgumentDoubleDash(commandID string, argument climanifest.Argument) error {
	switch argument.DoubleDash {
	case "terminates-flags":
		return nil
	case "":
		return genericArgumentError(commandID, argument.ID, "doubleDash mode is required")
	case "none":
		return genericArgumentError(commandID, argument.ID, "doubleDash mode %q is not supported by Cobra projection", argument.DoubleDash)
	default:
		return genericArgumentError(commandID, argument.ID, "unsupported doubleDash mode %q", argument.DoubleDash)
	}
}

func validateArgumentValueContract(commandID string, argument climanifest.Argument) error {
	if len(argument.Enum) > 0 && argument.ValueType != "string" && argument.ValueType != "stringArray" {
		return genericArgumentError(commandID, argument.ID, "enumerated choices are incompatible with value type %q", argument.ValueType)
	}
	if argument.Pattern != "" {
		if argument.ValueType != "string" && argument.ValueType != "stringArray" {
			return genericArgumentError(commandID, argument.ID, "pattern is incompatible with value type %q", argument.ValueType)
		}
		if _, err := regexp.Compile(argument.Pattern); err != nil {
			return genericArgumentError(commandID, argument.ID, "invalid pattern: %v", err)
		}
	}
	if argument.DefaultValue == nil {
		return nil
	}
	if argument.Required {
		return genericArgumentError(commandID, argument.ID, "required argument cannot declare a default")
	}
	value, err := typedGenericInputValue(argument.ValueType, argument.DefaultValue)
	if err != nil {
		return genericArgumentError(commandID, argument.ID, "invalid typed default: %v", err)
	}
	if err := validateArgumentCandidate(argument, value); err != nil {
		return genericArgumentError(commandID, argument.ID, "invalid typed default: %v", err)
	}
	return nil
}

func genericArgumentError(commandID, inputID, format string, args ...any) error {
	return fmt.Errorf("command %q argument %q: %s", commandID, inputID, fmt.Sprintf(format, args...))
}

func planRelationships(plan []plannedCommand, commandIndex int) ([]plannedRelationship, error) {
	record := plan[commandIndex].record
	available := effectiveParticipants(plan, commandIndex)
	keys := make([]string, 0, len(record.Relationships))
	for key := range record.Relationships {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	relationships := make([]plannedRelationship, 0, len(keys))
	for _, key := range keys {
		relationship := record.Relationships[key]
		if key != relationship.ID || strings.TrimSpace(relationship.ID) == "" {
			return nil, fmt.Errorf("command %q relationship map key %q does not match id %q", record.ID, key, relationship.ID)
		}
		planned, err := validateRelationshipShape(record.ID, relationship, available)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, planned)
	}
	return relationships, nil
}

func effectiveParticipants(plan []plannedCommand, commandIndex int) map[string]plannedParticipant {
	available := make(map[string]plannedParticipant)
	commandPath := plan[commandIndex].record.Path
	for index := 0; index <= commandIndex; index++ {
		record := plan[index].record
		if record.Path != commandPath && !strings.HasPrefix(commandPath, record.Path+" ") {
			continue
		}
		for _, flag := range record.Flags {
			if record.Path == commandPath || flag.Scope == "persistent" {
				names := append([]string{flag.Long}, flag.Aliases...)
				available[flag.ID] = plannedParticipant{
					id: flag.ID, kind: "flag", public: "--" + flag.Long, flagNames: names,
				}
			}
		}
	}
	for _, argument := range plan[commandIndex].arguments {
		available[argument.ID] = plannedParticipant{id: argument.ID, kind: "argument", public: argument.Name}
	}
	return available
}

func validateRelationshipShape(
	commandID string,
	relationship climanifest.Relationship,
	available map[string]plannedParticipant,
) (plannedRelationship, error) {
	mode := relationshipMode(relationship.Kind)
	if mode == "" {
		return plannedRelationship{}, fmt.Errorf("command %q relationship %q has unsupported kind %q", commandID, relationship.ID, relationship.Kind)
	}
	if len(relationship.Participants) == 0 || (mode == "group" && len(relationship.Participants) < 2) {
		return plannedRelationship{}, fmt.Errorf("command %q relationship %q has unsupported participant count %d", commandID, relationship.ID, len(relationship.Participants))
	}
	if (mode == "directed") != (relationship.When != nil) {
		return plannedRelationship{}, fmt.Errorf("command %q relationship %q has incompatible when trigger", commandID, relationship.ID)
	}
	planned := plannedRelationship{record: relationship}
	seen := make(map[string]bool)
	for _, reference := range relationship.Participants {
		participant, err := resolveParticipant(commandID, relationship.ID, reference, available)
		if err != nil {
			return plannedRelationship{}, err
		}
		if seen[participant.id] {
			return plannedRelationship{}, fmt.Errorf("command %q relationship %q repeats participant %q", commandID, relationship.ID, participant.id)
		}
		seen[participant.id] = true
		planned.participants = append(planned.participants, participant)
	}
	if relationship.When != nil {
		when, err := resolveParticipant(commandID, relationship.ID, *relationship.When, available)
		if err != nil {
			return plannedRelationship{}, err
		}
		if seen[when.id] {
			return plannedRelationship{}, fmt.Errorf("command %q relationship %q trigger %q depends on itself", commandID, relationship.ID, when.id)
		}
		planned.when = &when
	}
	return planned, nil
}

func relationshipMode(kind string) string {
	switch kind {
	case "mutually-exclusive", "required-together", "at-least-one", "conflict":
		return "group"
	case "dependency", "conditional":
		return "directed"
	default:
		return ""
	}
}

func resolveParticipant(
	commandID, relationshipID string,
	reference climanifest.ParticipantRef,
	available map[string]plannedParticipant,
) (plannedParticipant, error) {
	participant, exists := available[reference.ID]
	if !exists {
		return plannedParticipant{}, fmt.Errorf("command %q relationship %q references unknown participant %q", commandID, relationshipID, reference.ID)
	}
	if reference.Type != participant.kind {
		return plannedParticipant{}, fmt.Errorf(
			"command %q relationship %q participant %q is a %s, not %s",
			commandID, relationshipID, reference.ID, participant.kind, reference.Type,
		)
	}
	return participant, nil
}

func projectArgumentAndRelationshipRules(cmd *cobra.Command, plan plannedCommand) {
	if len(plan.arguments) > 0 {
		cmd.Args = func(command *cobra.Command, raw []string) error {
			return assignArgumentValues(command, plan, raw)
		}
	}
	if len(plan.flags) == 0 && len(plan.relationships) == 0 {
		return
	}
	cmd.PreRunE = func(command *cobra.Command, _ []string) error {
		if err := validateRequiredGenericFlags(command, plan); err != nil {
			return err
		}
		return validateRelationships(command, plan.relationships)
	}
}

func assignArgumentValues(cmd *cobra.Command, plan plannedCommand, raw []string) error {
	minimum, maximum := argumentCardinality(plan.arguments)
	if len(raw) < minimum {
		return fmt.Errorf("requires at least %d arg(s), only received %d", minimum, len(raw))
	}
	if maximum >= 0 && len(raw) > maximum {
		return fmt.Errorf("accepts at most %d arg(s), received %d", maximum, len(raw))
	}
	offset := 0
	counts := argumentValueCounts(plan.arguments, len(raw))
	for index, argument := range plan.arguments {
		count := counts[index]
		values := append([]string(nil), raw[offset:offset+count]...)
		offset += count
		value, err := parseArgumentValues(argument, values)
		if err != nil {
			return genericArgumentError(plan.record.ID, argument.ID, "%v", err)
		}
		if err := storeArgumentValue(cmd, argument, len(values) > 0, value); err != nil {
			return genericArgumentError(plan.record.ID, argument.ID, "store parsed value: %v", err)
		}
	}
	return nil
}

func argumentValueCounts(arguments []climanifest.Argument, supplied int) []int {
	counts := make([]int, len(arguments))
	offset := 0
	for index, argument := range arguments {
		count := supplied - offset - minimumCardinality(arguments[index+1:])
		if count < 0 {
			count = 0
		}
		if argument.MaxCardinality >= 0 && count > argument.MaxCardinality {
			count = argument.MaxCardinality
		}
		counts[index] = count
		offset += count
	}
	return counts
}

func argumentCardinality(arguments []climanifest.Argument) (int, int) {
	minimum := minimumCardinality(arguments)
	maximum := 0
	for _, argument := range arguments {
		if argument.MaxCardinality < 0 {
			return minimum, -1
		}
		maximum += argument.MaxCardinality
	}
	return minimum, maximum
}

func minimumCardinality(arguments []climanifest.Argument) int {
	total := 0
	for _, argument := range arguments {
		total += argument.MinCardinality
	}
	return total
}

func parseArgumentValues(argument climanifest.Argument, raw []string) (any, error) {
	if len(raw) == 0 {
		if argument.DefaultValue == nil {
			if argument.MaxCardinality != 1 {
				return emptyArgumentSlice(argument.ValueType), nil
			}
			return nil, nil
		}
		return typedGenericInputValue(argument.ValueType, argument.DefaultValue)
	}
	values := make([]any, len(raw))
	for index, item := range raw {
		value, err := parseGenericFlagString(argument.ValueType, item)
		if err != nil {
			return nil, fmt.Errorf("value %q: %w", item, err)
		}
		if err := validateArgumentCandidate(argument, value); err != nil {
			return nil, err
		}
		values[index] = value
	}
	if argument.MaxCardinality == 1 {
		return values[0], nil
	}
	return typedArgumentSlice(argument.ValueType, values), nil
}

func validateArgumentCandidate(argument climanifest.Argument, value any) error {
	if err := validateGenericChoice(stringChoiceSet(argument.Enum), value); err != nil {
		return err
	}
	if argument.Pattern == "" {
		return nil
	}
	pattern := regexp.MustCompile(argument.Pattern)
	for _, candidate := range argumentStrings(value) {
		if !pattern.MatchString(candidate) {
			return fmt.Errorf("value %q does not match declared pattern", candidate)
		}
	}
	return nil
}

func stringChoiceSet(choices []string) map[string]struct{} {
	result := make(map[string]struct{}, len(choices))
	for _, choice := range choices {
		result[choice] = struct{}{}
	}
	return result
}

func argumentStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return []string{typed}
	case []string:
		return typed
	default:
		return nil
	}
}

func emptyArgumentSlice(valueType string) any {
	switch valueType {
	case "bool":
		return []bool{}
	case "int":
		return []int{}
	case "int64":
		return []int64{}
	default:
		return []string{}
	}
}

func typedArgumentSlice(valueType string, values []any) any {
	switch valueType {
	case "bool":
		result := make([]bool, len(values))
		for index := range values {
			result[index] = values[index].(bool)
		}
		return result
	case "int":
		result := make([]int, len(values))
		for index := range values {
			result[index] = values[index].(int)
		}
		return result
	case "int64":
		result := make([]int64, len(values))
		for index := range values {
			result[index] = values[index].(int64)
		}
		return result
	default:
		result := make([]string, len(values))
		for index := range values {
			item := values[index]
			if repeated, ok := item.([]string); ok {
				result[index] = repeated[0]
			} else {
				result[index] = item.(string)
			}
		}
		return result
	}
}

func storeArgumentValue(cmd *cobra.Command, argument climanifest.Argument, present bool, value any) error {
	if cmd.Annotations == nil {
		cmd.Annotations = make(map[string]string)
	}
	valueType := argument.ValueType
	if argument.MaxCardinality != 1 {
		switch argument.ValueType {
		case "bool":
			valueType = "boolArray"
		case "int":
			valueType = "intArray"
		case "int64":
			valueType = "int64Array"
		default:
			valueType = "stringArray"
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(encodedArgumentValue{ValueType: valueType, Present: present, Value: raw})
	if err != nil {
		return err
	}
	cmd.Annotations[genericArgumentAnnotationPrefix+argument.ID] = string(encoded)
	return nil
}

func validateRelationships(cmd *cobra.Command, relationships []plannedRelationship) error {
	for _, relationship := range relationships {
		present := 0
		for _, participant := range relationship.participants {
			if participantPresent(cmd, participant) {
				present++
			}
		}
		switch relationship.record.Kind {
		case "mutually-exclusive", "conflict":
			if present > 1 {
				return relationshipError(relationship, "cannot be used together")
			}
		case "required-together":
			if present != 0 && present != len(relationship.participants) {
				return relationshipError(relationship, "must be provided together")
			}
		case "at-least-one":
			if present == 0 {
				return relationshipError(relationship, "requires at least one of")
			}
		case "dependency", "conditional":
			if relationship.when != nil && participantPresent(cmd, *relationship.when) &&
				present != len(relationship.participants) {
				return relationshipError(relationship, relationship.when.public+" requires")
			}
		}
	}
	return nil
}

func participantPresent(cmd *cobra.Command, participant plannedParticipant) bool {
	if participant.kind == "flag" {
		for _, name := range participant.flagNames {
			if flag := lookupCommandFlag(cmd, name); flag != nil && flag.Changed {
				return true
			}
		}
		return false
	}
	encoded := cmd.Annotations[genericArgumentAnnotationPrefix+participant.id]
	var value encodedArgumentValue
	return json.Unmarshal([]byte(encoded), &value) == nil && value.Present
}

func relationshipError(relationship plannedRelationship, message string) error {
	names := make([]string, len(relationship.participants))
	for index, participant := range relationship.participants {
		names[index] = participant.public
	}
	return fmt.Errorf(
		"input relationship %q: %s %s",
		relationship.record.ID,
		message,
		strings.Join(names, ", "),
	)
}
