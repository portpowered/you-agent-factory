package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
)

func TestInstallPackagedFactoryDelegatesToDefinitionsOperation(t *testing.T) {
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
	var output bytes.Buffer
	ctx := startupcli.WithWorkingDirectory(context.Background(), "/workspace")
	err := factorydefinitionscli.InstallPackagedFactory(
		factorydefinitionscli.InstallPackagedFactoryConfig{
			Context:      ctx,
			HomeDir:      "/home/operator",
			Package:      "@you/goal",
			Dir:          "alternate-factories",
			DirChanged:   true,
			Format:       "yaml",
			FormatChanged: true,
			Output:       &output,
		},
		install,
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	if got.RootDir != filepath.Join("/workspace", "alternate-factories") ||
		got.Name != "@you/goal" ||
		got.Format != factorydefinitions.PackagedFactoryFormatYAML ||
		got.Replace {
		t.Fatalf("request = %#v, want delegated packaged install inputs", got)
	}
	if got := output.String(); !strings.Contains(got, "Installed packaged factory @you/goal") {
		t.Fatalf("stdout = %q, want human success output", got)
	}
}

func TestInstallPackagedFactoryRendersJSONSuccess(t *testing.T) {
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
	var output bytes.Buffer
	err := factorydefinitionscli.InstallPackagedFactory(
		factorydefinitionscli.InstallPackagedFactoryConfig{
			Context: ctx(t),
			HomeDir: "/home/operator",
			Package: "@you/goal",
			JSON:    true,
			Output:  &output,
		},
		install,
	)
	if err != nil {
		t.Fatalf("InstallPackagedFactory() error = %v", err)
	}
	var payload factorydefinitionscli.InstallPackagedFactoryResult
	if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Outcome != "skipped" || payload.Name != "@you/goal" {
		t.Fatalf("payload = %#v, want skipped JSON facts", payload)
	}
}

func TestInstallPackagedFactoryPreservesDefinitionsErrors(t *testing.T) {
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
	err := factorydefinitionscli.InstallPackagedFactory(
		factorydefinitionscli.InstallPackagedFactoryConfig{
			Context: ctx(t),
			HomeDir: "/home/operator",
			Package: "@you/missing",
			Output:  io.Discard,
		},
		install,
	)
	if !errors.Is(err, factorydefinitions.ErrUnknownPackagedFactoryIdentity) {
		t.Fatalf("error = %v, want unknown packaged factory identity", err)
	}
}

func TestInstallPackagedFactoryRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()
	called := false
	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		called = true
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	}
	err := factorydefinitionscli.InstallPackagedFactory(
		factorydefinitionscli.InstallPackagedFactoryConfig{
			Context:       ctx(t),
			HomeDir:       "/home/operator",
			Package:       "@you/goal",
			Format:        "toml",
			FormatChanged: true,
			Output:        io.Discard,
		},
		install,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("error = %v, want unsupported format failure", err)
	}
	if called {
		t.Fatal("install operation called after format validation failure")
	}
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	return startupcli.WithWorkingDirectory(t.Context(), "/workspace")
}
