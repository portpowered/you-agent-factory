package workers

import "github.com/portpowered/infinite-you/pkg/services/work"

func PreviousChainingTraceIDs(tokens []Token) []string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return PreviousChainingTraceIDsFromColors(colors)
}

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

func CurrentChainingTraceID(tokens []Token, ignoredWorkTypeIDs ...string) string {
	colors := make([]Color, len(tokens))
	for i := range tokens {
		colors[i] = tokens[i].Color
	}
	return CurrentChainingTraceIDFromColors(colors, ignoredWorkTypeIDs...)
}

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
