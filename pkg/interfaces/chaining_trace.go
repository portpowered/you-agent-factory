package interfaces

import "sort"

// CanonicalChainingTraceIDs applies the shared chaining-trace fan-in rule:
// keep only non-empty predecessor chain IDs, dedupe them, and sort them.
func CanonicalChainingTraceIDs(traceIDs []string) []string {
	if len(traceIDs) == 0 {
		return nil
	}

	ordered := append([]string(nil), traceIDs...)
	sort.Strings(ordered)

	deduped := ordered[:0]
	var previous string
	for i, traceID := range ordered {
		if traceID == "" || (i > 0 && traceID == previous) {
			previous = traceID
			continue
		}
		deduped = append(deduped, traceID)
		previous = traceID
	}
	if len(deduped) == 0 {
		return nil
	}
	return deduped
}

// PreviousChainingTraceIDsFromTokens collects predecessor chain IDs from
// non-resource input tokens using the canonical fan-in ordering rule.
func PreviousChainingTraceIDsFromTokens(tokens []Token) []string {
	traceIDs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// PreviousChainingTraceIDsFromTokenColors collects predecessor chain IDs from
// non-resource token colors using the canonical fan-in ordering rule.
func PreviousChainingTraceIDsFromTokenColors(colors []TokenColor) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// PreviousChainingTraceIDsFromWorkItems collects predecessor chain IDs from
// canonical work items using the shared deterministic fan-in rule.
func PreviousChainingTraceIDsFromWorkItems(items []FactoryWorkItem) []string {
	traceIDs := make([]string, 0, len(items))
	for _, item := range items {
		traceIDs = append(traceIDs, firstNonEmptyString(item.CurrentChainingTraceID, item.TraceID))
	}
	return CanonicalChainingTraceIDs(traceIDs)
}

// CurrentChainingTraceIDFromWorkItems resolves the current dispatch chain from
// the first non-system customer work item, falling back to any work item when
// only system work is present.
func CurrentChainingTraceIDFromWorkItems(items []FactoryWorkItem) string {
	for _, item := range items {
		if IsSystemTimeWorkType(item.WorkTypeID) {
			continue
		}
		if item.CurrentChainingTraceID != "" {
			return item.CurrentChainingTraceID
		}
		if item.TraceID != "" {
			return item.TraceID
		}
	}
	for _, item := range items {
		if item.CurrentChainingTraceID != "" {
			return item.CurrentChainingTraceID
		}
		if item.TraceID != "" {
			return item.TraceID
		}
	}
	return ""
}

// CurrentChainingTraceIDFromTokens resolves the current dispatch chain from
// the first non-resource customer work token, falling back to any non-resource
// token when only system work is present.
func CurrentChainingTraceIDFromTokens(tokens []Token) string {
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource || token.Color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
	}
	for _, token := range tokens {
		if token.Color.DataType != DataTypeResource {
			return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
		}
	}
	return ""
}

// ChainingTraceDepthForTokenColor returns the stored chain depth for one token
// color, falling back to depth 1 when only a trace identifier is available.
func ChainingTraceDepthForTokenColor(color TokenColor) int {
	if color.ChainingTraceDepth > 0 {
		return color.ChainingTraceDepth
	}
	if firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID) != "" {
		return 1
	}
	return 0
}

// ChainingTraceDepthForWorkItem returns the stored chain depth for one work
// item, falling back to depth 1 when only a trace identifier is available.
func ChainingTraceDepthForWorkItem(item FactoryWorkItem) int {
	if item.ChainingTraceDepth > 0 {
		return item.ChainingTraceDepth
	}
	if firstNonEmptyString(item.CurrentChainingTraceID, item.TraceID) != "" {
		return 1
	}
	return 0
}

// ChainingTraceDepthFromTokenColors resolves the next chain depth from the
// deepest non-resource token color, defaulting initial traced work to depth 1.
func ChainingTraceDepthFromTokenColors(colors []TokenColor) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		if candidate := ChainingTraceDepthForTokenColor(color); candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	if CurrentChainingTraceIDFromTokenColors(colors) != "" {
		return 1
	}
	return 0
}

func CurrentChainingTraceIDFromTokenColors(colors []TokenColor) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
