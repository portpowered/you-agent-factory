package runtimeopening

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	operatorconfig 	"github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"go.uber.org/zap"
)

func TestLoadRuntimePreservesValidatedPortableRecording(t *testing.T) {
	path := filepath.Join(
		"..", "..", "..", "recordings", "artifacts", "testdata", "valid-v2.json",
	)
	rootDir := t.TempDir()
	var loggerSessionID string
	var loggerFolderPath string
	var loggerFactoryDir string
	loaded, err := LoadRuntime(
		t.TempDir(),
		"",
		path,
		operatorconfig.ResolvedDefaults{},
		nil,
		RuntimeRoot{FactoryRootDir: rootDir, BaseLogger: zap.NewNop()},
		nil,
		nil,
		nil,
		nil,
		nil,
		func(base *zap.Logger, sessionID, folderPath, factoryDir string) *zap.Logger {
			loggerSessionID = sessionID
			loggerFolderPath = folderPath
			loggerFactoryDir = factoryDir
			return base
		},
		fileeffects.ReplayRecordingReader(os.ReadFile),
	)
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}
	if loaded.PortableRecording == nil {
		t.Fatal("portable recording = nil")
	}
	if got := loaded.PortableRecording.Session.ID; got != "session-js-001" {
		t.Fatalf("session id = %q, want session-js-001", got)
	}
	if loaded.ReplayArtifact != nil || loaded.LoadedFactoryCfg != nil {
		t.Fatal("portable recording mixed with Factory-event replay state")
	}
	if loggerSessionID != "~default" || loggerFolderPath != rootDir || loggerFactoryDir == "" {
		t.Fatalf(
			"logger identity = (%q, %q, %q), want injected session and directories",
			loggerSessionID, loggerFolderPath, loggerFactoryDir,
		)
	}
}

func TestPortableRecordingReplayRequiresAndUsesInjectedReader(t *testing.T) {
	t.Parallel()

	if _, _, err := loadPortableRecordingReplay("recording.json", nil); err == nil {
		t.Fatal("missing replay recording reader error = nil")
	}
	want := errors.New("recording read unavailable")
	_, _, err := loadPortableRecordingReplay("recording.json", func(path string) ([]byte, error) {
		if path != "recording.json" {
			t.Fatalf("path = %q, want recording.json", path)
		}
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("loadPortableRecordingReplay error = %v, want %v", err, want)
	}
}

func TestClockForReplayPreservesOverridesAndInjectedDefaults(t *testing.T) {
	explicit := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 1, 0, 0, 0, time.UTC))
	replay := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 2, 0, 0, 0, time.UTC))
	fallback := clockwork.NewFakeClockAt(time.Date(2026, time.July, 20, 3, 0, 0, 0, time.UTC))
	artifact := &factorydefinitions.ReplayArtifact{}

	selected, err := clockForReplay(
		explicit,
		artifact,
		func(*factorydefinitions.ReplayArtifact) recordings.Clock { return replay },
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != explicit.Now() {
		t.Fatalf("explicit clock = (%v, %v), want preserved override", selected, err)
	}

	selected, err = clockForReplay(
		nil,
		artifact,
		func(*factorydefinitions.ReplayArtifact) recordings.Clock { return replay },
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != replay.Now() {
		t.Fatalf("replay clock = (%v, %v), want replay-selected clock", selected, err)
	}

	selected, err = clockForReplay(
		nil,
		nil,
		nil,
		func(clock factoryruntime.Clock) factoryruntime.Clock {
			if clock == nil {
				return fallback
			}
			return clock
		},
	)
	if err != nil || selected.Now() != fallback.Now() {
		t.Fatalf("default clock = (%v, %v), want injected fallback", selected, err)
	}
}

func TestClockForReplayRejectsMissingOrNilResolver(t *testing.T) {
	if _, err := clockForReplay(nil, nil, nil, nil); err == nil {
		t.Fatal("missing clock resolver error = nil")
	}
	if _, err := clockForReplay(
		nil, nil, nil,
		func(factoryruntime.Clock) factoryruntime.Clock { return nil },
	); err == nil {
		t.Fatal("nil resolved clock error = nil")
	}
}

func TestLoadRuntimeRejectsMissingOrNilSessionLoggerFactory(t *testing.T) {
	root := RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: zap.NewNop()}
	if _, err := LoadRuntime("", "", "", operatorconfig.ResolvedDefaults{}, nil, root, nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("missing session logger factory error = nil")
	}
	if _, err := LoadRuntime(
		"", "", "", operatorconfig.ResolvedDefaults{}, nil, root,
		nil, nil, nil, nil, nil,
		func(*zap.Logger, string, string, string) *zap.Logger { return nil },
		nil,
	); err == nil {
		t.Fatal("nil session logger error = nil")
	}
}

func TestNewDurableExecutionCanonicalizesOperatorDefaultsAndPresets(t *testing.T) {
	var got factoryruntime.JavaScriptWorkerSettings
	executionFactory := func(
		_ string,
		_ factorysessions.PersistencePolicy,
		_ workerprovider.Provider,
		_ factoryruntime.Clock,
		_ map[string]struct{},
		settings factoryruntime.JavaScriptWorkerSettings,
		_ *workers.MockWorkersConfig,
	) (factorysessions.ExecutionService, error) {
		got = settings
		return nil, nil
	}
	_, err := NewDurableExecution(
		func(string) (operatorconfig.Config, error) {
			return operatorconfig.Config{
				Defaults: operatorconfig.Defaults{WorkerModelProvider: "customer"},
				WorkerPresets: []operatorconfig.WorkerPreset{{
					ID: "review", ModelProvider: "agent",
				}},
			}, nil
		},
		factorydefinitions.RuntimeOpeningRequest{Directory: t.TempDir()},
		factorysessions.SessionRuntimeOpeningRequest{SystemConfigHome: t.TempDir()},
		RuntimeRoot{FactoryRootDir: t.TempDir()},
		nil,
		nil,
		nil,
		executionFactory,
		factorysessions.ProviderIdentityResolver(func(identity string) (string, error) {
			switch identity {
			case "customer":
				return "customer.provider", nil
			case "agent":
				return "cursor", nil
			default:
				return "", errors.New("unexpected provider")
			}
		}),
	)
	if err != nil {
		t.Fatalf("NewDurableExecution: %v", err)
	}
	if got.DefaultModelProvider != "customer.provider" {
		t.Fatalf("default provider = %q, want canonical extension identity", got.DefaultModelProvider)
	}
	if preset := got.Presets["review"]; preset.ModelProvider != "cursor" {
		t.Fatalf("review preset = %#v, want canonical cursor identity", preset)
	}
}
