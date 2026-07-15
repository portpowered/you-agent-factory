package builtcliacceptance

import (
	"os"
	"path/filepath"
	"strings"
)

// ProcessEnvForIsolatedHome returns a child-process environment with HOME and
// Windows profile variables redirected to homeDir while preserving other entries.
func ProcessEnvForIsolatedHome(homeDir string) []string {
	env := make([]string, 0, len(os.Environ())+4)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH":
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
	return env
}
