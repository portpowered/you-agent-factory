package initializer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
)

// MCPOptions carries MCP-specific composition inputs beyond factory startup config.
type MCPOptions struct {
	FixtureCatalogPath string
	RuntimeBacked      bool
	ProjectRoot        string
}

// MCPConfig combines optional factory startup config with MCP serve options.
type MCPConfig struct {
	Factory *Config
	Options MCPOptions
}

// MCPTransport bundles initializer-produced domain services with the durable
// Factory Session execution service used by MCP tool handlers without
// constructing root FactoryService at the composition boundary.
type MCPTransport struct {
	Services         *Services
	SessionExecution factorysessionexecution.Service
}

// InitializeMCPTransport composes MCP tool dependencies through pkg/initializer.
func InitializeMCPTransport(ctx context.Context, cfg *MCPConfig) (*MCPTransport, error) {
	if cfg == nil {
		cfg = &MCPConfig{}
	}

	factoryCfg := cfg.Factory
	if factoryCfg == nil && cfg.Options.RuntimeBacked {
		projectRoot := strings.TrimSpace(cfg.Options.ProjectRoot)
		if projectRoot != "" {
			if _, err := os.Stat(filepath.Join(projectRoot, "factory.json")); err == nil {
				factoryCfg = &Config{Dir: projectRoot}
			}
		}
	}

	var services *Services
	if factoryCfg != nil && strings.TrimSpace(factoryCfg.Dir) != "" {
		var err error
		services, err = Initialize(ctx, factoryCfg)
		if err != nil {
			return nil, err
		}
	}

	sessionExecution, err := resolveMCPSessionExecution(cfg.Options)
	if err != nil {
		return nil, err
	}

	return &MCPTransport{
		Services:         services,
		SessionExecution: sessionExecution,
	}, nil
}

// SessionClient returns the MCP Factory Session client backed by the composed
// durable session execution service.
func (t *MCPTransport) SessionClient() *mcpfactorysession.Client {
	if t == nil || t.SessionExecution == nil {
		return mcpfactorysession.NewClient()
	}
	return mcpfactorysession.NewClientWithService(t.SessionExecution)
}

func resolveMCPSessionExecution(opts MCPOptions) (factorysessionexecution.Service, error) {
	if opts.RuntimeBacked {
		projectRoot, err := resolveMCPProjectRoot(opts.ProjectRoot)
		if err != nil {
			return nil, err
		}
		service, err := factorysessionexecution.NewExecutionService(
			factorysessionexecution.ExecutionProviderJavaScriptRuntime,
			factorysessionexecution.ServiceConfig{ProjectRoot: projectRoot},
		)
		if err != nil {
			return nil, fmt.Errorf("initialize runtime-backed execution service: %w", err)
		}
		return service, nil
	}

	catalogPath, err := resolveMCPFixtureCatalogPath(opts.FixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return service, nil
}

func resolveMCPProjectRoot(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	return cwd, nil
}

func resolveMCPFixtureCatalogPath(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	relative := filepath.FromSlash(fixtures.ContractFixtureCatalogRelativePath)
	dir := cwd
	for {
		candidate := filepath.Join(dir, relative)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf(
		"fixture catalog not found; run from the repository root or set --fixture-catalog to %s",
		fixtures.ContractFixtureCatalogRelativePath,
	)
}
