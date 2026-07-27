package factorydefinitions

import "testing"

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
