// Package commandenv contains the retained provider-adapter environment
// compatibility policy. The active Providers adapters use their own package.
package commandenv

import workerprocess "github.com/portpowered/infinite-you/pkg/platform/process"

var automationDefaults = []workerprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

func Build(processEnvironment []string, envVars map[string]string) []string {
	return workerprocess.MergeCommandEnv(
		processEnvironment,
		workerprocess.CommandEnvEntriesFromMap(envVars),
		automationDefaults,
	)
}

func AutomationDefaults() []workerprocess.CommandEnvEntry {
	return append([]workerprocess.CommandEnvEntry(nil), automationDefaults...)
}
