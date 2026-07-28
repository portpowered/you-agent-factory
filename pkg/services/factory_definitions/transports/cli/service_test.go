package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
)

func TestNewRequiresInstallCollaborator(t *testing.T) {
	t.Parallel()

	if service := factorydefinitionscli.New(nil); service != nil {
		t.Fatalf("New(nil) = %T, want nil", service)
	}
}

func TestConstructedService_RequiresCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	}
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}

	err := service.InstallPackagedFactory(factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx(t),
		HomeDir: "/home/operator",
		Package: "@you/goal",
	})
	if err == nil || err.Error() != "packaged factory installation output is required" {
		t.Fatalf("error = %v, want packaged factory installation output is required", err)
	}
}

func TestConstructedService_InstallPackagedFactoryMatchesPackageFunction(t *testing.T) {
	t.Parallel()

	var got factorydefinitions.InstallPackagedFactoryRequest
	install := func(
		_ context.Context,
		request factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		got = request
		return factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
			},
			Outcome: factorydefinitions.PackagedFactoryInstallCreated,
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		}, nil
	}
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}

	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context:       startupcli.WithWorkingDirectory(context.Background(), "/workspace"),
		HomeDir:       "/home/operator",
		Package:       "@you/goal",
		Dir:           "alternate-factories",
		DirChanged:    true,
		Format:        "yaml",
		FormatChanged: true,
	}
	assertInstallPackagedFactoryParity(t, service, install, cfg, func() {
		if got.RootDir != filepath.Join("/workspace", "alternate-factories") ||
			got.Name != "@you/goal" ||
			got.Format != factorydefinitions.PackagedFactoryFormatYAML ||
			got.Replace {
			t.Fatalf("request = %#v, want delegated packaged install inputs", got)
		}
	})
}

func TestConstructedService_InstallPackagedFactoryRendersJSONSuccess(t *testing.T) {
	t.Parallel()

	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
			},
			Outcome: factorydefinitions.PackagedFactoryInstallSkipped,
			Format:  factorydefinitions.PackagedFactoryFormatJSON,
		}, nil
	}
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}

	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx(t),
		HomeDir: "/home/operator",
		Package: "@you/goal",
		JSON:    true,
	}
	assertInstallPackagedFactoryParity(t, service, install, cfg, func() {
		// parity assertion handled by output comparison
	})
}

func TestConstructedService_InstallPackagedFactoryPreservesDefinitionsErrors(t *testing.T) {
	t.Parallel()

	installErr := &factorydefinitions.UnknownPackagedFactoryError{
		Name:      "@you/missing",
		Available: []string{"@you/goal"},
	}
	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, installErr
	}
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}

	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx(t),
		HomeDir: "/home/operator",
		Package: "@you/missing",
	}
	assertInstallPackagedFactoryParity(t, service, install, cfg, func() {
		// parity assertion handled by error comparison
	})
}

func TestConstructedService_InstallPackagedFactoryRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	called := false
	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		called = true
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	}
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}

	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context:       ctx(t),
		HomeDir:       "/home/operator",
		Package:       "@you/goal",
		Format:        "toml",
		FormatChanged: true,
	}
	assertInstallPackagedFactoryParity(t, service, install, cfg, func() {
		if called {
			t.Fatal("install operation called after format validation failure")
		}
	})
}

func assertInstallPackagedFactoryParity(
	t *testing.T,
	service factorydefinitionscli.Service,
	install factorydefinitionscli.InstallPackagedFactoryOperation,
	cfg factorydefinitionscli.InstallPackagedFactoryConfig,
	after func(),
) {
	t.Helper()

	var serviceOut, commandOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	commandCfg := cfg
	commandCfg.Output = &commandOut

	serviceErr := service.InstallPackagedFactory(serviceCfg)
	commandErr := factorydefinitionscli.InstallPackagedFactory(commandCfg, install)

	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceErr != nil && commandErr != nil {
		if !errors.Is(serviceErr, commandErr) && serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
	if cfg.JSON && serviceErr == nil {
		var payload factorydefinitionscli.InstallPackagedFactoryResult
		if err := json.Unmarshal(serviceOut.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload.Name == "" || payload.FactoryDir == "" || payload.Outcome == "" || payload.Format == "" {
			t.Fatalf("payload = %#v, want retained JSON fields", payload)
		}
	}
	if serviceErr == nil && !cfg.JSON {
		got := serviceOut.String()
		if !strings.Contains(got, "@you/goal") {
			t.Fatalf("stdout = %q, want human success output mentioning package name", got)
		}
	}
	if serviceErr != nil && strings.Contains(cfg.Format, "toml") {
		if !strings.Contains(serviceErr.Error(), "unsupported format") {
			t.Fatalf("error = %v, want unsupported format failure", serviceErr)
		}
	}
	if serviceErr != nil && cfg.Package == "@you/missing" {
		if !errors.Is(serviceErr, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
			t.Fatalf("error = %v, want unknown packaged factory identity", serviceErr)
		}
	}
	if after != nil {
		after()
	}
}
