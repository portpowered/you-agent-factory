package wire_test

import (
	"io/fs"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() = %v", err)
	}

	service, err := settingswire.NewService(
		&stubFileSystem{},
		stubCreateTemporaryFile,
		stubConfigDecoder,
		stubConfigEncoder,
		stubProviderCatalog,
		providersRoot,
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}

	var root operatorsettings.Service = service
	if root == nil {
		t.Fatal("constructed value is not assignable to operatorsettings.Service")
	}
}

func TestNewServiceRejectsMissingRequiredPorts(t *testing.T) {
	t.Parallel()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() = %v", err)
	}

	tests := []struct {
		name string
		call func() (operatorsettings.Service, error)
		want string
	}{
		{
			name: "filesystem",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					nil,
					stubCreateTemporaryFile,
					stubConfigDecoder,
					stubConfigEncoder,
					stubProviderCatalog,
					providersRoot,
				)
			},
			want: "construct Operator Settings: filesystem is required",
		},
		{
			name: "create temporary file",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					&stubFileSystem{},
					nil,
					stubConfigDecoder,
					stubConfigEncoder,
					stubProviderCatalog,
					providersRoot,
				)
			},
			want: "construct Operator Settings: create temporary file is required",
		},
		{
			name: "config decoder",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					&stubFileSystem{},
					stubCreateTemporaryFile,
					nil,
					stubConfigEncoder,
					stubProviderCatalog,
					providersRoot,
				)
			},
			want: "construct Operator Settings: config decoder is required",
		},
		{
			name: "config encoder",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					&stubFileSystem{},
					stubCreateTemporaryFile,
					stubConfigDecoder,
					nil,
					stubProviderCatalog,
					providersRoot,
				)
			},
			want: "construct Operator Settings: config encoder is required",
		},
		{
			name: "provider catalog",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					&stubFileSystem{},
					stubCreateTemporaryFile,
					stubConfigDecoder,
					stubConfigEncoder,
					nil,
					providersRoot,
				)
			},
			want: "construct Operator Settings: provider catalog is required",
		},
		{
			name: "providers root",
			call: func() (operatorsettings.Service, error) {
				return settingswire.NewService(
					&stubFileSystem{},
					stubCreateTemporaryFile,
					stubConfigDecoder,
					stubConfigEncoder,
					stubProviderCatalog,
					nil,
				)
			},
			want: "construct Operator Settings: providers root is required",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service, err := test.call()
			if err == nil || service != nil {
				t.Fatalf("call = (%v, %v), want error %q", service, err, test.want)
			}
			if err.Error() != test.want {
				t.Fatalf("error = %q, want %q", err.Error(), test.want)
			}
		})
	}
}

type stubFileSystem struct{}

func (stubFileSystem) ReadFile(string) ([]byte, error) {
	panic("filesystem read during wire construction test")
}

func (stubFileSystem) MkdirAll(string, fs.FileMode) error {
	panic("filesystem mkdir during wire construction test")
}

func (stubFileSystem) Remove(string) error {
	panic("filesystem remove during wire construction test")
}

func (stubFileSystem) Chmod(string, fs.FileMode) error {
	panic("filesystem chmod during wire construction test")
}

func (stubFileSystem) Rename(string, string) error {
	panic("filesystem rename during wire construction test")
}

func stubCreateTemporaryFile(string, string) (operatorsettings.TemporaryFile, error) {
	panic("temp-file creation during wire construction test")
}

func stubConfigDecoder([]byte) (operatorsettings.Config, error) {
	panic("config decode during wire construction test")
}

func stubConfigEncoder(operatorsettings.Config) ([]byte, error) {
	panic("config encode during wire construction test")
}

func stubProviderCatalog(string) (string, bool) {
	panic("provider catalog during wire construction test")
}
