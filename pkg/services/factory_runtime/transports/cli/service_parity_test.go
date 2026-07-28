package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type stubInvocationCLIError struct {
	code    string
	message string
}

func (err stubInvocationCLIError) Error() string {
	return err.message
}

func (err stubInvocationCLIError) InvocationErrorCode() string {
	return err.code
}

func (err stubInvocationCLIError) InvocationErrorMessage() string {
	return err.message
}

type cancelingRuntimeRoot struct {
	stubRuntimeRoot
}

func (stub cancelingRuntimeRoot) Observe(
	ctx context.Context,
	req factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryruntime.ObserveResult{}, err
	}
	return stub.stubRuntimeRoot.Observe(ctx, req)
}

func assertRuntimeCLIParity(
	t *testing.T,
	runService func() (any, error),
	runPackage func() (any, error),
) {
	t.Helper()

	gotService, serviceErr := runService()
	gotPackage, packageErr := runPackage()

	if (serviceErr == nil) != (packageErr == nil) {
		t.Fatalf("service error = %v, package error = %v", serviceErr, packageErr)
	}
	if serviceErr != nil && packageErr != nil {
		var serviceInvocationErr, packageInvocationErr *factoryruntimecli.InvocationError
		if errors.As(serviceErr, &serviceInvocationErr) || errors.As(packageErr, &packageInvocationErr) {
			if !errors.As(serviceErr, &serviceInvocationErr) ||
				!errors.As(packageErr, &packageInvocationErr) {
				t.Fatalf("typed invocation error mismatch: service = %v, package = %v", serviceErr, packageErr)
			}
			if serviceInvocationErr.Code != packageInvocationErr.Code ||
				serviceInvocationErr.Message != packageInvocationErr.Message {
				t.Fatalf(
					"service error = %#v, package error = %#v",
					serviceInvocationErr,
					packageInvocationErr,
				)
			}
			return
		}
		if serviceErr.Error() != packageErr.Error() {
			t.Fatalf("service error = %q, package error = %q", serviceErr.Error(), packageErr.Error())
		}
		return
	}
	if fmt.Sprint(gotService) != fmt.Sprint(gotPackage) {
		t.Fatalf("service result = %#v, package result = %#v", gotService, gotPackage)
	}
}

func assertWriteInvocationErrorParity(t *testing.T, err error) {
	t.Helper()

	var serviceStderr, packageStderr bytes.Buffer
	serviceHandled := factoryruntimecli.WriteInvocationError(&serviceStderr, err, false)
	packageHandled := factoryruntimecli.WriteInvocationError(&packageStderr, err, false)
	if serviceHandled != packageHandled {
		t.Fatalf("handled = %v, want package handled = %v", serviceHandled, packageHandled)
	}
	if !serviceHandled {
		return
	}
	if serviceStderr.String() != packageStderr.String() {
		t.Fatalf("service stderr = %q, package stderr = %q", serviceStderr.String(), packageStderr.String())
	}
}

func TestConstructedService_MapCurrentFactoryFailureMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []struct {
		name       string
		err        error
		wantCode   factoryapi.ErrorResponseCode
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name:       "missing",
			err:        fmt.Errorf("load Current Factory: %w", fs.ErrNotExist),
			wantCode:   factoryruntimecli.CurrentFactoryNotFoundCode,
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "invalid",
			err:        errors.New("parse Current Factory: malformed JSON"),
			wantCode:   factoryruntimecli.CurrentFactoryInvalidCode,
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return nil, service.MapCurrentFactoryFailure(tc.err)
				},
				func() (any, error) {
					return nil, factoryruntimecli.MapCurrentFactoryFailure(tc.err)
				},
			)

			mapped := service.MapCurrentFactoryFailure(tc.err)
			var stderr bytes.Buffer
			if !factoryruntimecli.WriteInvocationError(&stderr, mapped, false) {
				t.Fatal("WriteInvocationError did not recognize Current Factory failure")
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &response); err != nil {
				t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
			}
			if response.Code != tc.wantCode || response.Family != tc.wantFamily {
				t.Fatalf("ErrorResponse = %#v", response)
			}
		})
	}
}

func TestConstructedService_MapServerFailureMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cause := &platformhttpserver.BindError{
		Host: "127.0.0.1", PreferredPort: 65534, Cause: errors.New("address in use"),
	}
	rootErr := fmt.Errorf("host runtime: %w", cause)

	assertRuntimeCLIParity(t,
		func() (any, error) {
			return nil, service.MapServerFailure(rootErr)
		},
		func() (any, error) {
			return nil, factoryruntimecli.MapServerFailure(rootErr)
		},
	)

	mapped := service.MapServerFailure(rootErr)
	var stderr bytes.Buffer
	if !factoryruntimecli.WriteInvocationError(&stderr, mapped, false) {
		t.Fatal("WriteInvocationError did not recognize server bind failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(factoryruntimecli.ServerBindFailedCode) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	if !cliserver.IsLocalBindError(cause) && !platformhttpserver.IsBindError(cause) {
		t.Fatal("test bind error should classify as bind failure")
	}
}

func TestConstructedService_MapInvocationFailurePreservesCancellationAndTimeout(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []struct {
		name     string
		cause    error
		wantCode string
	}{
		{
			name:     "cancelled",
			cause:    context.Canceled,
			wantCode: factoryruntimecli.InvocationErrorCodeCancelled,
		},
		{
			name:     "timeout",
			cause:    context.DeadlineExceeded,
			wantCode: factoryruntimecli.InvocationErrorCodeTimeout,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return nil, service.MapInvocationFailure(tc.cause)
				},
				func() (any, error) {
					return nil, factoryruntimecli.MapInvocationFailure(tc.cause)
				},
			)

			mapped := service.MapInvocationFailure(tc.cause)
			var invocationErr *factoryruntimecli.InvocationError
			if !errors.As(mapped, &invocationErr) {
				t.Fatalf("error = %T, want InvocationError", mapped)
			}
			if invocationErr.Code != tc.wantCode || !errors.Is(mapped, tc.cause) {
				t.Fatalf("InvocationError = %#v", invocationErr)
			}
			assertWriteInvocationErrorParity(t, mapped)
		})
	}
}

func TestConstructedService_MapRuntimeRootFailureMatchesPackageCommand(t *testing.T) {
	t.Parallel()

	runtime := stubRuntimeRoot{observeErr: factoryruntime.ErrAlreadyStopped}
	service := constructedRuntimeCLIService(t, runtime)

	assertRuntimeCLIParity(t,
		func() (any, error) {
			return nil, service.MapRuntimeRootFailure(factoryruntime.ErrAlreadyStopped)
		},
		func() (any, error) {
			return nil, factoryruntimecli.MapRuntimeRootFailure(runtime, factoryruntime.ErrAlreadyStopped)
		},
	)

	mapped := service.MapRuntimeRootFailure(factoryruntime.ErrAlreadyStopped)
	var invocationErr *factoryruntimecli.InvocationError
	if !errors.As(mapped, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", mapped)
	}
	if invocationErr.Message != "factory runtime is already stopped" {
		t.Fatalf("message = %q, want factory runtime is already stopped", invocationErr.Message)
	}
}

func TestConstructedService_WriteInvocationErrorRendersInvocationCLIErrorContract(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cliErr := stubInvocationCLIError{
		code:    "INVOCATION_RUNTIME_FAILURE",
		message: "goal execution failed",
	}
	if service == nil {
		t.Fatal("constructed service = nil")
	}

	var stderr strings.Builder
	if !factoryruntimecli.WriteInvocationError(&stderr, cliErr, false) {
		t.Fatal("terminal invocation failure was not handled")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr.String()), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	if response.Message != "goal execution failed" {
		t.Fatalf("message = %q", response.Message)
	}
}

func TestConstructedService_ValidateInvocationOutputSelectionQuietMatchesPackage(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []struct {
		name           string
		quiet          bool
		jsonOutput     bool
		explicitOutput bool
		wantConflict   bool
	}{
		{name: "human"},
		{name: "quiet", quiet: true},
		{name: "single JSON", jsonOutput: true},
		{name: "JSON response stream", jsonOutput: true, explicitOutput: true},
		{name: "quiet and JSON", quiet: true, jsonOutput: true, wantConflict: true},
		{name: "quiet and explicit output", quiet: true, explicitOutput: true, wantConflict: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return nil, service.ValidateInvocationOutputSelection(
						tc.quiet,
						tc.jsonOutput,
						tc.explicitOutput,
					)
				},
				func() (any, error) {
					return nil, factoryruntimecli.ValidateInvocationOutputSelection(
						tc.quiet,
						tc.jsonOutput,
						tc.explicitOutput,
					)
				},
			)

			err := service.ValidateInvocationOutputSelection(tc.quiet, tc.jsonOutput, tc.explicitOutput)
			if !tc.wantConflict {
				if err != nil {
					t.Fatalf("ValidateInvocationOutputSelection() error = %v", err)
				}
				return
			}
			var invocationErr *factoryruntimecli.InvocationError
			if !errors.As(err, &invocationErr) ||
				invocationErr.Code != factoryruntimecli.InvocationOutputConflictCode {
				t.Fatalf("error = %#v, want %s InvocationError", err, factoryruntimecli.InvocationOutputConflictCode)
			}
			assertWriteInvocationErrorParity(t, err)
		})
	}
}

func TestConstructedService_ValidateInvocationOutputModeFailurePathsMatchPackage(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []struct {
		name string
		req  factoryruntimecli.ValidateInvocationOutputModeRequest
	}{
		{
			name: "continuous unsupported",
			req: factoryruntimecli.ValidateInvocationOutputModeRequest{
				InvocationOutputMode: factoryruntimecli.InvocationOutputResponseStream,
				Continuously:         true,
				InvocationMode:       true,
			},
		},
		{
			name: "non-invocation run unsupported",
			req: factoryruntimecli.ValidateInvocationOutputModeRequest{
				InvocationOutputMode: factoryruntimecli.InvocationOutputResponseStream,
				InvocationMode:       false,
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return nil, service.ValidateInvocationOutputMode(tc.req)
				},
				func() (any, error) {
					return nil, factoryruntimecli.ValidateInvocationOutputMode(tc.req)
				},
			)

			err := service.ValidateInvocationOutputMode(tc.req)
			var invocationErr *factoryruntimecli.InvocationError
			if !errors.As(err, &invocationErr) ||
				invocationErr.Code != factoryruntimecli.InvocationOutputUnsupportedCode {
				t.Fatalf("error = %#v, want unsupported output mode", err)
			}
			assertWriteInvocationErrorParity(t, err)
		})
	}
}

func TestConstructedService_NormalizeInvocationOutputModeMatchesPackage(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{
			name: "empty defaults to primary",
			raw:  "",
			want: factoryruntimecli.InvocationOutputPrimaryResult,
		},
		{
			name: "primary literal accepted",
			raw:  "primary",
			want: factoryruntimecli.InvocationOutputPrimaryResult,
		},
		{
			name: "response-stream accepted",
			raw:  "response-stream",
			want: factoryruntimecli.InvocationOutputResponseStream,
		},
		{
			name:    "unknown rejected",
			raw:     "sse",
			wantErr: "unsupported --output value",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return service.NormalizeInvocationOutputMode(tc.raw)
				},
				func() (any, error) {
					return factoryruntimecli.NormalizeInvocationOutputMode(tc.raw)
				},
			)

			got, err := service.NormalizeInvocationOutputMode(tc.raw)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NormalizeInvocationOutputMode(%q) error = %v, want %q", tc.raw, err, tc.wantErr)
				}
				assertWriteInvocationErrorParity(t, err)
				return
			}
			if err != nil {
				t.Fatalf("NormalizeInvocationOutputMode(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConstructedService_FormatDurationHumanPresentationMatchesPackage(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	cases := []time.Duration{
		90 * time.Minute,
		2*time.Hour + 15*time.Minute,
		45 * time.Second,
	}
	for _, duration := range cases {
		duration := duration
		t.Run(duration.String(), func(t *testing.T) {
			t.Parallel()

			assertRuntimeCLIParity(t,
				func() (any, error) {
					return service.FormatDuration(duration), nil
				},
				func() (any, error) {
					return factoryruntimecli.FormatDuration(duration), nil
				},
			)
		})
	}
}

func TestConstructedService_ObserveRuntimeHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := constructedRuntimeCLIService(t, cancelingRuntimeRoot{})
	serviceErr := service.ObserveRuntime(ctx, factoryruntime.ObserveRequest{})
	packageErr := factoryruntimecli.ObserveRuntime(ctx, cancelingRuntimeRoot{}, factoryruntime.ObserveRequest{})

	if (serviceErr == nil) != (packageErr == nil) {
		t.Fatalf("service error = %v, package error = %v", serviceErr, packageErr)
	}
	var serviceInvocationErr, packageInvocationErr *factoryruntimecli.InvocationError
	if !errors.As(serviceErr, &serviceInvocationErr) || !errors.As(packageErr, &packageInvocationErr) {
		t.Fatalf("errors = %#v / %#v, want InvocationError", serviceErr, packageErr)
	}
	if serviceInvocationErr.Code != factoryruntimecli.InvocationErrorCodeCancelled ||
		packageInvocationErr.Code != factoryruntimecli.InvocationErrorCodeCancelled {
		t.Fatalf(
			"service code = %q, package code = %q, want %q",
			serviceInvocationErr.Code,
			packageInvocationErr.Code,
			factoryruntimecli.InvocationErrorCodeCancelled,
		)
	}
	if !errors.Is(serviceErr, context.Canceled) || !errors.Is(packageErr, context.Canceled) {
		t.Fatalf("service cause = %v, package cause = %v, want context.Canceled", serviceErr, packageErr)
	}
	assertWriteInvocationErrorParity(t, serviceErr)
}

func TestConstructedService_WriteInvocationErrorNilWriterStillRecognizesContract(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	mapped := service.MapInvocationFailure(context.Canceled)
	if !factoryruntimecli.WriteInvocationError(io.Discard, mapped, true) {
		t.Fatal("WriteInvocationError should recognize cancellation in quiet mode")
	}
	if !factoryruntimecli.WriteInvocationError(nil, mapped, true) {
		t.Fatal("WriteInvocationError should recognize cancellation with nil writer")
	}
}

func TestConstructedService_MapCurrentFactoryFailurePreservesFactoryLayoutNotFound(t *testing.T) {
	t.Parallel()

	service := constructedRuntimeCLIService(t, nil)
	err := fmt.Errorf("load Current Factory: %w", factorydefinitions.ErrFactoryLayoutNotFound)
	mapped := service.MapCurrentFactoryFailure(err)
	var invocationErr *factoryruntimecli.InvocationError
	if !errors.As(mapped, &invocationErr) {
		t.Fatalf("error = %#v, want InvocationError", mapped)
	}
	if invocationErr.Code != factoryruntimecli.CurrentFactoryNotFoundCode {
		t.Fatalf("code = %q, want %q", invocationErr.Code, factoryruntimecli.CurrentFactoryNotFoundCode)
	}
}
