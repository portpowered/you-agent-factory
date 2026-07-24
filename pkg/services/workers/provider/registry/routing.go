package registry

import (
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const legacyCursorRunnerID = workers.RunnerIDCursorCLI

type compatibilityAlias struct {
	alias     string
	canonical string
}

func providerCompatibilityAliases() []compatibilityAlias {
	return []compatibilityAlias{
		{alias: "anthropic", canonical: "claude"},
		{alias: legacyCursorRunnerID, canonical: "cursor"},
		{alias: "openai", canonical: "codex"},
	}
}

// ResolveRunnerSelection applies the existing runner precedence while using
// registry identity and selectability as the authority for every non-empty
// provider value.
func (r *Registry) ResolveRunnerSelection(
	workstationRunner string,
	factoryRunner string,
	workerModelProvider string,
) (workers.ResolvedRunnerSelection, error) {
	candidates := []struct {
		identity string
		source   workers.RunnerSelectionSource
	}{
		{workstationRunner, workers.RunnerSelectionSourceWorkstation},
		{factoryRunner, workers.RunnerSelectionSourceFactory},
		{workerModelProvider, workers.RunnerSelectionSourceLegacyProvider},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.identity) == "" {
			continue
		}
		if candidate.source == workers.RunnerSelectionSourceLegacyProvider &&
			isUnresolvedProviderTemplate(candidate.identity) {
			continue
		}
		runnerID, err := r.selectionRunnerID(candidate.identity)
		if err != nil {
			return workers.ResolvedRunnerSelection{}, err
		}
		return workers.ResolvedRunnerSelection{RunnerID: runnerID, Source: candidate.source}, nil
	}
	runnerID, err := r.RunnerID(workers.RunnerIDCodex)
	if err != nil {
		return workers.ResolvedRunnerSelection{}, fmt.Errorf("resolve default provider: %w", err)
	}
	return workers.ResolvedRunnerSelection{
		RunnerID: runnerID,
		Source:   workers.RunnerSelectionSourceDefault,
	}, nil
}

func isUnresolvedProviderTemplate(identity string) bool {
	trimmed := strings.TrimSpace(identity)
	return strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}")
}

func (r *Registry) selectionRunnerID(identity string) (string, error) {
	entry, err := r.Lookup(identity)
	if err != nil {
		return "", err
	}
	canonical := string(entry.Identity())
	if canonical == "cursor" {
		return legacyCursorRunnerID, nil
	}
	return canonical, nil
}

// RunnerID resolves a provider canonical ID or alias to the stable native
// runner ID retained by the current execution path.
func (r *Registry) RunnerID(identity string) (string, error) {
	entry, err := r.Lookup(identity)
	if err != nil {
		return "", err
	}
	canonical := string(entry.Identity())
	if entry.Manifest().ImplementationAvailability != ImplementationBundled {
		return "", fmt.Errorf(
			"provider %q is not available through the provider-native runner path",
			canonical,
		)
	}
	if canonical == "cursor" {
		return legacyCursorRunnerID, nil
	}
	return canonical, nil
}

// RunnerMetadata projects manifest-authoritative execution capabilities onto
// the existing runner diagnostic contract.
func (r *Registry) RunnerMetadata(identity string) (workers.RunnerMetadata, error) {
	entry, err := r.Lookup(identity)
	if err != nil {
		return workers.RunnerMetadata{}, err
	}
	manifest := entry.Manifest()
	runnerID := string(entry.Identity())
	if runnerID == "cursor" {
		runnerID = legacyCursorRunnerID
	}
	execution := manifest.MaximumExecutionCapabilities
	return workers.RunnerMetadata{
		ID:          runnerID,
		DisplayName: manifest.DisplayName.Value,
		Capabilities: workers.RunnerCapabilities{
			Baseline: baselineRunnerCapabilities(execution),
			Optional: optionalRunnerCapabilities(execution, runnerID),
		},
	}, nil
}

// ValidateRunnerPrerequisites checks the manifest-declared executable
// prerequisites without executing a provider command.
func (r *Registry) ValidateRunnerPrerequisites(
	locator platformprocess.ExecutableLocator,
	identity string,
) error {
	entry, err := r.Lookup(identity)
	if err != nil {
		return err
	}
	if locator == nil {
		return fmt.Errorf("%s runner executable locator is required", entry.Manifest().DisplayName.Value)
	}
	for _, command := range entry.DiscoveryPrerequisites().ExecutableNames {
		if _, err := locator.LookPath(command); err != nil {
			return fmt.Errorf(
				"%s runner requires %q on PATH: %w",
				entry.Manifest().DisplayName.Value,
				command,
				err,
			)
		}
	}
	return nil
}

func baselineRunnerCapabilities(execution ExecutionCapabilities) []workers.RunnerBaselineCapability {
	capabilities := make([]workers.RunnerBaselineCapability, 0, 2)
	if execution.PromptSubmission {
		capabilities = append(capabilities, workers.RunnerBaselineCapabilityPromptSubmission)
	}
	if execution.ToolExecution {
		capabilities = append(capabilities, workers.RunnerBaselineCapabilityToolExecution)
	}
	return capabilities
}

func optionalRunnerCapabilities(
	execution ExecutionCapabilities,
	runnerID string,
) []workers.RunnerOptionalCapabilitySupport {
	values := []struct {
		capability workers.RunnerOptionalCapability
		supported  bool
	}{
		{workers.RunnerOptionalCapabilityImageInput, execution.ImageInput},
		{workers.RunnerOptionalCapabilitySessionResume, execution.SessionResume},
		{workers.RunnerOptionalCapabilityStructuredOutput, execution.StructuredOutput},
		{workers.RunnerOptionalCapabilityWorkingDirectory, execution.WorkingDirectory},
		{workers.RunnerOptionalCapabilityWorktree, execution.Worktree},
	}
	result := make([]workers.RunnerOptionalCapabilitySupport, 0, len(values))
	for _, value := range values {
		status := workers.RunnerOptionalCapabilityStatusUnsupported
		if value.supported {
			status = workers.RunnerOptionalCapabilityStatusSupported
		}
		support := workers.RunnerOptionalCapabilitySupport{
			Capability: value.capability,
			Status:     status,
		}
		if runnerID == workers.RunnerIDCodex &&
			value.capability == workers.RunnerOptionalCapabilityWorktree {
			support.Detail = "factory-managed git worktree preparation under the factory root"
		}
		result = append(result, support)
	}
	return result
}
