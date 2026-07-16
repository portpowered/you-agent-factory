package token

import "github.com/portpowered/infinite-you/pkg/work"

// PreviousChainingTraceIDs returns canonical predecessor trace IDs from work
// tokens while ignoring resource tokens.
func PreviousChainingTraceIDs(tokens []Token) []string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return PreviousChainingTraceIDsFromColors(colors)
}

// PreviousChainingTraceIDsFromColors returns canonical predecessor trace IDs
// from work-token colors while ignoring resource colors.
func PreviousChainingTraceIDsFromColors(colors []Color) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmpty(color.CurrentChainingTraceID, color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

// CurrentChainingTraceID returns the first customer work trace, falling back
// to any non-resource trace. ignoredWorkTypeIDs identify system-owned inputs.
func CurrentChainingTraceID(tokens []Token, ignoredWorkTypeIDs ...string) string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return CurrentChainingTraceIDFromColors(colors, ignoredWorkTypeIDs...)
}

// CurrentChainingTraceIDFromColors returns the first customer work trace,
// falling back to any non-resource trace.
func CurrentChainingTraceIDFromColors(colors []Color, ignoredWorkTypeIDs ...string) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || contains(ignoredWorkTypeIDs, color.WorkTypeID) {
			continue
		}
		return firstNonEmpty(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmpty(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

// ChainingTraceDepthFromColors derives the next chaining depth from the deepest
// non-resource input.
func ChainingTraceDepthFromColors(colors []Color) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		candidate := color.ChainingTraceDepth
		if candidate == 0 && firstNonEmpty(color.CurrentChainingTraceID, color.TraceID) != "" {
			candidate = 1
		}
		if candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
