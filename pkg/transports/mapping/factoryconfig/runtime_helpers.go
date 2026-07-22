package factoryconfig

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func validateOpenCodeAgentField(path, agent string) error {
	if strings.TrimSpace(agent) == "" {
		return fmt.Errorf("%s.openCodeAgent must be a non-empty string", path)
	}
	return nil
}

func mergeStopWords(base []string, extra []string) []string {
	if len(base) == 0 {
		return append([]string(nil), extra...)
	}
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(base)+len(extra))
	for _, stopWord := range base {
		seen[stopWord] = true
	}
	for _, stopWord := range extra {
		if seen[stopWord] {
			continue
		}
		out = append(out, stopWord)
		seen[stopWord] = true
	}
	return out
}

func normalizeCanonicalWorkstationRuntime(
	workstation *interfaces.FactoryWorkstationConfig,
) {
	if workstation == nil {
		return
	}
	normalizeWorkstationTaxonomyKind(workstation)
	if workstation.PromptTemplate == "" {
		workstation.PromptTemplate = workstation.Body
	}
	NormalizeWorkstationExecutionLimit(workstation)
}

func normalizeWorkstationTaxonomyKind(
	workstation *interfaces.FactoryWorkstationConfig,
) {
	if workstation == nil {
		return
	}
	if interfaces.StrictPublicFactoryWorkstationType(workstation.Type) !=
		interfaces.WorkstationTypePoller {
		return
	}
	switch workstation.Kind {
	case "", interfaces.WorkstationKindStandard:
		workstation.Kind = interfaces.WorkstationKindPoller
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func NormalizeWorkstationExecutionLimit(
	cfg *interfaces.FactoryWorkstationConfig,
) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.Limits.MaxExecutionTime) == "" &&
		strings.TrimSpace(cfg.Timeout) != "" {
		cfg.Limits.MaxExecutionTime = cfg.Timeout
	}
	cfg.Timeout = ""
}
