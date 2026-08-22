package retiredboundary

// RetiredFieldAlias describes a field that is no longer part of the
// canonical Factory Definition contract and the guidance shown to authors
// when it is encountered.
type RetiredFieldAlias struct {
	Key         string
	Replacement string
}

// Field is the retired-field shape used by boundary-specific rejection rules.
type Field = RetiredFieldAlias

// RetiredFactoryFieldAliases returns the retired top-level Factory fields
// shared by authored and generated boundary validation.
func RetiredFactoryFieldAliases() []RetiredFieldAlias {
	return []RetiredFieldAlias{
		{Key: "project", Replacement: "use id"},
		{Key: "work_types", Replacement: "use workTypes"},
		{Key: "factoryDir", Replacement: "use factoryDirectory"},
		{Key: "factory_dir", Replacement: "use factoryDirectory"},
		{Key: "resourceManifest", Replacement: "use supportingFiles"},
		{Key: "resource_manifest", Replacement: "use supportingFiles"},
		{Key: "workflowId", Replacement: "remove workflowId"},
		{Key: "workflow_id", Replacement: "remove workflowId"},
	}
}

// RetiredWorkerFieldAliases returns retired worker fields shared by authored
// and generated boundary validation.
func RetiredWorkerFieldAliases() []RetiredFieldAlias {
	return []RetiredFieldAlias{
		{Key: "model_provider", Replacement: "use modelProvider"},
		{Key: "sessionId", Replacement: "remove sessionId; provider sessions are runtime-owned"},
		{Key: "session_id", Replacement: "remove sessionId; provider sessions are runtime-owned"},
		{Key: "concurrency", Replacement: "remove concurrency; use resources to limit concurrent work"},
	}
}

// RetiredWorkstationFieldAliases returns retired workstation fields shared by
// authored and generated boundary validation.
func RetiredWorkstationFieldAliases() []RetiredFieldAlias {
	return []RetiredFieldAlias{
		{Key: "kind", Replacement: "use behavior"},
		{Key: "runtimeType", Replacement: "use type"},
		{Key: "runtime_type", Replacement: "use type"},
		{Key: "resourceUsage", Replacement: "use resources"},
		{Key: "resource_usage", Replacement: "use resources"},
		{Key: "resource-usage", Replacement: "use resources"},
		{Key: "stopToken", Replacement: "use stopWords"},
		{Key: "stop_token", Replacement: "use stopWords"},
		{Key: "runtimeStopWords", Replacement: "use stopWords"},
		{Key: "runtime_stop_words", Replacement: "use stopWords"},
		{Key: "timeout", Replacement: "use limits.maxExecutionTime"},
	}
}

// RetiredCronFieldAliases returns retired cron fields shared by authored and
// generated boundary validation.
func RetiredCronFieldAliases() []RetiredFieldAlias {
	return []RetiredFieldAlias{
		{Key: "trigger_at_start", Replacement: "use triggerAtStart"},
		{Key: "expiry_window", Replacement: "use expiryWindow"},
	}
}
