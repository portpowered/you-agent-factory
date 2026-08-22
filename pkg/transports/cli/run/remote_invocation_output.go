package run

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func remoteDurableLifecycleStatus(status *factoryapi.FactorySessionDurableLifecycleStatus) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func writeRemoteInvocationResult(cfg RunConfig, result apisurface.FactoryInvocationResult) error {
	if cfg.Output == nil {
		cfg.Output = cfg.StartupOutput
	}
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		if cfg.JSONOutput {
			if err := writeRemoteInvocationNDJSON(cfg.Output, result); err != nil {
				return err
			}
			if result.Status != interfaces.InvocationTerminalStatusCompleted {
				return invocationResultFailure(result)
			}
			return nil
		}
		if result.Status != interfaces.InvocationTerminalStatusCompleted {
			if err := writeRemoteInvocationHumanFailure(cfg.Output, result); err != nil {
				return err
			}

			return invocationResultFailure(result)
		}
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return writeInvocationFailure(cfg, result, nil)
	}
	return writeInvocationSuccess(cfg, result, nil)
}

type remoteInvocationNDJSONRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}

func writeRemoteInvocationNDJSON(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation response stream: process output is required")
	}
	encoded, err := json.Marshal(remoteInvocationNDJSONRecord{
		RecordType: "invocation_result",
		Response:   apisurface.InvocationResponseFromResult(result),
	})
	if err != nil {
		return fmt.Errorf("marshal remote invocation terminal record: %w", err)
	}
	_, err = fmt.Fprintln(output, string(encoded))
	return err
}

func writeRemoteInvocationHumanFailure(output io.Writer, result apisurface.FactoryInvocationResult) error {
	if output == nil {
		return fmt.Errorf("write remote invocation outcome: process output is required")
	}
	if _, err := fmt.Fprintln(output, "--- invocation outcome ---"); err != nil {
		return err
	}
	lines := []string{"status: " + string(result.Status)}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}
