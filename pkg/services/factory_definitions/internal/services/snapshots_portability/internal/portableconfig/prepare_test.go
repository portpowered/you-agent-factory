package portableconfig_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	snapshotsportabilityprepare "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/internal/prepare"
)

func TestNewPreparerClonesAndAppliesPortableContentInOrder(t *testing.T) {
	original := &factorydefinitions.FactoryConfig{Name: "authored"}
	var calls []string
	prepare := snapshotsportabilityprepare.NewPreparer(
		func(factory *factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error) {
			calls = append(calls, "clone")
			return &factorydefinitions.FactoryConfig{Name: factory.Name + "-clone"}, nil
		},
		func(
			factoryDir string,
			factory *factorydefinitions.FactoryConfig,
			includeInlineContent bool,
			overwrite bool,
		) error {
			calls = append(calls, "bundled")
			if factoryDir != "/factory" || !includeInlineContent || overwrite {
				t.Fatalf(
					"bundled arguments = (%q, %v, %v)",
					factoryDir,
					includeInlineContent,
					overwrite,
				)
			}
			factory.Project = "portable"
			return nil
		},
		func(factoryDir string, factory *factorydefinitions.FactoryConfig) error {
			calls = append(calls, "starter-work")
			if factoryDir != "/factory" || factory.Project != "portable" {
				t.Fatalf("starter Work received (%q, %#v)", factoryDir, factory)
			}
			return nil
		},
	)

	got, err := prepare("/factory", original, true)
	if err != nil {
		t.Fatalf("prepare portable Factory: %v", err)
	}
	if !reflect.DeepEqual(calls, []string{"clone", "bundled", "starter-work"}) {
		t.Fatalf("calls = %v", calls)
	}
	if got == original || got.Name != "authored-clone" || got.Project != "portable" {
		t.Fatalf("prepared Factory = %#v", got)
	}
	if original.Project != "" {
		t.Fatalf("original Factory was mutated: %#v", original)
	}
}

func TestPrepareRequiresEveryInjectedOperation(t *testing.T) {
	clone := func(factory *factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error) {
		return factory, nil
	}
	bundled := func(string, *factorydefinitions.FactoryConfig, bool, bool) error {
		return nil
	}
	starter := func(string, *factorydefinitions.FactoryConfig) error {
		return nil
	}
	tests := []struct {
		name    string
		clone   factorydefinitions.FactoryConfigCloner
		bundled factorydefinitions.PortableBundledFilesApplier
		starter factorydefinitions.FactoryStarterWorkApplier
		want    string
	}{
		{name: "cloner", bundled: bundled, starter: starter, want: "cloner is required"},
		{name: "bundled files", clone: clone, starter: starter, want: "applier is required"},
		{name: "starter Work", clone: clone, bundled: bundled, want: "applier is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := snapshotsportabilityprepare.PrepareConfig(
				"/factory",
				&factorydefinitions.FactoryConfig{},
				false,
				test.clone,
				test.bundled,
				test.starter,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Prepare error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestPrepareStopsAfterOperationFailure(t *testing.T) {
	sentinel := errors.New("portable files failed")
	starterCalled := false
	_, err := snapshotsportabilityprepare.PrepareConfig(
		"/factory",
		&factorydefinitions.FactoryConfig{},
		false,
		func(factory *factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error) {
			return factory, nil
		},
		func(string, *factorydefinitions.FactoryConfig, bool, bool) error {
			return sentinel
		},
		func(string, *factorydefinitions.FactoryConfig) error {
			starterCalled = true
			return nil
		},
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Prepare error = %v, want %v", err, sentinel)
	}
	if starterCalled {
		t.Fatal("starter Work ran after portable bundled-file failure")
	}
}
