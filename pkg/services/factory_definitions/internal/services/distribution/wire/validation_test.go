package wire_test

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	distributionwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/distribution/wire"
)

func TestValidateInstallPackagedFactoryRequest_RejectsScaffoldOptions(t *testing.T) {
	tests := []struct {
		name     string
		scaffold factorydefinitions.CreateFactoryScaffoldRequest
	}{
		{
			name:     "target dir",
			scaffold: factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: "./factory"},
		},
		{
			name:     "type",
			scaffold: factorydefinitions.CreateFactoryScaffoldRequest{Type: "ralph"},
		},
		{
			name:     "executor",
			scaffold: factorydefinitions.CreateFactoryScaffoldRequest{Executor: "claude"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := distributionwire.ValidateInstallPackagedFactoryRequest(factorydefinitions.InstallPackagedFactoryRequest{
				Name:     "@you/goal",
				RootDir:  "/tmp/factories",
				Scaffold: test.scaffold,
			})
			if err == nil || err != factorydefinitions.ErrIncompatibleFactoryDistributeOptions {
				t.Fatalf("ValidateInstallPackagedFactoryRequest() error = %v, want %v", err, factorydefinitions.ErrIncompatibleFactoryDistributeOptions)
			}
		})
	}
}

func TestValidateCreateFactoryScaffoldRequest_RejectsBlankTargetDir(t *testing.T) {
	err := distributionwire.ValidateCreateFactoryScaffoldRequest(factorydefinitions.CreateFactoryScaffoldRequest{TargetDir: "  "})
	if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v, want %v", err, factorydefinitions.ErrFactoryDistributeFailed)
	}
}

func TestValidateCreateFactoryScaffoldRequest_RejectsUnsupportedType(t *testing.T) {
	err := distributionwire.ValidateCreateFactoryScaffoldRequest(factorydefinitions.CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
		Type:      "ralph",
	})
	if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryDistributeFailed) {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v, want %v", err, factorydefinitions.ErrFactoryDistributeFailed)
	}
}

func TestValidateCreateFactoryScaffoldRequest_AllowsDefaultScaffoldRequest(t *testing.T) {
	if err := distributionwire.ValidateCreateFactoryScaffoldRequest(factorydefinitions.CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
		Type:      factorydefinitions.DefaultScaffoldType,
		Executor:  "codex",
	}); err != nil {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v", err)
	}
	if err := distributionwire.ValidateCreateFactoryScaffoldRequest(factorydefinitions.CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
	}); err != nil {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v", err)
	}
}

func TestValidateInstallPackagedFactoryRequest_AllowsPackagedOnlyRequest(t *testing.T) {
	if err := distributionwire.ValidateInstallPackagedFactoryRequest(factorydefinitions.InstallPackagedFactoryRequest{
		Name:    "@you/goal",
		RootDir: "/tmp/factories",
		Format:  factorydefinitions.PackagedFactoryFormatJSON,
		Replace: true,
	}); err != nil {
		t.Fatalf("ValidateInstallPackagedFactoryRequest() error = %v", err)
	}
}
