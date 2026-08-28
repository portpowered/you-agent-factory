package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const sharedDefinitionsServiceHostShutdownTimeout = 15 * time.Second

// sharedDefinitionsServiceHost owns one continuous service-mode invocation.
// The host Factory and process wiring are shared only by scenarios that use
// the same public service boundary; each scenario still owns its project,
// HOME, session, request payload, and captured streams.
type sharedDefinitionsServiceHost struct {
	process    support.ApplicationProcess
	baseURL    string
	factoryDir string
	homeDir    string
	env        []string
	provider   *support.RecordingCommandRunner
	cancel     context.CancelFunc
	done       <-chan error
}

var (
	sharedDefinitionsValidationHostOnce sync.Once
	sharedDefinitionsValidationHost     *sharedDefinitionsServiceHost
	sharedDefinitionsValidationHostErr  error
	sharedDefinitionsValidationReady    sync.Once

	sharedDefinitionsInitHostOnce sync.Once
	sharedDefinitionsInitHost     *sharedDefinitionsServiceHost
	sharedDefinitionsInitHostErr  error
	sharedDefinitionsInitReady    sync.Once

	sharedDefinitionsInitClientOnce     sync.Once
	sharedDefinitionsInitClient         support.ApplicationProcess
	sharedDefinitionsInitClientErr      error
	sharedDefinitionsInitClientCloseErr error
)

func sharedDefinitionsValidationServer(t testing.TB) *sharedDefinitionsServiceHost {
	t.Helper()
	sharedDefinitionsValidationHostOnce.Do(func() {
		sharedDefinitionsValidationHost, sharedDefinitionsValidationHostErr = startSharedDefinitionsServiceHost(
			validAPIValidationFactoryConfig(),
			"unexpected provider invocation in shared Factory Definitions validation host",
		)
	})
	if sharedDefinitionsValidationHostErr != nil {
		t.Fatalf("start shared Factory Definitions validation host: %v", sharedDefinitionsValidationHostErr)
	}
	if sharedDefinitionsValidationHost == nil {
		t.Fatal("shared Factory Definitions validation host is unavailable")
	}
	sharedDefinitionsValidationReady.Do(func() {
		support.WaitForStatus(t, sharedDefinitionsValidationHost.baseURL, sharedDefinitionsServiceHostShutdownTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
	})
	return sharedDefinitionsValidationHost
}

func sharedDefinitionsInitServer(t testing.TB) *sharedDefinitionsServiceHost {
	t.Helper()
	sharedDefinitionsInitHostOnce.Do(func() {
		sharedDefinitionsInitHost, sharedDefinitionsInitHostErr = startSharedDefinitionsServiceHost(
			initHostFactoryConfig(),
			"unexpected provider invocation in shared Factory Definitions init host",
		)
	})
	if sharedDefinitionsInitHostErr != nil {
		t.Fatalf("start shared Factory Definitions init host: %v", sharedDefinitionsInitHostErr)
	}
	if sharedDefinitionsInitHost == nil {
		t.Fatal("shared Factory Definitions init host is unavailable")
	}
	sharedDefinitionsInitReady.Do(func() {
		support.WaitForStatus(t, sharedDefinitionsInitHost.baseURL, sharedDefinitionsServiceHostShutdownTimeout, func(status factoryapi.StatusResponse) bool {
			return status.RuntimeStatus != ""
		})
	})
	return sharedDefinitionsInitHost
}

func sharedDefinitionsInitProcess(t testing.TB) support.ApplicationProcess {
	t.Helper()
	sharedDefinitionsInitClientOnce.Do(func() {
		sharedDefinitionsInitClient, sharedDefinitionsInitClientErr = support.BuildProcessWithContext(
			context.Background(), serviceedges.Edges{},
		)
	})
	if sharedDefinitionsInitClientErr != nil {
		t.Fatalf("build shared Factory Definitions init client: %v", sharedDefinitionsInitClientErr)
	}
	if sharedDefinitionsInitClient == nil {
		t.Fatal("shared Factory Definitions init client is unavailable")
	}
	return sharedDefinitionsInitClient
}

func (host *sharedDefinitionsServiceHost) URL() string {
	if host == nil {
		return ""
	}
	return host.baseURL
}

func startSharedDefinitionsServiceHost(
	cfg map[string]any,
	providerError string,
) (*sharedDefinitionsServiceHost, error) {
	factoryDir, err := writeSharedDefinitionsFactory(cfg)
	if err != nil {
		return nil, err
	}
	homeDir, err := os.MkdirTemp("", "c05-factory-definitions-home-")
	if err != nil {
		_ = os.RemoveAll(factoryDir)
		return nil, fmt.Errorf("create shared service home: %w", err)
	}

	provider := support.NewRecordingCommandRunner(providerError)
	api := support.NewProcessAPIServer()
	processContext, cancel := context.WithCancel(context.Background())
	process, err := support.BuildProcessWithContext(processContext, serviceedges.Edges{
		APIServerStarter:      api.Start,
		ProviderCommandRunner: provider,
	})
	if err != nil {
		cancel()
		_ = os.RemoveAll(factoryDir)
		_ = os.RemoveAll(homeDir)
		return nil, fmt.Errorf("build shared service root process: %w", err)
	}

	inputs := support.FakeInputs(processContext, []string{
		"you", "run", "--continuously", "--with-server", "--quiet", "--dir", factoryDir, "--no-record",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = factoryDir
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(inputs.Input)
	}()

	baseURL, err := api.WaitForBaseURL(sharedDefinitionsServiceHostShutdownTimeout)
	if err != nil {
		_ = stopSharedDefinitionsServiceHost(&sharedDefinitionsServiceHost{
			process: process, factoryDir: factoryDir, homeDir: homeDir,
			cancel: cancel, done: done,
		})
		return nil, fmt.Errorf("wait for shared service API server: %w", err)
	}

	return &sharedDefinitionsServiceHost{
		process:    process,
		baseURL:    baseURL,
		factoryDir: factoryDir,
		homeDir:    homeDir,
		env:        append([]string(nil), inputs.Input.Env...),
		provider:   provider,
		cancel:     cancel,
		done:       done,
	}, nil
}

func writeSharedDefinitionsFactory(cfg map[string]any) (string, error) {
	factoryDir, err := os.MkdirTemp("", "c05-factory-definitions-factory-")
	if err != nil {
		return "", fmt.Errorf("create shared service Factory directory: %w", err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		_ = os.RemoveAll(factoryDir)
		return "", fmt.Errorf("marshal shared service Factory config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), raw, 0o644); err != nil {
		_ = os.RemoveAll(factoryDir)
		return "", fmt.Errorf("write shared service Factory config: %w", err)
	}

	workstations, ok := cfg["workstations"].([]map[string]any)
	if !ok {
		return factoryDir, nil
	}
	for _, workstation := range workstations {
		name, _ := workstation["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		workstationDir := filepath.Join(factoryDir, "workstations", name)
		if err := os.MkdirAll(workstationDir, 0o755); err != nil {
			_ = os.RemoveAll(factoryDir)
			return "", fmt.Errorf("create shared workstation %q: %w", name, err)
		}
		if err := os.WriteFile(
			filepath.Join(workstationDir, "AGENTS.md"),
			[]byte("---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"),
			0o644,
		); err != nil {
			_ = os.RemoveAll(factoryDir)
			return "", fmt.Errorf("write shared workstation %q: %w", name, err)
		}
	}
	return factoryDir, nil
}

func closeSharedDefinitionsServiceHosts() error {
	sharedDefinitionsInitClientCloseErr = nil
	var failures []string
	for name, host := range map[string]*sharedDefinitionsServiceHost{
		"validation": sharedDefinitionsValidationHost,
		"init":       sharedDefinitionsInitHost,
	} {
		if err := stopSharedDefinitionsServiceHost(host); err != nil {
			failures = append(failures, fmt.Sprintf("%s host: %v", name, err))
		}
	}
	if sharedDefinitionsInitClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedDefinitionsServiceHostShutdownTimeout)
		sharedDefinitionsInitClientCloseErr = sharedDefinitionsInitClient.Close(ctx)
		if sharedDefinitionsInitClientCloseErr != nil {
			failures = append(failures, fmt.Sprintf("init client: %v", sharedDefinitionsInitClientCloseErr))
		}
		cancel()
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

func stopSharedDefinitionsServiceHost(host *sharedDefinitionsServiceHost) error {
	if host == nil {
		return nil
	}
	if host.cancel != nil {
		host.cancel()
	}
	var failures []string
	if host.done != nil {
		select {
		case err := <-host.done:
			if err != nil && !errors.Is(err, context.Canceled) {
				failures = append(failures, fmt.Sprintf("Execute: %v", err))
			}
		case <-time.After(sharedDefinitionsServiceHostShutdownTimeout):
			failures = append(failures, "timed out waiting for Execute shutdown")
		}
	}
	if host.process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), sharedDefinitionsServiceHostShutdownTimeout)
		if err := host.process.Close(ctx); err != nil {
			failures = append(failures, fmt.Sprintf("close process: %v", err))
		}
		cancel()
	}
	if err := os.RemoveAll(host.factoryDir); err != nil {
		failures = append(failures, fmt.Sprintf("remove Factory directory: %v", err))
	}
	if err := os.RemoveAll(host.homeDir); err != nil {
		failures = append(failures, fmt.Sprintf("remove home directory: %v", err))
	}
	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}
