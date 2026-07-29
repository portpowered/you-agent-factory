// Package service implements the packaged Providers catalog.
package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattn/go-shellwords"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	builtins "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/builtins"
)

type catalogDocument struct {
	ACP []catalogACPIntegration `json:"acp"`
}

type catalogACPIntegration struct {
	Name      string   `json:"name"`
	Aliases   []string `json:"aliases,omitempty"`
	Transport string   `json:"transport"`
	Command   string   `json:"command"`
}

type Service struct {
	integrations []providers.ACPIntegration
}

var _ builtins.Service = (*Service)(nil)

func New(document []byte) (builtins.Service, error) {
	var decoded catalogDocument
	if err := json.Unmarshal(document, &decoded); err != nil {
		return nil, fmt.Errorf("decode packaged provider catalog: %w", err)
	}
	seen := make(map[providers.ID]struct{}, len(decoded.ACP))
	integrations := make([]providers.ACPIntegration, 0, len(decoded.ACP))
	for index, entry := range decoded.ACP {
		name := providers.ID(strings.TrimSpace(entry.Name))
		if err := name.Validate(); err != nil {
			return nil, fmt.Errorf("packaged ACP integration %d: %w", index, err)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("packaged ACP integration %q is duplicated", name)
		}
		seen[name] = struct{}{}
		if strings.ToLower(strings.TrimSpace(entry.Transport)) != "stdio" {
			return nil, fmt.Errorf("packaged ACP integration %q has unsupported transport %q", name, entry.Transport)
		}
		if argv, err := shellwords.Parse(strings.TrimSpace(entry.Command)); err != nil || len(argv) == 0 {
			return nil, fmt.Errorf("packaged ACP integration %q has invalid command", name)
		}
		aliases := make([]string, 0, len(entry.Aliases))
		for _, rawAlias := range entry.Aliases {
			alias := strings.TrimSpace(rawAlias)
			if err := providers.ID(alias).Validate(); err != nil {
				return nil, fmt.Errorf("packaged ACP integration %q alias: %w", name, err)
			}
			aliases = append(aliases, alias)
		}
		integrations = append(integrations, providers.ACPIntegration{
			ID:        string(name),
			Name:      name,
			Aliases:   aliases,
			Transport: "stdio",
			Command:   strings.TrimSpace(entry.Command),
		})
	}
	return &Service{integrations: integrations}, nil
}

func (service *Service) ACPIntegrations() []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, len(service.integrations))
	for index, integration := range service.integrations {
		result[index] = integration.Clone()
	}
	return result
}
