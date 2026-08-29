package builtcliacceptance

import (
	"os"
	"path/filepath"
	"strings"
)

const browserOpenOptOutEnvironment = "YOU_NO_BROWSER_OPEN"

// ProcessEnvForIsolatedHome returns a child-process environment with HOME and
// Windows profile variables redirected to homeDir while preserving other
// entries and enforcing the canonical browser-open opt-out.
func ProcessEnvForIsolatedHome(homeDir string) []string {
	env := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch {
		case key == "HOME", key == "USERPROFILE", key == "HOMEDRIVE", key == "HOMEPATH":
			continue
		case strings.EqualFold(key, browserOpenOptOutEnvironment):
			continue
		}
		env = append(env, entry)
	}
	env = append(env,
		"HOME="+homeDir,
		"USERPROFILE="+homeDir,
		"HOMEDRIVE="+filepath.VolumeName(homeDir),
		"HOMEPATH="+string(os.PathSeparator),
	)
	return normalizeBrowserOpenEnvironment(env)
}

func normalizeBrowserOpenEnvironment(env []string) []string {
	normalized := make([]string, 0, len(env)+1)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, browserOpenOptOutEnvironment) {
			continue
		}
		normalized = append(normalized, entry)
	}
	return append(normalized, browserOpenOptOutEnvironment+"=1")
}
