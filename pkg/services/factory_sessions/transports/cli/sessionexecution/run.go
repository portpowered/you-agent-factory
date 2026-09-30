package sessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

// DirectJavaScriptLifecyclePlan pairs one CLI completion with its optional
// process HTTP transport. Factory Sessions owns the shutdown order.
func DirectJavaScriptLifecyclePlan(transport lifecycle.Component, completion func(context.Context) error) (lifecycle.Plan, error) {
	return processlifecycle.BuildDirectJavaScriptLifecyclePlan(transport, completion)
}

// RunCanonicalSync presents the process-owned synchronous Factory Session
// result without opening a second durable execution service.
func RunCanonicalSync(
	ctx context.Context,
	service factorysessionexecution.Service,
	request factorysessionexecution.SessionStartRequest,
	jsonOutput bool,
	output io.Writer,
) error {
	if output == nil {
		return fmt.Errorf("workflow run output is required")
	}
	if service == nil {
		return writeRunError(output, jsonOutput, fmt.Errorf("Factory Sessions service is required"))
	}
	result, err := service.Start(ctx, request)
	if err != nil {
		return writeRunError(output, jsonOutput, err)
	}
	if result.Sync == nil {
		return writeRunError(output, jsonOutput, fmt.Errorf("synchronous Factory Session result is required"))
	}
	mapped := factorysession.SyncStartResponseToAPI(*result.Sync)
	if jsonOutput {
		var encoded []byte
		var marshalErr error
		if isSyncTimeoutOutcome(mapped) {
			availability, availabilityErr := service.ReadResult(ctx, factorysessionexecution.SessionResultReadRequest{
				SessionID: mapped.SessionId,
				Mode:      factorysessionexecution.SessionOperationModeDurable,
				Request:   factorysessionexecution.ResultRequest{Mode: factorysessionexecution.ResultModeFinal},
			})
			if availabilityErr != nil {
				return writeRunError(output, jsonOutput, availabilityErr)
			}
			if availability.Durable == nil {
				return writeRunError(output, jsonOutput, fmt.Errorf("durable Factory Session result is required"))
			}
			encoded, marshalErr = marshalSyncTimeoutJSON(mapped, request.Correlation.RequestID, request.Wait.CancelOnTimeout, factoryapi.FactorySessionResultStatus(availability.Durable.Status))
		} else {
			encoded, marshalErr = json.Marshal(mapped)
		}
		if marshalErr != nil {
			return fmt.Errorf("marshal sync run response: %w", marshalErr)
		}
		_, err = fmt.Fprintln(output, string(encoded))
		return err
	}
	return renderSyncRunHuman(output, mapped, request.Wait.CancelOnTimeout)
}

func writeRunError(output io.Writer, jsonOutput bool, err error) error {
	if writeExecutionError(output, err, jsonOutput) {
		return err
	}
	_, _ = fmt.Fprintln(output, err.Error())
	return err
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
			"Follow-up: you session show %s\n",
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
