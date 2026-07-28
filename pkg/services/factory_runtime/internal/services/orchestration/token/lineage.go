package token

import workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

// PreviousChainingTraceIDs returns canonical predecessor trace IDs from work
// tokens while ignoring resource tokens.
func PreviousChainingTraceIDs(tokens []Token) []string {
	return workerexecution.PreviousChainingTraceIDs(tokens)
}

// PreviousChainingTraceIDsFromColors returns canonical predecessor trace IDs
// from work-token colors while ignoring resource colors.
func PreviousChainingTraceIDsFromColors(colors []Color) []string {
	return workerexecution.PreviousChainingTraceIDsFromColors(colors)
}

// CurrentChainingTraceID returns the first customer work trace, falling back
// to any non-resource trace. ignoredWorkTypeIDs identify system-owned inputs.
func CurrentChainingTraceID(tokens []Token, ignoredWorkTypeIDs ...string) string {
	return workerexecution.CurrentChainingTraceID(tokens, ignoredWorkTypeIDs...)
}

// CurrentChainingTraceIDFromColors returns the first customer work trace,
// falling back to any non-resource trace.
func CurrentChainingTraceIDFromColors(colors []Color, ignoredWorkTypeIDs ...string) string {
	return workerexecution.CurrentChainingTraceIDFromColors(colors, ignoredWorkTypeIDs...)
}

// ChainingTraceDepthFromColors derives the next chaining depth from the deepest
// non-resource input.
func ChainingTraceDepthFromColors(colors []Color) int {
	return workerexecution.ChainingTraceDepthFromColors(colors)
}
