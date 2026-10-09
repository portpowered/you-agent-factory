package packagedinstallation

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutpersist "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/persist"
)

func TestStagingLeaseIdentityChangePreservesOwner(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"revalidate", "release"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			name := "@test/identity-change"
			path := stagingOwnershipPath(root, name)
			if err := os.MkdirAll(path, 0755); err != nil {
				t.Fatal(err)
			}
			original := ownerRecord{PID: 42, Host: "local", Start: "first"}
			changed := original
			changed.Start = "second"
			service := newWithOwnerProbe(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, &scriptedOwnerProbe{record: original, liveness: ownerLivenessOrphaned}, logging.NoopLogger{})
			if err := service.publishOwnerRecord(path, changed); err != nil {
				t.Fatal(err)
			}
			var err error
			if phase == "revalidate" {
				_, err = service.reclaimOrphanedStaging("scope", root, name, path, original)
			} else {
				err = service.releaseStagingOwnership(&stagingLease{path: path, root: root, name: name, owner: original})
			}
			if !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) || !strings.Contains(err.Error(), "racing") {
				t.Fatalf("changed identity: %v", err)
			}
			owner, _, err := service.readOwnerRecord(path)
			if err != nil || owner != changed {
				t.Fatalf("changed owner lost: %+v %v", owner, err)
			}
		})
	}
}

func TestLocalOwnerPublicationFallsBackOnUnavailableIdentity(t *testing.T) {
	t.Parallel()
	probe := localOwnerProbe{incarnations: factorydefinitions.PackagedInstallationProcessProbe(func(int) (platformprocess.Incarnation, error) {
		return platformprocess.Incarnation{}, errors.New("unsupported")
	})}
	owner, err := probe.Current()
	if err != nil || owner != (ownerRecord{PID: os.Getpid()}) {
		t.Fatalf("legacy publication: %+v %v", owner, err)
	}
}

func TestInstallPackagedFactory_IncarnationOwnership(t *testing.T) {
	t.Parallel()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	full := fmt.Sprintf(`{"pid":42,"host":%q,"start":"old"}`, host)
	for _, cell := range []struct {
		name, record string
		lookupErr    error
		start        string
		want         ownerLiveness
	}{
		{"gone", full, platformprocess.ErrProcessGone, "", ownerLivenessOrphaned},
		{"reused", full, nil, "new", ownerLivenessOrphaned},
		{"matching", full, nil, "old", ownerLivenessActive},
		{"legacy live", `{"pid":42}`, nil, "new", ownerLivenessActive},
		{"legacy PID-only live", `{"pid":42}`, nil, "", ownerLivenessActive},
		{"full identity cannot use PID-only observation", full, nil, "", ownerLivenessIndeterminate},
		{"denied", full, fs.ErrPermission, "", ownerLivenessPermissionDenied},
		{"unknown", full, errors.New("query unavailable"), "", ownerLivenessIndeterminate},
		{"foreign host gone", `{"pid":42,"host":"foreign-fixture-host","start":"old"}`, platformprocess.ErrProcessGone, "", ownerLivenessIndeterminate},
		{"partial", `{"pid":42,"host":"local"}`, nil, "new", ownerLivenessIndeterminate},
		{"typed", `{"pid":42,"host":3,"start":"old"}`, nil, "new", ownerLivenessIndeterminate},
		{"null", `{"pid":42,"host":null,"start":null}`, nil, "new", ownerLivenessIndeterminate},
		{"empty", `{"pid":42,"host":"","start":""}`, nil, "new", ownerLivenessIndeterminate},
		{"invalid PID", `{"pid":0}`, nil, "new", ownerLivenessIndeterminate},
		{"malformed", `{`, nil, "new", ownerLivenessIndeterminate},
	} {
		t.Run(cell.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			definition := installationDefinitionFixture()
			staging := stagingOwnershipPath(root, definition.Name)
			if err := os.MkdirAll(staging, 0755); err != nil {
				t.Fatal(err)
			}
			metadata := filepath.Join(staging, stagingOwnerMetadataName)
			if err := os.WriteFile(metadata, []byte(cell.record), 0600); err != nil {
				t.Fatal(err)
			}
			probe := localOwnerProbe{incarnations: factorydefinitions.PackagedInstallationProcessProbe(func(pid int) (platformprocess.Incarnation, error) {
				if pid == os.Getpid() {
					return platformprocess.Incarnation{PID: pid, Host: host, Start: "self"}, nil
				}
				return platformprocess.Incarnation{PID: pid, Host: host, Start: cell.start}, cell.lookupErr
			})}
			service := newWithOwnerProbe(packagedInstallationTestPersistence(), platformfilesystem.Local{}, os.Mkdir, probe, logging.NoopLogger{})
			// A prior committed target must survive refusal; replacement is
			// permitted only after affirmative abandonment.
			target := filepath.Join(root, filepath.FromSlash(definition.Name))
			if err := os.MkdirAll(target, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(target, "factory.json"), []byte("prior"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := service.InstallPackagedFactory(t.Context(), factorydefinitions.PackagedFactoryInstallParams{NamedFactoriesRoot: root, Definition: definition, Format: factorydefinitions.PackagedFactoryFormatJSON, Replace: true})
			if cell.want == ownerLivenessOrphaned {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(staging); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("lease remains: %v", err)
				}
			} else {
				if !errors.Is(err, factorydefinitions.ErrFactoryInstallationContention) || !strings.Contains(err.Error(), "owner_liveness="+string(cell.want)) {
					t.Fatalf("contention: %v", err)
				}
				assertInstallationFileUnchanged(t, metadata, cell.record)
				assertInstallationFileUnchanged(t, filepath.Join(target, "factory.json"), "prior")
			}
		})
	}
}

func assertInstallationFileUnchanged(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("file mutated: %s %v", data, err)
	}
}

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
				fileSystem.Mkdir,
				probe,
				logging.NoopLogger{},
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
				fileSystem.Mkdir,
				probe,
				logging.NoopLogger{},
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
			_, err := newNativeTestInstaller(packagedInstallationTestPersistence(), fileSystem, fileSystem.Mkdir, logging.NoopLogger{}).InstallPackagedFactory(
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
				fileSystem.Mkdir,
				probe,
				logging.NoopLogger{},
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
	factorydefinitions.PackagedFactoryPersistence
}

func (persistence *successfulPackagedInstallationPersistence) PreparePackagedFactoryLayout(
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
