// Package service implements the packaged Providers catalog.
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattn/go-shellwords"
	"github.com/portpowered/infinite-you/internal/providerprofiles"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	builtins "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins"
)

type catalogDocument struct {
	ACP []catalogACPIntegration `json:"acp"`
}

type catalogACPIntegration struct {
	Name           string                `json:"name"`
	Aliases        []string              `json:"aliases,omitempty"`
	Transport      string                `json:"transport"`
	Command        string                `json:"command"`
	Arguments      []string              `json:"arguments,omitempty"`
	Posture        string                `json:"posture"`
	Implementation runtimeImplementation `json:"implementation"`
}

type runtimeImplementation struct {
	Kind    string `json:"kind"`
	Profile string `json:"profile"`
}

var registeredRuntimeProfiles = func() map[string]struct{} {
	profiles := providerprofiles.RegisteredACPProfileIDs()
	result := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		result[profile] = struct{}{}
	}
	return result
}()

type Service struct {
	integrations []providers.ACPIntegration
}

var _ builtins.Service = (*Service)(nil)

func New(document []byte) (builtins.Service, error) {
	var decoded catalogDocument
	if err := json.Unmarshal(document, &decoded); err != nil {
		return nil, fmt.Errorf("decode packaged provider catalog: %w", err)
	}
	seen := make(map[string]string, len(decoded.ACP))
	integrations := make([]providers.ACPIntegration, 0, len(decoded.ACP))
	for index, entry := range decoded.ACP {
		integration, err := decodeIntegration(index, entry, seen)
		if err != nil {
			return nil, err
		}
		integrations = append(integrations, integration)
	}
	return &Service{integrations: integrations}, nil
}

func decodeIntegration(index int, entry catalogACPIntegration, seen map[string]string) (providers.ACPIntegration, error) {
	name := providers.ID(strings.TrimSpace(entry.Name))
	if err := name.Validate(); err != nil {
		return providers.ACPIntegration{}, fmt.Errorf("packaged ACP integration %d: %w", index, err)
	}
	if err := claimIdentity(name, seen); err != nil {
		return providers.ACPIntegration{}, err
	}
	posture, profile, err := validateRuntimeLaunch(name, entry)
	if err != nil {
		return providers.ACPIntegration{}, err
	}
	aliases, err := parseAliases(name, entry.Aliases, seen)
	if err != nil {
		return providers.ACPIntegration{}, err
	}
	return providers.ACPIntegration{
		ID:                    string(name),
		Name:                  name,
		Aliases:               aliases,
		Transport:             "stdio",
		Command:               strings.TrimSpace(entry.Command),
		Arguments:             append([]string(nil), entry.Arguments...),
		RuntimePosture:        posture,
		ImplementationProfile: profile,
	}, nil
}

func claimIdentity(name providers.ID, seen map[string]string) error {
	identityKey := strings.ToLower(name.String())
	if _, duplicate := seen[identityKey]; duplicate {
		return fmt.Errorf("packaged ACP integration %q is duplicated", name)
	}
	seen[identityKey] = name.String()
	return nil
}

func validateRuntimeLaunch(name providers.ID, entry catalogACPIntegration) (string, string, error) {
	if strings.ToLower(strings.TrimSpace(entry.Transport)) != "stdio" {
		return "", "", fmt.Errorf("packaged ACP integration %q has unsupported transport %q", name, entry.Transport)
	}
	argv, err := shellwords.Parse(strings.TrimSpace(entry.Command))
	if err != nil || len(argv) == 0 {
		return "", "", fmt.Errorf("packaged ACP integration %q has invalid command", name)
	}
	posture := strings.TrimSpace(entry.Posture)
	if posture != "bundled" && posture != "package_runner" && posture != "installed_executable" {
		return "", "", fmt.Errorf("packaged ACP integration %q has invalid launch posture %q", name, entry.Posture)
	}
	if entry.Implementation.Kind != providerprofiles.ACPAgentImplementationKind {
		return "", "", fmt.Errorf("packaged ACP integration %q has unsupported implementation kind %q", name, entry.Implementation.Kind)
	}
	profile := strings.TrimSpace(entry.Implementation.Profile)
	if profile == "" {
		return "", "", fmt.Errorf("packaged ACP integration %q has incomplete runtime binding", name)
	}
	if _, registered := registeredRuntimeProfiles[profile]; !registered {
		return "", "", fmt.Errorf("packaged ACP integration %q references unknown runtime profile %q", name, profile)
	}
	if !sameStrings(argv[1:], entry.Arguments) {
		return "", "", fmt.Errorf("packaged ACP integration %q command arguments drift from its runtime projection", name)
	}
	return posture, profile, nil
}

func parseAliases(name providers.ID, rawAliases []string, seen map[string]string) ([]string, error) {
	aliases := make([]string, 0, len(rawAliases))
	seenAliases := make(map[string]struct{}, len(rawAliases))
	for _, rawAlias := range rawAliases {
		alias := strings.TrimSpace(rawAlias)
		if err := providers.ID(alias).Validate(); err != nil {
			return nil, fmt.Errorf("packaged ACP integration %q alias: %w", name, err)
		}
		aliasKey := strings.ToLower(alias)
		if aliasKey == strings.ToLower(name.String()) {
			return nil, fmt.Errorf("packaged ACP integration %q alias duplicates its canonical identity", name)
		}
		if _, duplicate := seenAliases[aliasKey]; duplicate {
			return nil, fmt.Errorf("packaged ACP integration %q alias %q is duplicated", name, alias)
		}
		if owner, collision := seen[aliasKey]; collision {
			return nil, fmt.Errorf("packaged ACP integration %q alias %q collides with %q", name, alias, owner)
		}
		seenAliases[aliasKey] = struct{}{}
		seen[aliasKey] = name.String()
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (service *Service) ACPIntegrations() []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, len(service.integrations))
	for index, integration := range service.integrations {
		result[index] = integration.Clone()
	}
	return result
}
