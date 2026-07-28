package factorydefinitions

import (
	"errors"
	"testing"
)

func TestValidateInstallPackagedFactoryRequest_RejectsScaffoldOptions(t *testing.T) {
	tests := []struct {
		name    string
		scaffold CreateFactoryScaffoldRequest
	}{
		{
			name: "target dir",
			scaffold: CreateFactoryScaffoldRequest{TargetDir: "./factory"},
		},
		{
			name: "type",
			scaffold: CreateFactoryScaffoldRequest{Type: "ralph"},
		},
		{
			name: "executor",
			scaffold: CreateFactoryScaffoldRequest{Executor: "claude"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateInstallPackagedFactoryRequest(InstallPackagedFactoryRequest{
				Name:     "@you/goal",
				RootDir:  "/tmp/factories",
				Scaffold: test.scaffold,
			})
			if err == nil || err != ErrIncompatibleFactoryDistributeOptions {
				t.Fatalf("ValidateInstallPackagedFactoryRequest() error = %v, want %v", err, ErrIncompatibleFactoryDistributeOptions)
			}
		})
	}
}

func TestValidateCreateFactoryScaffoldRequest_RejectsBlankTargetDir(t *testing.T) {
	err := ValidateCreateFactoryScaffoldRequest(CreateFactoryScaffoldRequest{TargetDir: "  "})
	if err == nil || !errors.Is(err, ErrFactoryDistributeFailed) {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v, want %v", err, ErrFactoryDistributeFailed)
	}
}

func TestValidateCreateFactoryScaffoldRequest_RejectsUnsupportedType(t *testing.T) {
	err := ValidateCreateFactoryScaffoldRequest(CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
		Type:      "ralph",
	})
	if err == nil || !errors.Is(err, ErrFactoryDistributeFailed) {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v, want %v", err, ErrFactoryDistributeFailed)
	}
}

func TestValidateCreateFactoryScaffoldRequest_AllowsDefaultScaffoldRequest(t *testing.T) {
	if err := ValidateCreateFactoryScaffoldRequest(CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
		Type:      DefaultScaffoldType,
		Executor:  "codex",
	}); err != nil {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v", err)
	}
	if err := ValidateCreateFactoryScaffoldRequest(CreateFactoryScaffoldRequest{
		TargetDir: "/tmp/factories/alpha",
	}); err != nil {
		t.Fatalf("ValidateCreateFactoryScaffoldRequest() error = %v", err)
	}
}

func TestValidateInstallPackagedFactoryRequest_AllowsPackagedOnlyRequest(t *testing.T) {
	if err := ValidateInstallPackagedFactoryRequest(InstallPackagedFactoryRequest{
		Name:    "@you/goal",
		RootDir: "/tmp/factories",
		Format:  PackagedFactoryFormatJSON,
		Replace: true,
	}); err != nil {
		t.Fatalf("ValidateInstallPackagedFactoryRequest() error = %v", err)
	}
}
