package local_inference_test

import (
	"strings"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

func cleanModelsEnvironment(home string) []string {
	environment := functionalHomeEnvironment(home)
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, runcli.ModelCacheDirEnvironment) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func (launcher *recordingModelHostLauncher) Active() bool {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.active
}
