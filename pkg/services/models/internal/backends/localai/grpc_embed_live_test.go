package localai

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

const (
	liveEmbedBackendDirEnv = "YOU_LOCALAI_LLAMA_CPP_BACKEND_DIR"
	liveEmbedGGUFPathEnv   = "YOU_LOCALAI_EMBED_GGUF_PATH"
	liveEmbedOutputLimit   = 16 * 1024
)

// TestLiveInstalledLlamaCPPEmbedLoad verifies the managed Linux Health and
// LoadModel exchange against an explicitly selected, installed backend.
func TestLiveInstalledLlamaCPPEmbedLoad(t *testing.T) {
	backendDir, modelPath := liveEmbedPaths(t)
	address := reserveLiveEmbedAddress(t)
	child := startLiveEmbedBackend(t, backendDir, address)
	defer child.stop()
	negotiator := NewPinnedGRPCHostProtocolNegotiator(platformgrpc.NetworkDialer{})
	configuration := modelseffects.ResolvedHostConfiguration{
		ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
		Backend:         "localai-llamacpp",
		Platform:        models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64", Accelerator: "cuda"},
		ModelName:       models.BuiltInModelNameEmbed,
		ModelPath:       modelPath,
	}
	awaitLiveEmbedHealth(t, child, negotiator, address, configuration)
	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelLoad()
	result, err := negotiator.Negotiate(loadCtx, address, modelseffects.HostProtocolNegotiationRequest{Configuration: configuration})
	if err != nil {
		child.fail(t, "Health+LoadModel", err)
	}
	if !result.Ready || result.Backend != configuration.Backend || result.ProtocolVersion != configuration.ProtocolVersion {
		child.fail(t, "Health+LoadModel", fmt.Errorf("unexpected negotiation result: %+v", result))
	}
}

func liveEmbedPaths(t *testing.T) (string, string) {
	t.Helper()
	backendDir := strings.TrimSpace(os.Getenv(liveEmbedBackendDirEnv))
	modelPath := strings.TrimSpace(os.Getenv(liveEmbedGGUFPathEnv))
	if backendDir == "" && modelPath == "" {
		t.Skip("set both live LocalAI backend and GGUF environment variables")
	}
	if backendDir == "" || modelPath == "" {
		t.Fatalf("set both %s and %s", liveEmbedBackendDirEnv, liveEmbedGGUFPathEnv)
	}
	if runtime.GOOS != "linux" {
		t.Skip("installed LocalAI executable requires Linux")
	}
	if !filepath.IsAbs(backendDir) || !filepath.IsAbs(modelPath) {
		t.Fatal("backend directory and GGUF path must be absolute")
	}
	if info, err := os.Stat(modelPath); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("GGUF file %q is unavailable or not regular: %v", modelPath, err)
	}
	return backendDir, modelPath
}

func reserveLiveEmbedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

type liveEmbedBackend struct {
	command *exec.Cmd
	cancel  context.CancelFunc
	done    chan error
	stdout  *liveEmbedTail
	stderr  *liveEmbedTail
	once    sync.Once
	waitErr error
}

func startLiveEmbedBackend(t *testing.T, backendDir, address string) *liveEmbedBackend {
	t.Helper()
	executable := filepath.Join(backendDir, "llama-cpp-grpc")
	assertLiveEmbedExecutable(t, executable)
	commandPath, commandArgs := liveEmbedEntrypoint(t, backendDir, executable, address)
	childCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	command := exec.CommandContext(childCtx, commandPath, commandArgs...)
	command.WaitDelay = 5 * time.Second
	command.Dir = backendDir
	command.Env = liveEmbedEnvironment(backendDir)
	child := &liveEmbedBackend{command: command, cancel: cancel, done: make(chan error, 1), stdout: &liveEmbedTail{}, stderr: &liveEmbedTail{}}
	command.Stdout, command.Stderr = child.stdout, child.stderr
	if err := command.Start(); err != nil {
		cancel()
		t.Fatalf("start installed LocalAI backend: %v", err)
	}
	go func() { child.done <- command.Wait() }()
	return child
}

func assertLiveEmbedExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed executable %q is unavailable or not executable: %v", path, err)
	}
}

func liveEmbedEntrypoint(t *testing.T, backendDir, executable, address string) (string, []string) {
	t.Helper()
	loader := filepath.Join(backendDir, "lib", "ld.so")
	if _, err := os.Lstat(loader); os.IsNotExist(err) {
		return executable, []string{"--addr=" + address}
	} else if err != nil {
		t.Fatalf("inspect installed loader: %v", err)
	}
	assertLiveEmbedExecutable(t, loader)
	return loader, []string{executable, "--addr=" + address}
}

func liveEmbedEnvironment(backendDir string) []string {
	environment := withoutEnvironmentKey(os.Environ(), "LLAMACPP_GRPC_SERVERS")
	libraryPath := filepath.Join(backendDir, "lib")
	if existing := os.Getenv("LD_LIBRARY_PATH"); existing != "" {
		libraryPath += ":" + existing
	}
	return append(withoutEnvironmentKey(environment, "LD_LIBRARY_PATH"), "LD_LIBRARY_PATH="+libraryPath)
}

func (child *liveEmbedBackend) stop() {
	child.once.Do(func() {
		child.cancel()
		child.waitErr = <-child.done
	})
}

func (child *liveEmbedBackend) fail(t *testing.T, stage string, cause error) {
	t.Helper()
	child.stop()
	status := "unavailable"
	if child.command.ProcessState != nil {
		status = fmt.Sprintf("%d", child.command.ProcessState.ExitCode())
	}
	t.Fatalf("LocalAI %s failed: %v; child exit status=%s (wait: %v)\nchild stderr (last %d bytes):\n%s\nchild stdout (last %d bytes):\n%s",
		stage, cause, status, child.waitErr, liveEmbedOutputLimit, child.stderr.String(), liveEmbedOutputLimit, child.stdout.String())
}

func awaitLiveEmbedHealth(t *testing.T, child *liveEmbedBackend, negotiator modelseffects.HostProtocolNegotiator, address string, configuration modelseffects.ResolvedHostConfiguration) {
	t.Helper()
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelStartup()
	for {
		probeCtx, cancelProbe := context.WithTimeout(startupCtx, time.Second)
		_, err := negotiator.Negotiate(probeCtx, address, modelseffects.HostProtocolNegotiationRequest{
			Configuration: modelseffects.ResolvedHostConfiguration{
				ProtocolVersion: configuration.ProtocolVersion,
				Backend:         configuration.Backend,
			},
		})
		cancelProbe()
		if err == nil {
			return
		}
		select {
		case <-startupCtx.Done():
			child.fail(t, "startup Health", fmt.Errorf("%w: last probe: %v", startupCtx.Err(), err))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

type liveEmbedTail struct {
	mu   sync.Mutex
	data []byte
}

func (tail *liveEmbedTail) Write(p []byte) (int, error) {
	tail.mu.Lock()
	defer tail.mu.Unlock()
	tail.data = append(tail.data, p...)
	if len(tail.data) > liveEmbedOutputLimit {
		tail.data = append([]byte(nil), tail.data[len(tail.data)-liveEmbedOutputLimit:]...)
	}
	return len(p), nil
}

func (tail *liveEmbedTail) String() string {
	tail.mu.Lock()
	defer tail.mu.Unlock()
	return string(tail.data)
}

func withoutEnvironmentKey(environment []string, key string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if name, _, ok := strings.Cut(entry, "="); !ok || name != key {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
