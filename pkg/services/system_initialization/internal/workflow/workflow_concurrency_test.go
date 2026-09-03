package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
)

type concurrentBootstrapSettings struct{}

func (concurrentBootstrapSettings) LoadFileConfig(string) (operatorsettings.Config, error) {
	return operatorsettings.Config{BackendScopeID: "local-concurrent-bootstrap"}, nil
}

func (concurrentBootstrapSettings) EnsureLocalBackendScope(string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("unexpected configuration creation")
}

type observingBootstrapInstaller struct {
	entered chan struct{}
	release chan struct{}
	active  atomic.Int32
	max     atomic.Int32
}

func (installer *observingBootstrapInstaller) EnsurePackagedFactories(
	ctx context.Context,
	_ string,
	_ string,
	_ []factorydefinitions.PackagedDefinition,
) ([]factorydefinitions.PackagedFactoryInstallResult, error) {
	active := installer.active.Add(1)
	for current := installer.max.Load(); active > current && !installer.max.CompareAndSwap(current, active); current = installer.max.Load() {
	}
	installer.entered <- struct{}{}
	select {
	case <-ctx.Done():
		installer.active.Add(-1)
		return nil, ctx.Err()
	case <-installer.release:
		installer.active.Add(-1)
		return nil, nil
	}
}

func TestInitializeSerializesOnlyBootstrapForTheSameCustomerHome(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := concurrentBootstrapConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create config parent: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installer := &observingBootstrapInstaller{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}, 2),
	}
	initializer := newTestInitializer(t, concurrentBootstrapSettings{}, installer, nil)
	results := make(chan error, 2)
	initialize := func() {
		_, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
		results <- err
	}

	go initialize()
	<-installer.entered
	go initialize()
	secondEnteredEarly := false
	select {
	case <-installer.entered:
		secondEnteredEarly = true
	case <-time.After(100 * time.Millisecond):
	}
	installer.release <- struct{}{}
	if !secondEnteredEarly {
		<-installer.entered
	}
	installer.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	}
	if secondEnteredEarly {
		t.Fatal("second bootstrap entered the same customer home before the first completed")
	}
	if got := installer.max.Load(); got != 1 {
		t.Fatalf("maximum concurrent bootstrap operations for one home = %d, want 1", got)
	}
}

func TestInitializeBootstrapsDifferentCustomerHomesConcurrently(t *testing.T) {
	t.Parallel()

	firstHome := t.TempDir()
	secondHome := t.TempDir()
	writeConcurrentBootstrapConfig(t, firstHome)
	writeConcurrentBootstrapConfig(t, secondHome)
	installer := &observingBootstrapInstaller{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}, 2),
	}
	initializer := newTestInitializer(t, concurrentBootstrapSettings{}, installer, nil)
	results := make(chan error, 2)
	for _, homeDir := range []string{firstHome, secondHome} {
		homeDir := homeDir
		go func() {
			_, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
			results <- err
		}()
	}

	for range 2 {
		select {
		case <-installer.entered:
		case <-time.After(time.Second):
			t.Fatal("different customer homes did not enter bootstrap concurrently")
		}
	}
	installer.release <- struct{}{}
	installer.release <- struct{}{}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
	}
	if got := installer.max.Load(); got != 2 {
		t.Fatalf("maximum concurrent bootstrap operations for different homes = %d, want 2", got)
	}
}

func TestInitializeWaitingForCustomerHomeHonorsCancellation(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := concurrentBootstrapConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create config parent: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	installer := &observingBootstrapInstaller{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}, 1),
	}
	initializer := newTestInitializer(t, concurrentBootstrapSettings{}, installer, nil)
	firstResult := make(chan error, 1)
	go func() {
		_, err := initializer.Initialize(t.Context(), systeminitialization.Request{HomeDir: homeDir})
		firstResult <- err
	}()
	<-installer.entered

	waitingContext, cancelWaiting := context.WithCancel(t.Context())
	secondResult := make(chan error, 1)
	go func() {
		_, err := initializer.Initialize(waitingContext, systeminitialization.Request{HomeDir: homeDir})
		secondResult <- err
	}()
	waitForBootstrapHomeUsers(t, initializer, filepath.Clean(homeDir), 2)
	cancelWaiting()

	select {
	case err := <-secondResult:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, systeminitialization.ErrInitializeCancelled) {
			t.Fatalf("waiting Initialize() error = %v, want initialization cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting Initialize did not honor cancellation while first bootstrap remained active")
	}
	installer.release <- struct{}{}
	if err := <-firstResult; err != nil {
		t.Fatalf("first Initialize() error = %v", err)
	}
}

func waitForBootstrapHomeUsers(t *testing.T, initializer *Initializer, homeDir string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		initializer.homeGatesMu.Lock()
		gate := initializer.homeGates[homeDir]
		got := 0
		if gate != nil {
			got = gate.users
		}
		initializer.homeGatesMu.Unlock()
		if got == want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("bootstrap users for %q did not reach %d", homeDir, want)
}

func writeConcurrentBootstrapConfig(t *testing.T, homeDir string) {
	t.Helper()
	configPath := concurrentBootstrapConfigPath(homeDir)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("create config parent: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func concurrentBootstrapConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".you-agent-factory", "config.json")
}
