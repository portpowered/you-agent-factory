package smoke

import (
	"os"
	"strings"
)

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// namedFactorySmokeEnvironment gives spawned CLIs one unambiguous home across
// operating systems. Windows resolves USERPROFILE instead of HOME.
func namedFactorySmokeEnvironment(homeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "HOME") || strings.EqualFold(name, "USERPROFILE") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, "HOME="+homeDir, "USERPROFILE="+homeDir)
}
