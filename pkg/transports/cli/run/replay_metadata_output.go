package run

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func replayMetadataOutput(cfg RunConfig) io.Writer {
	if cfg.Output != nil {
		return cfg.Output
	}
	if cfg.ReplayMetadataOutput != nil {
		return cfg.ReplayMetadataOutput
	}
	return cfg.StartupOutput
}

func emitReplayMetadataWarnings(
	output io.Writer,
	warnings []recordings.MetadataMismatchWarning,
) error {
	if output == nil || len(warnings) == 0 {
		return nil
	}
	components := replayMetadataWarningComponents(warnings)
	if len(components) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(
		output,
		"Replay warning: current Factory Definition differs from the recording; affected components: %s. Replay continues with recorded inputs.\n",
		strings.Join(components, ", "),
	); err != nil {
		return fmt.Errorf("write replay drift warning: %w", err)
	}
	return nil
}

func replayMetadataWarningComponents(
	warnings []recordings.MetadataMismatchWarning,
) []string {
	seen := make(map[string]struct{}, len(warnings))
	components := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		component := replayMetadataWarningComponent(warning.Key)
		if component == "" {
			continue
		}
		if _, ok := seen[component]; ok {
			continue
		}
		seen[component] = struct{}{}
		components = append(components, component)
	}
	sort.Strings(components)
	return components
}

func replayMetadataWarningComponent(key string) string {
	switch key {
	case "factory_hash":
		return "Factory Definition"
	case "workers_hash":
		return "workers"
	case "workstations_hash":
		return "workstations"
	case "runtime_config_hash":
		return "runtime configuration"
	default:
		return strings.TrimSpace(key)
	}
}
