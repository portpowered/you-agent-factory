// Package cli owns the Worker Sessions service CLI adapter.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// IDGenerator supplies caller-owned identities for CLI requests. Production
// composition selects the implementation in pkg/wire so the transport does
// not reach directly into an identity provider.
type IDGenerator func() string

// ExecutionFileReader supplies execution-document bytes for the invoke
// command. Production composition selects the filesystem implementation in
// pkg/wire; tests can replace it without touching the host filesystem.
type ExecutionFileReader func(string) ([]byte, error)

// Effects contains the external effects used while normalizing direct CLI
// requests. Continue uses only GenerateID; invoke uses both fields.
type Effects struct {
	GenerateID IDGenerator
	ReadFile   ExecutionFileReader
}

func selectEffects(effects []Effects) Effects {
	if len(effects) == 0 {
		return Effects{}
	}
	return effects[0]
}

func workerSessionConfirmationState(session factoryapi.WorkerSessionObservation) factoryapi.ConfirmationState {
	if session.ConfirmationState == factoryapi.CONFIRMED {
		return session.ConfirmationState
	}
	return factoryapi.UNCONFIRMED
}

const (
	// maxWorkerSessionExecutionStdinBytes is the inclusive byte limit for a
	// direct Worker execution document deliberately supplied through stdin.
	maxWorkerSessionExecutionStdinBytes = 1 * 1024 * 1024

	// maxWorkerSessionMessageStdinBytes is the inclusive byte limit for a
	// direct Worker message deliberately supplied through stdin by invoke,
	// continue, or interrupt.
	maxWorkerSessionMessageStdinBytes = 1 * 1024 * 1024
)

// readBoundedWorkerSessionStdin reads at most limit plus one byte. The extra
// byte is an overflow sentinel and is discarded when the inclusive limit is
// exceeded.
func readBoundedWorkerSessionStdin(stdin io.Reader, limit int, label, overflowGuidance string) ([]byte, error) {
	if stdin == nil {
		return nil, fmt.Errorf("read %s: process stdin reader is required", label)
	}
	data, err := io.ReadAll(io.LimitReader(stdin, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf(
			"%s exceeds the %d-byte limit; %s",
			label,
			limit,
			overflowGuidance,
		)
	}
	return data, nil
}

func readInvokeRequest(config InvokeConfig) (factoryapi.WorkerSessionStartRequest, error) {
	decoded, err := readInvokeRequestWithDiagnostics(config)
	return decoded.Request, err
}

func readInvokeRequestWithDiagnostics(config InvokeConfig) (invokeRequestDecodeResult, error) {
	input := strings.TrimSpace(config.ExecutionJSON)
	if input == "" {
		return invokeRequestDecodeResult{}, nil
	}
	var data []byte
	if input == "-" {
		if config.Stdin == nil {
			return invokeRequestDecodeResult{}, newCLIError("WORKER_SESSION_INPUT_MISSING", "--execution - requires JSON on stdin", nil)
		}
		var err error
		data, err = readBoundedWorkerSessionStdin(
			config.Stdin,
			maxWorkerSessionExecutionStdinBytes,
			"direct Worker execution stdin",
			"use --execution FILE for larger input",
		)
		if err != nil {
			return invokeRequestDecodeResult{}, newCLIError("WORKER_SESSION_INPUT_FAILED", fmt.Sprintf("failed to read direct Worker execution from stdin: %v", err), err)
		}
	} else if strings.HasPrefix(input, "{") {
		data = []byte(input)
	} else {
		var err error
		if config.ReadFile == nil {
			return invokeRequestDecodeResult{}, newCLIError("WORKER_SESSION_INPUT_FAILED", "direct Worker execution file reader is unavailable", nil)
		}
		data, err = config.ReadFile(input)
		if err != nil {
			return invokeRequestDecodeResult{}, newCLIError("WORKER_SESSION_INPUT_FAILED", "failed to read direct Worker execution file", err)
		}
	}
	document, err := readInvokeSingleDocument(data)
	if err != nil {
		return invokeRequestDecodeResult{}, err
	}
	decoded, err := httpcompat.DecodeBytes[factoryapi.WorkerSessionStartRequest](document)
	if err != nil {
		return invokeRequestDecodeResult{}, newCLIError("WORKER_SESSION_INPUT_INVALID", "direct Worker execution input is not valid JSON", err)
	}
	return invokeRequestDecodeResult{
		Request:          decoded.Value,
		IgnoredJSONPaths: decoded.Diagnostics.Paths(),
	}, nil
}

func readInvokeSingleDocument(data []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var document json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return nil, newCLIError("WORKER_SESSION_INPUT_INVALID", "direct Worker execution input is not valid JSON", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, newCLIError("WORKER_SESSION_INPUT_INVALID", "direct Worker execution input must contain exactly one JSON object", err)
	}
	return document, nil
}

func writeInvokeResultWithCompatibilityWarning(
	config InvokeConfig,
	jsonOutput bool,
	result invokeResult,
	synchronous bool,
	ignoredJSONPaths []string,
) error {
	if err := writeInvokeResult(config, jsonOutput, result, synchronous); err != nil {
		return err
	}
	writeInvokeCompatibilityWarning(config.Diagnostics, ignoredJSONPaths)
	return nil
}

func writeInvokeCompatibilityWarning(output io.Writer, ignoredJSONPaths []string) {
	paths := httpcompat.SortedUniquePaths(ignoredJSONPaths)
	if output == nil || len(paths) == 0 {
		return
	}
	_, _ = fmt.Fprintf(output, "warning: ignored unknown direct Worker execution fields at %s\n", strings.Join(paths, ", "))
}

// ListOperation is the composition-facing Worker Sessions list role.
type ListOperation func(ListConfig) error

// ShowOperation is the composition-facing Worker Sessions show role.
type ShowOperation func(ShowConfig) error

// ReadOperation is the composition-facing Worker Sessions transcript role.
type ReadOperation func(ReadConfig) error

// StreamOperation is the composition-facing Worker Sessions event stream role.
type StreamOperation func(StreamConfig) error

// BindList returns a list operation bound to one injected HTTP protocol.
func BindList(transport clihttp.Protocol) ListOperation {
	if transport == nil {
		return nil
	}
	return NewList(transport)
}

// BindShow returns a show operation bound to one injected HTTP protocol.
func BindShow(transport clihttp.Protocol) ShowOperation {
	if transport == nil {
		return nil
	}
	return NewShow(transport)
}

// BindRead returns a transcript operation bound to one injected HTTP protocol.
func BindRead(transport clihttp.Protocol) ReadOperation {
	if transport == nil {
		return nil
	}
	return NewRead(transport)
}

// BindStream returns a stream operation bound to one injected HTTP protocol.
func BindStream(transport clihttp.Protocol) StreamOperation {
	if transport == nil {
		return nil
	}
	return NewStream(transport)
}
