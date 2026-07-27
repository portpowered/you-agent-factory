package script

import (
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// inheritedExecutionEnvironmentKeys are the only process-environment entries
// preserved when a Factory declares a bounded script environment.
var inheritedExecutionEnvironmentKeys = map[string]struct{}{
	"HOME":        {},
	"HOMEDRIVE":   {},
	"HOMEPATH":    {},
	"PATH":        {},
	"PATHEXT":     {},
	"SYSTEMROOT":  {},
	"TEMP":        {},
	"TMP":         {},
	"TMPDIR":      {},
	"USERPROFILE": {},
	"WINDIR":      {},
}

func mergedEnvironment(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return platformprocess.MergeCommandEnv(
			base,
			platformprocess.CommandEnvEntriesFromMap(overrides),
		)
	}
	return platformprocess.MergeCommandEnv(
		filterInheritedExecutionEnvironment(base),
		platformprocess.CommandEnvEntriesFromMap(overrides),
	)
}

func filterInheritedExecutionEnvironment(base []string) []string {
	if len(base) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(inheritedExecutionEnvironmentKeys))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		if _, allow := inheritedExecutionEnvironmentKeys[strings.ToUpper(name)]; allow {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
