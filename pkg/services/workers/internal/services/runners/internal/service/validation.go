package service

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func validateMetadata(metadata workers.RunnerMetadata) error {
	if err := validateIdentity(metadata.ID); err != nil {
		return fmt.Errorf("identity: %w", err)
	}
	if strings.TrimSpace(metadata.DisplayName) == "" ||
		metadata.DisplayName != strings.TrimSpace(metadata.DisplayName) {
		return fmt.Errorf("display name must be non-empty without surrounding whitespace")
	}
	if err := validateBaselineCapabilities(metadata.Capabilities.Baseline); err != nil {
		return err
	}
	return validateOptionalCapabilities(metadata.Capabilities.Optional)
}

func validateBaselineCapabilities(
	capabilities []workers.RunnerBaselineCapability,
) error {
	required := map[workers.RunnerBaselineCapability]bool{
		workers.RunnerBaselineCapabilityPromptSubmission: false,
		workers.RunnerBaselineCapabilityToolExecution:    false,
	}
	for _, capability := range capabilities {
		seen, recognized := required[capability]
		if !recognized {
			return fmt.Errorf("unknown baseline capability %q", capability)
		}
		if seen {
			return fmt.Errorf("duplicate baseline capability %q", capability)
		}
		required[capability] = true
	}
	for capability, present := range required {
		if !present {
			return fmt.Errorf("missing baseline capability %q", capability)
		}
	}
	return nil
}

func validateOptionalCapabilities(
	capabilities []workers.RunnerOptionalCapabilitySupport,
) error {
	seen := make(map[workers.RunnerOptionalCapability]struct{}, len(capabilities))
	for _, support := range capabilities {
		if !knownOptionalCapability(support.Capability) {
			return fmt.Errorf("unknown optional capability %q", support.Capability)
		}
		if _, duplicate := seen[support.Capability]; duplicate {
			return fmt.Errorf("duplicate optional capability %q", support.Capability)
		}
		seen[support.Capability] = struct{}{}
		switch support.Status {
		case workers.RunnerOptionalCapabilityStatusSupported,
			workers.RunnerOptionalCapabilityStatusUnsupported:
		default:
			return fmt.Errorf(
				"optional capability %q has unknown status %q",
				support.Capability,
				support.Status,
			)
		}
		if support.Detail != strings.TrimSpace(support.Detail) {
			return fmt.Errorf(
				"optional capability %q detail has surrounding whitespace",
				support.Capability,
			)
		}
	}
	return nil
}

func knownOptionalCapability(capability workers.RunnerOptionalCapability) bool {
	switch capability {
	case workers.RunnerOptionalCapabilityImageInput,
		workers.RunnerOptionalCapabilitySessionResume,
		workers.RunnerOptionalCapabilityStructuredOutput,
		workers.RunnerOptionalCapabilityWorkingDirectory,
		workers.RunnerOptionalCapabilityWorktree:
		return true
	default:
		return false
	}
}
