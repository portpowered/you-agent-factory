package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerscli "github.com/portpowered/infinite-you/pkg/services/providers/transports/cli"
)

func constructedProvidersCLIService(
	t *testing.T,
	root providers.Service,
) providerscli.Service {
	t.Helper()
	service := providerscli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Providers CLI service")
	}
	return service
}

func assertListParity(
	t *testing.T,
	service providerscli.Service,
	root providers.Service,
	cfg providerscli.ListConfig,
) {
	t.Helper()

	newCfg := func() providerscli.ListConfig {
		invocation := cfg
		invocation.Output = &bytes.Buffer{}
		return invocation
	}
	run := func(invoke func(providerscli.ListConfig) error) (*bytes.Buffer, error) {
		invocation := newCfg()
		return invocation.Output.(*bytes.Buffer), invoke(invocation)
	}

	serviceOut, serviceErr := run(service.List)
	commandOut, commandErr := run(func(invocation providerscli.ListConfig) error {
		return providerscli.List(invocation, root)
	})

	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceErr != nil && commandErr != nil && serviceErr.Error() != commandErr.Error() {
		t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
}

func TestConstructedService_ListInvokesProvidersRootListProviders(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{
		listResult: providers.ListProvidersResult{
			Providers: []providers.Descriptor{{
				ID:           providers.IDCodex,
				DisplayName:  "Codex",
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessReady,
			}},
		},
	}
	service := constructedProvidersCLIService(t, root)
	var out bytes.Buffer
	if err := service.List(providerscli.ListConfig{
		Context: context.Background(),
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if root.listProvidersCalls != 1 {
		t.Fatalf("ListProviders calls = %d, want 1", root.listProvidersCalls)
	}
	if !strings.Contains(out.String(), "codex\tCodex\tselectable\tready\tnone") {
		t.Fatalf("stdout = %q, want codex row", out.String())
	}
}

func TestConstructedService_ListJSONOutputPreservesAcceptedEnvelope(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{
		listResult: providers.ListProvidersResult{
			Providers: []providers.Descriptor{{
				ID:           providers.IDCodex,
				DisplayName:  "Codex",
				Aliases:      []string{"openai-codex"},
				Availability: providers.AvailabilitySelectable,
				Readiness:    providers.ReadinessReady,
				Capabilities: []providers.Capability{
					providers.CapabilityPromptSubmission,
					providers.CapabilityNativeStreaming,
				},
			}},
		},
	}
	service := constructedProvidersCLIService(t, root)
	var out bytes.Buffer
	if err := service.List(providerscli.ListConfig{
		Context: context.Background(),
		JSON:    true,
		Output:  &out,
	}); err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var got struct {
		Providers []struct {
			ID           string   `json:"id"`
			DisplayName  string   `json:"displayName"`
			Aliases      []string `json:"aliases"`
			Availability string   `json:"availability"`
			Readiness    string   `json:"readiness"`
			Capabilities []string `json:"capabilities"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	if len(got.Providers) != 1 || got.Providers[0].ID != "codex" {
		t.Fatalf("providers = %#v, want codex entry", got.Providers)
	}
	if got.Providers[0].DisplayName != "Codex" ||
		got.Providers[0].Availability != "selectable" ||
		got.Providers[0].Readiness != "ready" {
		t.Fatalf("provider entry = %#v, want codex catalog facts", got.Providers[0])
	}
	if len(got.Providers[0].Capabilities) != 2 {
		t.Fatalf("capabilities = %#v, want two entries", got.Providers[0].Capabilities)
	}
	if bytes.Contains(out.Bytes(), []byte("ID\tDISPLAY NAME")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}
}

func TestConstructedService_ListHumanAndJSONMatchPackageCommand(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{
		listResult: providers.ListProvidersResult{
			Providers: []providers.Descriptor{
				{
					ID:           providers.IDCursor,
					DisplayName:  "Cursor",
					Aliases:      []string{"cursor"},
					Availability: providers.AvailabilitySupportedButUnavailable,
					Readiness:    providers.ReadinessUnavailable,
				},
				{
					ID:           providers.IDCodex,
					DisplayName:  "Codex",
					Availability: providers.AvailabilitySelectable,
					Readiness:    providers.ReadinessReady,
				},
			},
		},
	}
	service := constructedProvidersCLIService(t, root)
	for name, jsonMode := range map[string]bool{"human": false, "json": true} {
		name, jsonMode := name, jsonMode
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertListParity(t, service, root, providerscli.ListConfig{
				Context: context.Background(),
				JSON:    jsonMode,
			})
		})
	}
}

func TestConstructedService_ListHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := &recordingProvidersRoot{
		listFn: func(callCtx context.Context) (providers.ListProvidersResult, error) {
			if err := callCtx.Err(); err != nil {
				return providers.ListProvidersResult{}, err
			}
			return providers.ListProvidersResult{}, nil
		},
	}
	service := constructedProvidersCLIService(t, root)
	err := service.List(providerscli.ListConfig{Context: ctx, Output: io.Discard})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
}

func TestConstructedService_ListVerboseDiagnosticsOnRootFailure(t *testing.T) {
	t.Parallel()

	root := &recordingProvidersRoot{
		listErr: errors.New("catalog unavailable"),
	}
	service := constructedProvidersCLIService(t, root)
	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := service.List(providerscli.ListConfig{
		Context:     context.Background(),
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	})
	if err == nil || err.Error() != "catalog unavailable" {
		t.Fatalf("List() error = %v, want catalog failure", err)
	}
	diag := diagnostics.String()
	if !strings.Contains(diag, "providers list failed") {
		t.Fatalf("diagnostics missing failure detail:\n%s", diag)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty on failure, got %q", out.String())
	}
}
