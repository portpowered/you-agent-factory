package workersessionmcp

// ToolHandlerBinding is the non-executable identity of a Worker Session handler.
type ToolHandlerBinding struct {
	ToolID    string
	HandlerID string
}

// ProjectCanonicalToolHandlerBindings returns all adapter-owned dispatch identities.
func ProjectCanonicalToolHandlerBindings() []ToolHandlerBinding {
	return []ToolHandlerBinding{
		{ToolID: "mcp.tool.you.worker_session.list", HandlerID: "mcp.handler.you.worker_session.list"},
		{ToolID: "mcp.tool.you.worker_session.read", HandlerID: "mcp.handler.you.worker_session.read"},
		{ToolID: "mcp.tool.you.worker_session.control", HandlerID: "mcp.handler.you.worker_session.control"},
	}
}
