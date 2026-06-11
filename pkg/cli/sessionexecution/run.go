package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
)

// RunConfig holds CLI inputs for one durable Factory Session synchronous execution.
type RunConfig struct {
	StartConfig
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunSync normalizes CLI inputs, executes one synchronous durable Factory Session start
// through the shared execution service, and renders deterministic human or JSON output.
func RunSync(ctx context.Context, cfg RunConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}

	normalized, mode, err := NormalizeStartRequest(cfg.StartConfig)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}
	if mode != ExecutionModeSync {
		return writeRunError(cfg.Output, cfg.JSON, newExecutionError(
			ErrorCodeUnsupportedMode,
			"workflow run requires sync execution mode",
			"mode",
		))
	}

	service, err := resolveExecutionService(cfg)
	if err != nil {
		return err
	}

	result, err := service.StartSync(ctx, normalized)
	if err != nil {
		return writeRunError(cfg.Output, cfg.JSON, err)
	}

	mapped := factorysession.SyncStartResponseToAPI(result)
	if cfg.JSON {
		encoded, marshalErr := json.Marshal(mapped)
		if marshalErr != nil {
			return fmt.Errorf("marshal sync run response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	return renderSyncRunHuman(cfg.Output, mapped)
}

func writeRunError(output io.Writer, jsonOutput bool, err error) error {
	if WriteExecutionError(output, err, jsonOutput) {
		return err
	}
	_, _ = fmt.Fprintln(output, err.Error())
	return err
}

func resolveExecutionService(cfg RunConfig) (factorysessionexecution.Service, error) {
	if cfg.Service != nil {
		return cfg.Service, nil
	}
	catalogPath, err := resolveFixtureCatalogPath(cfg.FixtureCatalogPath)
	if err != nil {
		return nil, err
	}
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		return nil, fmt.Errorf("load durable session fixture catalog: %w", err)
	}
	return service, nil
}

func resolveFixtureCatalogPath(explicit string) (string, error) {
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

func renderSyncRunHuman(output io.Writer, result factoryapi.FactorySessionSyncExecutionResponse) error {
	switch result.SyncOutcome {
	case factoryapi.FactorySessionSyncExecutionOutcomeCompleted:
		if _, err := fmt.Fprintf(
			output,
			"Factory session %s completed (%s).\n",
			result.SessionId,
			result.Status,
		); err != nil {
			return err
		}
	case factoryapi.FactorySessionSyncExecutionOutcomeTimedOut:
		if _, err := fmt.Fprintf(
			output,
			"Factory session %s timed out (%s).\n",
			result.SessionId,
			result.Status,
		); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintf(
			output,
			"Factory session %s finished with sync outcome %s (%s).\n",
			result.SessionId,
			result.SyncOutcome,
			result.Status,
		); err != nil {
			return err
		}
	}

	if result.SourceHash != nil && strings.TrimSpace(*result.SourceHash) != "" {
		if _, err := fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*result.SourceHash)); err != nil {
			return err
		}
	} else if ref := result.ResolvedSource.SourceRef; ref != nil && strings.TrimSpace(*ref) != "" {
		if _, err := fmt.Fprintf(output, "Source ref: %s\n", strings.TrimSpace(*ref)); err != nil {
			return err
		}
	}

	if summary := primaryResultSummary(result.Result); summary != "" {
		if _, err := fmt.Fprintf(output, "Primary result: %s\n", summary); err != nil {
			return err
		}
	}

	if result.Links != nil {
		if result.Links.Session != nil && strings.TrimSpace(*result.Links.Session) != "" {
			if _, err := fmt.Fprintf(output, "Session link: %s\n", strings.TrimSpace(*result.Links.Session)); err != nil {
				return err
			}
		}
		if result.Links.Results != nil && strings.TrimSpace(*result.Links.Results) != "" {
			if _, err := fmt.Fprintf(output, "Results link: %s\n", strings.TrimSpace(*result.Links.Results)); err != nil {
				return err
			}
		}
	}
	return nil
}

func primaryResultSummary(result *factoryapi.FactorySessionResult) string {
	if result == nil || result.PrimaryResult == nil {
		return ""
	}
	for _, part := range *result.PrimaryResult {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		if trimmed := strings.TrimSpace(textPart.Text); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
