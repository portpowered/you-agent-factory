package wire

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/wire/internal/managedbackend"
)

type localAILiveCommandRunner struct{}

func (localAILiveCommandRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	command := exec.CommandContext(ctx, request.Command, request.Args...)
	output, err := command.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); ok {
		return platformprocess.CommandResult{Stderr: output, ExitCode: exit.ExitCode()}, nil
	}
	return platformprocess.CommandResult{Stdout: output}, err
}

// This test uses LocalAI's real gallery only when its binary and a dedicated
// backend cache are explicitly supplied. The regular Go suite remains offline.
func TestLocalAIGalleryCUDAInstalledBackendLive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("LocalAI gallery CUDA is exercised by a Linux process")
	}
	binary := strings.TrimSpace(os.Getenv("LOCALAI_BINARY"))
	root := strings.TrimSpace(os.Getenv("YOU_LOCALAI_LIVE_BACKENDS_PATH"))
	if binary == "" || root == "" {
		t.Skip("set LOCALAI_BINARY and YOU_LOCALAI_LIVE_BACKENDS_PATH for the live gallery check")
	}
	platform := provideModelAssetHostPlatform(serviceedges.Edges{})
	if platform.Accelerator != "cuda" {
		t.Fatalf("host accelerator = %q, want CUDA", platform.Accelerator)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// This opt-in check probes the real external gallery command with the
	// explicitly selected installation directory, then its production launcher.
	result, err := (localAILiveCommandRunner{}).Run(ctx, platformprocess.CommandRequest{
		Command: binary, Args: []string{"backends", "install", "--backends-path=" + root, "cuda12-llama-cpp"},
	})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("install gallery CUDA backend: exit=%d error=%v", result.ExitCode, err)
	}
	directory := filepath.Join(root, "cuda12-llama-cpp")
	launch, err := managedbackend.ResolveManagedBackendLaunch(ctx, serviceedges.HostProcessStartSpec{
		Backend: "localai-llamacpp", BackendFiles: []string{directory},
	})
	if err != nil {
		t.Fatalf("resolve installed CUDA backend launch: %v", err)
	}
	assertLocalAICUDALaunch(t, launch, directory)
	startCtx, stop := context.WithTimeout(ctx, 30*time.Second)
	defer stop()
	command := exec.CommandContext(startCtx, launch.Command, launch.Args...)
	command.Dir = launch.WorkDir
	command.Env = append(os.Environ(), launch.Env...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatalf("start CUDA backend: %v", err)
	}
	defer func() { stop(); _ = command.Wait() }()
	address := strings.TrimPrefix(launch.Endpoint, "grpc://")
	for {
		connection, dialErr := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			break
		}
		if startCtx.Err() != nil {
			t.Fatalf("CUDA backend did not listen at %s: %v", address, dialErr)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func assertLocalAICUDALaunch(t *testing.T, launch managedbackend.ManagedBackendLaunch, directory string) {
	t.Helper()
	if launch.WorkDir != directory ||
		(launch.Command != filepath.Join(directory, "llama-cpp-grpc") && launch.Command != filepath.Join(directory, "lib", "ld.so")) ||
		len(launch.Env) != 1 || !strings.HasPrefix(launch.Env[0], "LD_LIBRARY_PATH="+filepath.Join(directory, "lib")) {
		t.Fatalf("CUDA backend launch = %#v", launch)
	}
}
