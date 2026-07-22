package token

import "github.com/portpowered/infinite-you/pkg/services/work"

// Clone returns a detached token copy.
func Clone(value Token) Token {
	value.Color = CloneColor(value.Color)
	value.History = CloneHistory(value.History)
	return value
}

// CloneSlice returns detached token copies.
func CloneSlice(values []Token) []Token {
	if values == nil {
		return nil
	}
	clones := make([]Token, len(values))
	for i := range values {
		clones[i] = Clone(values[i])
	}
	return clones
}

// CloneColor returns a detached token-color copy.
func CloneColor(value Color) Color {
	value.PreviousChainingTraceIDs = cloneStringSlice(value.PreviousChainingTraceIDs)
	value.Tags = cloneStringMap(value.Tags)
	value.Relations = cloneRelations(value.Relations)
	value.Content = work.CloneWorkContentParts(value.Content)
	value.Payload = cloneBytes(value.Payload)
	value.InvocationArguments = work.CloneInvocationArguments(value.InvocationArguments)
	return value
}

// CloneHistory returns a detached token-history copy.
func CloneHistory(value History) History {
	value.TotalVisits = cloneStringIntMap(value.TotalVisits)
	value.ConsecutiveFailures = cloneStringIntMap(value.ConsecutiveFailures)
	value.PlaceVisits = cloneStringIntMap(value.PlaceVisits)
	value.FailureLog = cloneFailures(value.FailureLog)
	return value
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func cloneRelations(values []work.Relation) []work.Relation {
	if values == nil {
		return nil
	}
	return append([]work.Relation{}, values...)
}

func cloneBytes(values []byte) []byte {
	if values == nil {
		return nil
	}
	return append([]byte{}, values...)
}

func cloneFailures(values []Failure) []Failure {
	if values == nil {
		return nil
	}
	return append([]Failure{}, values...)
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
