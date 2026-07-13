package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// RunConfig holds CLI inputs for one durable Factory Session synchronous execution.
type RunConfig struct {
	StartConfig
	ExecutionBackendConfig
	JSON               bool
	Output             io.Writer
	Service            factorysessionexecution.Service
	FixtureCatalogPath string
}

// RunSync normalizes CLI inputs, executes one synchronous durable Factory Session start
// through the shared execution service, and renders deterministic human or JSON output.
func RunSync(ctx context.Context, cfg RunConfig) error {
	if cfg.Output == nil {
		cfg.Output = defaultOutputWriter()
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
		var encoded []byte
		var marshalErr error
		if isSyncTimeoutOutcome(mapped) {
			availability, availabilityErr := syncResultAvailability(ctx, service, mapped.SessionId)
			if availabilityErr != nil {
				return writeRunError(cfg.Output, cfg.JSON, availabilityErr)
			}
			cancelOnTimeout := normalized.Wait != nil && normalized.Wait.CancelOnTimeout
			encoded, marshalErr = marshalSyncTimeoutJSON(mapped, normalized.RequestID, cancelOnTimeout, availability)
		} else {
			encoded, marshalErr = json.Marshal(mapped)
		}
		if marshalErr != nil {
			return fmt.Errorf("marshal sync run response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(cfg.Output, string(encoded))
		return err
	}
	cancelOnTimeout := normalized.Wait != nil && normalized.Wait.CancelOnTimeout
	return renderSyncRunHuman(cfg.Output, mapped, cancelOnTimeout)
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
	provider, err := normalizeExecutionProvider(cfg.Provider)
	if err != nil {
		return nil, err
	}
	if provider == factorysessionexecution.ExecutionProviderJavaScriptRuntime {
		projectRoot, err := resolveProjectRoot(cfg.ProjectRoot)
		if err != nil {
			return nil, err
		}
		persistence, err := factorysessionexecution.ProjectPersistence(projectRoot)
		if err != nil {
			return nil, err
		}
		return factorysessionexecution.NewExecutionService(provider, factorysessionexecution.ServiceConfig{
			ProjectRoot:       projectRoot,
			ChildExecutorMode: cfg.StartConfig.ChildExecutorMode,
			Persistence:       persistence,
		})
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

func normalizeExecutionProvider(provider string) (factorysessionexecution.ExecutionProvider, error) {
	switch strings.TrimSpace(provider) {
	case "", string(factorysessionexecution.ExecutionProviderFake):
		return factorysessionexecution.ExecutionProviderFake, nil
	case string(factorysessionexecution.ExecutionProviderJavaScriptRuntime):
		return factorysessionexecution.ExecutionProviderJavaScriptRuntime, nil
	default:
		return "", newExecutionError(
			ErrorCodeValidation,
			fmt.Sprintf("execution provider %q is unsupported: use fake or javascript-runtime", provider),
			"executionProvider",
		)
	}
}

func resolveProjectRoot(explicit string) (string, error) {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	return cwd, nil
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

type syncTimeoutCLIResponse struct {
	factoryapi.FactorySessionSyncExecutionResponse
	RequestID          string `json:"requestId,omitempty"`
	CancelOnTimeout    bool   `json:"cancelOnTimeout,omitempty"`
	ResultAvailability string `json:"resultAvailability,omitempty"`
}

func isSyncTimeoutOutcome(result factoryapi.FactorySessionSyncExecutionResponse) bool {
	return result.SyncOutcome == factoryapi.FactorySessionSyncExecutionOutcomeTimedOut ||
		(result.TimedOut != nil && *result.TimedOut)
}

func syncResultAvailability(
	ctx context.Context,
	service factorysessionexecution.Service,
	sessionID string,
) (factoryapi.FactorySessionResultStatus, error) {
	result, err := service.GetResult(ctx, sessionID, factorysessionexecution.ResultRequest{
		Mode: factorysessionexecution.ResultModeFinal,
	})
	if err != nil {
		return "", err
	}
	return factoryapi.FactorySessionResultStatus(result.ResultStatus), nil
}

func marshalSyncTimeoutJSON(
	response factoryapi.FactorySessionSyncExecutionResponse,
	requestID string,
	cancelOnTimeout bool,
	resultAvailability factoryapi.FactorySessionResultStatus,
) ([]byte, error) {
	payload := syncTimeoutCLIResponse{
		FactorySessionSyncExecutionResponse: response,
	}
	if trimmed := strings.TrimSpace(requestID); trimmed != "" {
		payload.RequestID = trimmed
	}
	if cancelOnTimeout {
		payload.CancelOnTimeout = true
	}
	if resultAvailability != "" {
		payload.ResultAvailability = string(resultAvailability)
	}
	return json.Marshal(payload)
}

func renderSyncRunHuman(
	output io.Writer,
	result factoryapi.FactorySessionSyncExecutionResponse,
	cancelOnTimeout bool,
) error {
	if err := writeSyncRunOutcomeHeader(output, result); err != nil {
		return err
	}
	if isSyncTimeoutOutcome(result) {
		if err := writeSyncTimeoutHumanDetails(output, result, cancelOnTimeout); err != nil {
			return err
		}
	}
	if err := writeResolvedSourceHuman(output, result.SourceHash, result.ResolvedSource); err != nil {
		return err
	}
	if !isSyncTimeoutOutcome(result) {
		if summary := primaryResultSummary(result.Result); summary != "" {
			if _, err := fmt.Fprintf(output, "Primary result: %s\n", summary); err != nil {
				return err
			}
		}
	}
	if err := writeExecutionLinksHuman(output, result.Links); err != nil {
		return err
	}
	if isSyncTimeoutOutcome(result) {
		if _, err := fmt.Fprintf(
			output,
			"Follow-up: you workflow status %s\n",
			result.SessionId,
		); err != nil {
			return err
		}
	}
	return nil
}

func writeSyncRunOutcomeHeader(output io.Writer, result factoryapi.FactorySessionSyncExecutionResponse) error {
	switch result.SyncOutcome {
	case factoryapi.FactorySessionSyncExecutionOutcomeCompleted:
		_, err := fmt.Fprintf(
			output,
			"Factory session %s completed (%s).\n",
			result.SessionId,
			result.Status,
		)
		return err
	case factoryapi.FactorySessionSyncExecutionOutcomeTimedOut:
		_, err := fmt.Fprintf(
			output,
			"Factory session %s timed out (%s).\n",
			result.SessionId,
			result.Status,
		)
		return err
	default:
		_, err := fmt.Fprintf(
			output,
			"Factory session %s finished with sync outcome %s (%s).\n",
			result.SessionId,
			result.SyncOutcome,
			result.Status,
		)
		return err
	}
}

func writeSyncTimeoutHumanDetails(
	output io.Writer,
	result factoryapi.FactorySessionSyncExecutionResponse,
	cancelOnTimeout bool,
) error {
	if cancelOnTimeout {
		if _, err := fmt.Fprintln(output, "Cancel on timeout: requested"); err != nil {
			return err
		}
	}
	if result.SessionCanceledByTimeout != nil && *result.SessionCanceledByTimeout {
		if _, err := fmt.Fprintln(output, "Session canceled by timeout: true"); err != nil {
			return err
		}
	}
	if result.TimedOut != nil && *result.TimedOut {
		if _, err := fmt.Fprintln(output, "Timed out: true"); err != nil {
			return err
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

func writeOptionalTrimmedLine(output io.Writer, label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "%s: %s\n", label, trimmed)
	return err
}

func writeResolvedSourceHuman(
	output io.Writer,
	sourceHash *string,
	resolvedSource factoryapi.FactorySessionResolvedSourceIdentity,
) error {
	if sourceHash != nil && strings.TrimSpace(*sourceHash) != "" {
		_, err := fmt.Fprintf(output, "Source hash: %s\n", strings.TrimSpace(*sourceHash))
		return err
	}
	if ref := resolvedSource.SourceRef; ref != nil {
		return writeOptionalTrimmedLine(output, "Source ref", *ref)
	}
	return nil
}

func writeExecutionLinksHuman(output io.Writer, links *factoryapi.FactorySessionExecutionLinks) error {
	if links == nil {
		return nil
	}
	if links.Status != nil {
		if err := writeOptionalTrimmedLine(output, "Status link", *links.Status); err != nil {
			return err
		}
	}
	if links.Session != nil {
		if err := writeOptionalTrimmedLine(output, "Session link", *links.Session); err != nil {
			return err
		}
	}
	if links.Results != nil {
		if err := writeOptionalTrimmedLine(output, "Results link", *links.Results); err != nil {
			return err
		}
	}
	return nil
}

func writeResultAvailabilityHuman(output io.Writer, availability *factoryapi.FactorySessionResultAvailabilityDetail) error {
	if availability == nil {
		return nil
	}
	if reason := availability.Reason; reason != nil {
		if err := writeOptionalTrimmedLine(output, "Availability reason", *reason); err != nil {
			return err
		}
	}
	if message := availability.Message; message != nil {
		if err := writeOptionalTrimmedLine(output, "Availability message", *message); err != nil {
			return err
		}
	}
	if retryable := availability.Retryable; retryable != nil && *retryable {
		if _, err := fmt.Fprintln(output, "Retryable: true"); err != nil {
			return err
		}
	}
	return nil
}

func writeResultFailureHuman(output io.Writer, failure *factoryapi.FailureDetail) error {
	if failure == nil {
		return nil
	}
	if reason := string(failure.Reason); reason != "" {
		if err := writeOptionalTrimmedLine(output, "Failure reason", reason); err != nil {
			return err
		}
	}
	if message := failure.Message; message != "" {
		if err := writeOptionalTrimmedLine(output, "Failure message", message); err != nil {
			return err
		}
	}
	return nil
}
