package cli_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
)

func TestBindInstallPackagedFactoryRequiresCollaborator(t *testing.T) {
	t.Parallel()

	if operation := factorydefinitionscli.BindInstallPackagedFactory(nil); operation != nil {
		t.Fatalf("BindInstallPackagedFactory(nil) = %T, want nil", operation)
	}
}

func TestBindInstallPackagedFactoryDelegatesThroughAdapterService(t *testing.T) {
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
			Format:  factorydefinitions.PackagedFactoryFormatYAML,
		}, nil
	}
	operation := factorydefinitionscli.BindInstallPackagedFactory(install)
	if operation == nil {
		t.Fatal("BindInstallPackagedFactory(install) = nil, want composition operation")
	}

	var output bytes.Buffer
	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context:       startupcli.WithWorkingDirectory(context.Background(), "/workspace/fleet"),
		HomeDir:       "/home/operator",
		Package:       "@you/goal",
		Dir:           "alternate-factories",
		DirChanged:    true,
		Format:        "yaml",
		FormatChanged: true,
		Replace:       true,
		Output:        &output,
	}
	if err := operation(cfg); err != nil {
		t.Fatalf("operation(cfg) error = %v", err)
	}
	if got.RootDir != filepath.Join("/workspace/fleet", "alternate-factories") ||
		got.Name != "@you/goal" ||
		got.Format != factorydefinitions.PackagedFactoryFormatYAML ||
		!got.Replace {
		t.Fatalf("request = %#v, want delegated packaged install inputs", got)
	}
	if output.String() == "" {
		t.Fatal("operation output is empty, want human success presentation")
	}
}

func TestBindInstallPackagedFactoryMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	installErr := errors.New("packaged factory install failed")
	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, installErr
	}
	operation := factorydefinitionscli.BindInstallPackagedFactory(install)

	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx(t),
		HomeDir: "/home/operator",
		Package: "@you/goal",
		Output:  bytes.NewBuffer(nil),
	}
	boundErr := operation(cfg)
	directErr := factorydefinitionscli.InstallPackagedFactory(cfg, install)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && boundErr.Error() != directErr.Error() {
		t.Fatalf("bound error = %q, direct error = %q", boundErr.Error(), directErr.Error())
	}
}
