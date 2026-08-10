package packagedinstallation

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
)

func TestInstallPackagedFactory_OwnershipAcquisitionFailuresAreBounded(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*failingPackagedInstallationFileSystem, *scriptedOwnerProbe)
		want      string
	}{
		{
			name: "mkdir all",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) {
				fileSystem.mkdirAllErr = errors.New("mkdir all failed")
			},
			want: "mkdir all failed",
		},
		{
			name: "mkdir lease",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) {
				fileSystem.mkdirErr = errors.New("mkdir lease failed")
			},
			want: "mkdir lease failed",
		},
		{
			name: "owner probe",
			configure: func(_ *failingPackagedInstallationFileSystem, probe *scriptedOwnerProbe) {
				probe.currentErr = errors.New("owner probe failed")
			},
			want: "owner probe failed",
		},
		{
			name: "invalid owner pid",
			configure: func(_ *failingPackagedInstallationFileSystem, probe *scriptedOwnerProbe) {
				probe.record.PID = 0
			},
			want: "owner PID is invalid",
		},
		{
			name: "write owner metadata",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) {
				fileSystem.writeFileErr = errors.New("write owner metadata failed")
			},
			want: "write owner metadata failed",
		},
		{
			name: "publish owner metadata",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) {
				fileSystem.renameErr = errors.New("publish owner metadata failed")
			},
			want: "publish owner metadata failed",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			fileSystem := &failingPackagedInstallationFileSystem{}
			probe := &scriptedOwnerProbe{
				record:   ownerRecord{PID: 101},
				liveness: ownerLivenessActive,
			}
			test.configure(fileSystem, probe)

			_, err := newWithOwnerProbe(
				packagedInstallationTestPersistence(),
				fileSystem,
				probe,
			).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition: factorydefinitions.PackagedDefinition{
					Name: "@test/ownership-failure",
					JSON: []byte(`{}`),
				},
				Format: factorydefinitions.PackagedFactoryFormatJSON,
			})
			if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
				t.Fatalf("InstallPackagedFactory() error = %v, want typed contention", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("InstallPackagedFactory() error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestInstallPackagedFactory_PreExistingStagingReportsOwnerEvidence(t *testing.T) {
	tests := []struct {
		name      string
		configure func(string, *failingPackagedInstallationFileSystem, *scriptedOwnerProbe) string
		want      []string
	}{
		{
			name: "unowned transaction stage",
			configure: func(root string, _ *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) string {
				name := "@test/unowned-stage"
				path := filepath.Join(root, authoringlayoutpersist.StagingDirectoryPrefix(name)+"transaction")
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatalf("create transaction stage: %v", err)
				}
				return name
			},
			want: []string{"outcome=indeterminate-contention", "owner_liveness=indeterminate"},
		},
		{
			name: "permission denied owner metadata",
			configure: func(root string, fileSystem *failingPackagedInstallationFileSystem, _ *scriptedOwnerProbe) string {
				name := "@test/permission-owner"
				path := stagingOwnershipPath(root, name)
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("create owner stage: %v", err)
				}
				fileSystem.readFileErr = fs.ErrPermission
				return name
			},
			want: []string{"outcome=indeterminate-contention", "owner_liveness=permission-denied"},
		},
		{
			name: "owner liveness indeterminate",
			configure: func(root string, _ *failingPackagedInstallationFileSystem, probe *scriptedOwnerProbe) string {
				name := "@test/indeterminate-owner"
				path := stagingOwnershipPath(root, name)
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("create owner stage: %v", err)
				}
				if err := os.WriteFile(filepath.Join(path, stagingOwnerMetadataName), []byte(`{"pid":123}`), 0o600); err != nil {
					t.Fatalf("write owner metadata: %v", err)
				}
				probe.liveness = ownerLivenessIndeterminate
				return name
			},
			want: []string{"outcome=indeterminate-contention", "owner_liveness=indeterminate", "owner_pid=123"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			fileSystem := &failingPackagedInstallationFileSystem{}
			probe := &scriptedOwnerProbe{record: ownerRecord{PID: 101}, liveness: ownerLivenessActive}
			name := test.configure(root, fileSystem, probe)
			_, err := newWithOwnerProbe(
				packagedInstallationTestPersistence(),
				fileSystem,
				probe,
			).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition: factorydefinitions.PackagedDefinition{
					Name: name,
					JSON: []byte(`{}`),
				},
				Format: factorydefinitions.PackagedFactoryFormatJSON,
			})
			if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
				t.Fatalf("InstallPackagedFactory() error = %v, want typed contention", err)
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("InstallPackagedFactory() error = %q, want %q", err, want)
				}
			}
		})
	}
}

func TestInstallPackagedFactory_StagingInspectionErrorsAreReported(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*failingPackagedInstallationFileSystem, string)
		want      string
	}{
		{
			name: "ownership stat",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, root string) {
				fileSystem.statPath = stagingOwnershipPath(root, "@test/inspection-error")
				fileSystem.statErr = errors.New("ownership stat failed")
			},
			want: "ownership stat failed",
		},
		{
			name: "staging directory read",
			configure: func(fileSystem *failingPackagedInstallationFileSystem, _ string) {
				fileSystem.readDirErr = errors.New("staging directory read failed")
			},
			want: "staging directory read failed",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			fileSystem := &failingPackagedInstallationFileSystem{}
			test.configure(fileSystem, root)
			_, err := New(packagedInstallationTestPersistence(), fileSystem).InstallPackagedFactory(
				t.Context(),
				factorydefinitions.PackagedFactoryInstallParams{
					NamedFactoriesRoot: root,
					Definition: factorydefinitions.PackagedDefinition{
						Name: "@test/inspection-error",
						JSON: []byte(`{}`),
					},
					Format: factorydefinitions.PackagedFactoryFormatJSON,
				},
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("InstallPackagedFactory() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestInstallPackagedFactory_ReportsLeaseReleaseFailures(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*failingPackagedInstallationFileSystem)
		want      string
	}{
		{
			name: "owner metadata read",
			configure: func(fileSystem *failingPackagedInstallationFileSystem) {
				fileSystem.readFileErr = errors.New("release metadata read failed")
			},
			want: "owner_liveness=indeterminate",
		},
		{
			name: "owner changed",
			configure: func(fileSystem *failingPackagedInstallationFileSystem) {
				fileSystem.overrideReadFile = true
				fileSystem.readFileData = []byte(`{"pid":202}`)
			},
			want: "owner_liveness=racing",
		},
		{
			name: "lease removal",
			configure: func(fileSystem *failingPackagedInstallationFileSystem) {
				fileSystem.removeErr = errors.New("lease removal failed")
			},
			want: "owner_liveness=active",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			fileSystem := &failingPackagedInstallationFileSystem{}
			test.configure(fileSystem)
			probe := &scriptedOwnerProbe{record: ownerRecord{PID: 101}, liveness: ownerLivenessActive}
			_, err := newWithOwnerProbe(
				&successfulPackagedInstallationPersistence{},
				fileSystem,
				probe,
			).InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{
				NamedFactoriesRoot: root,
				Definition: factorydefinitions.PackagedDefinition{
					Name: "@test/release-error",
					JSON: []byte(`{}`),
				},
				Format: factorydefinitions.PackagedFactoryFormatJSON,
			})
			if err == nil || !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) {
				t.Fatalf("InstallPackagedFactory() error = %v, want typed contention", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("InstallPackagedFactory() error = %q, want %q", err, test.want)
			}
		})
	}
}

type successfulPackagedInstallationPersistence struct {
	factorydefinitions.Persistence
}

func (persistence *successfulPackagedInstallationPersistence) PrepareFactoryLayout(
	context.Context,
	string,
	[]byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	return &factorydefinitions.PreparedFactoryLayoutPayload{}, nil
}

func (persistence *successfulPackagedInstallationPersistence) CreateNamedFactory(
	rootDir string,
	name string,
	_ *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	return filepath.Join(rootDir, name), nil
}
