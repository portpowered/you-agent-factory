package factorysession

// IsCanonicalToolHandlerRegistered reports whether the live CallTool path
// registers a handler for one canonical Factory Session tool name.
func IsCanonicalToolHandlerRegistered(name string) bool {
	switch name {
	case ToolListSessions,
		ToolValidateSource,
		ToolStartSync,
		ToolStartAsync,
		ToolGetSession,
		ToolGetResult,
		ToolListDispatches,
		ToolListArtifacts,
		ToolControl,
		ToolReadEvents:
		return true
	default:
		return false
	}
}
