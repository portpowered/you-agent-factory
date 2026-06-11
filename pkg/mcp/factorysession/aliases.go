package factorysession

// DiscoverCompatibilityAliases returns workflow-named MCP tool aliases that
// resolve to canonical Factory Session tool implementations.
func DiscoverCompatibilityAliases() []CompatibilityAlias {
	return []CompatibilityAlias{
		{
			Name:          ToolWorkflowValidate,
			CanonicalName: ToolValidateSource,
			Description: "Compatibility-only alias for you.factory_session.validate_source. " +
				"Uses the same Factory preview validation contract and response shape.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowRun,
			CanonicalName: ToolStartSync,
			Description: "Compatibility-only alias for you.factory_session.start_sync. " +
				"Uses the same sync Factory Session start contract and response shape.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowStatus,
			CanonicalName: ToolGetSession,
			Description: "Compatibility-only alias for you.factory_session.get. " +
				"Uses the same durable Factory Session status read model.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowResult,
			CanonicalName: ToolGetResult,
			Description: "Compatibility-only alias for you.factory_session.get_result. " +
				"Uses the same durable Factory Session result read contract.",
			CompatibilityOnly: true,
		},
		{
			Name:          ToolWorkflowArtifacts,
			CanonicalName: ToolListArtifacts,
			Description: "Compatibility-only alias for you.factory_session.list_artifacts. " +
				"Uses the same FactoryArtifact listing response shape.",
			CompatibilityOnly: true,
		},
	}
}

// ResolveToolName maps one workflow compatibility alias to its canonical Factory
// Session tool name. Unknown names pass through unchanged.
func ResolveToolName(name string) string {
	for _, alias := range DiscoverCompatibilityAliases() {
		if alias.Name == name {
			return alias.CanonicalName
		}
	}
	return name
}

// IsCompatibilityAlias reports whether name is a workflow-named compatibility alias.
func IsCompatibilityAlias(name string) bool {
	for _, alias := range DiscoverCompatibilityAliases() {
		if alias.Name == name {
			return true
		}
	}
	return false
}
