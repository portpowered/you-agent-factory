// Package cli owns the Worker Sessions service CLI adapter.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	httpcompat "github.com/portpowered/infinite-you/pkg/transports/http/compat"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func invokeCaller(config InvokeConfig) (*workersessions.CallerIdentity, error) {
	headers := make(http.Header)
	if config.Caller != nil {
		headers.Set("X-You-Worker-Session-Id", config.Caller.WorkerSessionID)
		headers.Set("Authorization", "Bearer "+config.Caller.Token)
	} else if config.LookupEnv != nil {
		if id, present := config.LookupEnv("YOU_WORKER_SESSION_ID"); present {
			headers.Set("X-You-Worker-Session-Id", id)
		}
		if token, present := config.LookupEnv("YOU_WORKER_SESSION_TOKEN"); present {
			headers.Set("Authorization", "Bearer "+token)
		}
	}
	return apisurface.WorkerSessionCallerFromHeaders(headers)
}

// Transport failures can quote credentials supplied to the HTTP effect. Keep
// the typed diagnostic while dropping any secret-bearing cause before rendering.
func sanitizeInvokeCallerError(caller *workersessions.CallerIdentity, err error) error {
	if caller == nil || caller.Token == "" || err == nil {
		return err
	}
	redact := func(value string) string { return strings.ReplaceAll(value, caller.Token, "[REDACTED]") }
	var typed *CLIError
	if errors.As(err, &typed) {
		safe := *typed
		safe.Code = redact(typed.Code)
		safe.Message = redact(typed.Message)
		if typed.Cause != nil && strings.Contains(typed.Cause.Error(), caller.Token) {
			safe.Cause = errors.New(redact(typed.Cause.Error()))
		}
		return &safe
	}
	if strings.Contains(err.Error(), caller.Token) {
		return errors.New(redact(err.Error()))
	}
	return err
}

// IDGenerator supplies caller-owned identities for CLI requests. Production
// composition selects the implementation in pkg/wire so the transport does
// not reach directly into an identity provider.
type IDGenerator func() string

// ExecutionFileReader supplies execution-document bytes for the invoke
// command. Production composition selects the filesystem implementation in
// pkg/wire; tests can replace it without touching the host filesystem.
type ExecutionFileReader func(string) ([]byte, error)

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
func BindList(transport clihttp.Protocol, clock clihttp.Clock) ListOperation {
	if transport == nil || clock == nil {
		return nil
	}
	return NewList(transport, clock)
}

// BindShow returns a show operation bound to one injected HTTP protocol.
func BindShow(transport clihttp.Protocol) ShowOperation {
	if transport == nil {
		return nil
	}
	return NewShow(transport)
}

// BindRead binds finite reads and follow to their injected HTTP protocols.
func BindRead(transport, streaming clihttp.Protocol) ReadOperation {
	if transport == nil || streaming == nil {
		return nil
	}
	return NewRead(transport, streaming)
}

// BindStream returns a stream operation bound to one injected HTTP protocol.
func BindStream(transport clihttp.Protocol) StreamOperation {
	if transport == nil {
		return nil
	}
	return NewStream(transport)
}
