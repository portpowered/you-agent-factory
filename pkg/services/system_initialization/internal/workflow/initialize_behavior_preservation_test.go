package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

// TestInitializePreservesIdempotentRepeatFactsThroughRootCollaborator proves
// repeat Initialize on an already-initialized home reports skipped system-config
// outcome and does not rewrite customer-owned operator config, routing load-only
// Settings commands through the injected root collaborator on the second run.
func TestInitializePreservesIdempotentRepeatFactsThroughRootCollaborator(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	settings := &settingsCommandRecorder{}
	initializer := newTestInitializer(t, settings, &fakePackagedInstaller{}, nil)

	first, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
	if first.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("first SystemConfigOutcome = %q, want created", first.SystemConfigOutcome)
	}

	wantConfigPath := operatorsettings.DefaultConfigPath(homeDir)
	customerConfig := []byte(`{"customer":"owned"}`)
	if err := os.WriteFile(wantConfigPath, customerConfig, 0o600); err != nil {
		t.Fatal(err)
	}

	second, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
	if err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	if second.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("second SystemConfigOutcome = %q, want skipped", second.SystemConfigOutcome)
	}

	after, err := os.ReadFile(wantConfigPath)
	if err != nil {
		t.Fatalf("read config after repeat = %v", err)
	}
	if string(after) != string(customerConfig) {
		t.Fatalf("operator config rewritten on repeat: before %q after %q", customerConfig, after)
	}

	if len(settings.ensureCalls) != 1 || settings.ensureCalls[0] != wantConfigPath {
		t.Fatalf("EnsureLocalBackendScope calls = %#v, want one create at %q", settings.ensureCalls, wantConfigPath)
	}
	if len(settings.loadCalls) != 2 ||
		settings.loadCalls[0] != wantConfigPath ||
		settings.loadCalls[1] != wantConfigPath {
		t.Fatalf("LoadFileConfig calls = %#v, want load on both invocations at %q", settings.loadCalls, wantConfigPath)
	}
}

// TestInitializeSettingsFailurePreservesPartialFailureRollbackFactsThroughRootCollaborator
// proves Settings load/ensure failures routed through the root collaborator still
// surface Bootstrap ErrInitializePartialFailure with inspectable rollback facts.
func TestInitializeSettingsFailurePreservesPartialFailureRollbackFactsThroughRootCollaborator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		prepare    func(t *testing.T, homeDir, configPath string)
		settings   *settingsCommandRecorder
		wantCause  string
		wantEnsure int
		wantLoad   int
	}{
		{
			name: "ensure failure on create path",
			settings: &settingsCommandRecorder{
				ensureErr: errors.New("ensure denied"),
			},
			wantCause:  "create system config",
			wantEnsure: 1,
		},
		{
			name: "load failure on existing config skip path",
			prepare: func(t *testing.T, homeDir, configPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(configPath, []byte(`{"customer":"owned"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			settings: &settingsCommandRecorder{
				loadErr: errors.New("load denied"),
			},
			wantCause:  "read existing operator config",
			wantLoad:   1,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			homeDir := t.TempDir()
			configPath := operatorsettings.DefaultConfigPath(homeDir)
			if test.prepare != nil {
				test.prepare(t, homeDir, configPath)
			}

			_, err := newTestInitializer(t, test.settings, &fakePackagedInstaller{}, nil).
				Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
			if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
				t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
			}
			var partialFailure systeminitialization.InitializePartialFailure
			if !errors.As(err, &partialFailure) {
				t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
			}
			if !strings.Contains(partialFailure.Cause.Error(), test.wantCause) {
				t.Fatalf("Initialize() cause = %v, want substring %q", partialFailure.Cause, test.wantCause)
			}
			if len(partialFailure.Facts) != 2 ||
				partialFailure.Facts[0].Step != systeminitialization.InitializeStepLegacyMigration ||
				partialFailure.Facts[0].Outcome != systeminitialization.RollbackStepCompleted ||
				partialFailure.Facts[1].Step != systeminitialization.InitializeStepSystemConfig ||
				partialFailure.Facts[1].Outcome != systeminitialization.RollbackStepUnresolved {
				t.Fatalf("Initialize() rollback facts = %#v", partialFailure.Facts)
			}

			if len(test.settings.ensureCalls) != test.wantEnsure {
				t.Fatalf("EnsureLocalBackendScope calls = %d, want %d", len(test.settings.ensureCalls), test.wantEnsure)
			}
			if len(test.settings.loadCalls) != test.wantLoad {
				t.Fatalf("LoadFileConfig calls = %d, want %d", len(test.settings.loadCalls), test.wantLoad)
			}
		})
	}
}

// TestInitializeValidationAndCancellationPreserveNoRollbackFactsThroughRootCollaborator
// proves validation and cancellation failures do not invent Settings rollback work
// facts and do not invoke Settings collaborator commands.
func TestInitializeValidationAndCancellationPreserveNoRollbackFactsThroughRootCollaborator(t *testing.T) {
	t.Parallel()

	settings := &settingsCommandRecorder{}

	_, validationErr := newTestInitializer(t, settings, &fakePackagedInstaller{}, nil).
		Initialize(t.Context(), systeminitialization.Request{HomeDir: "  "})
	if !errors.Is(validationErr, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("validation error = %v, want ErrMissingHomeDir", validationErr)
	}
	var validationPartialFailure systeminitialization.InitializePartialFailure
	if errors.As(validationErr, &validationPartialFailure) {
		t.Fatalf("validation error = %v, want no rollback facts", validationErr)
	}
	if len(settings.loadCalls) != 0 || len(settings.ensureCalls) != 0 {
		t.Fatalf("validation invoked Settings collaborator: load=%#v ensure=%#v", settings.loadCalls, settings.ensureCalls)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, cancellationErr := newTestInitializer(t, settings, &fakePackagedInstaller{}, nil).
		Initialize(ctx, systeminitialization.Request{HomeDir: t.TempDir()})
	if !errors.Is(cancellationErr, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("cancellation error = %v, want ErrInitializeCancelled", cancellationErr)
	}
	var cancellationPartialFailure systeminitialization.InitializePartialFailure
	if errors.As(cancellationErr, &cancellationPartialFailure) {
		t.Fatalf("cancellation error = %v, want no rollback facts", cancellationErr)
	}
	if len(settings.loadCalls) != 0 || len(settings.ensureCalls) != 0 {
		t.Fatalf("cancellation invoked Settings collaborator: load=%#v ensure=%#v", settings.loadCalls, settings.ensureCalls)
	}
}
