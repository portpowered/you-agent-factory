package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	startupTimeout = 30 * time.Second
	healthMethod   = "/backend.Backend/Health"
)

type healthResult struct {
	err error
}

func main() {
	binary := flag.String("binary", "", "path to the packaged backend executable")
	workDir := flag.String("workdir", "", "directory containing the packaged backend executable")
	timeout := flag.Duration("timeout", startupTimeout, "maximum time allowed for startup and health negotiation")
	flag.Parse()

	if *binary == "" {
		fatal("-binary is required")
	}
	if *timeout <= 0 {
		fatal("-timeout must be positive")
	}
	if _, err := os.Stat(*binary); err != nil {
		fatal("packaged backend executable is unavailable: %v", err)
	}

	if *workDir == "" {
		*workDir = filepath.Dir(*binary)
	}
	if err := runSmoke(*binary, *workDir, *timeout); err != nil {
		fatal("%v", err)
	}
}

func runSmoke(binary, workDir string, timeout time.Duration) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("reserve a loopback health port: %w", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return fmt.Errorf("release the loopback health port: %w", err)
	}

	command := exec.Command(binary, "--addr="+address)
	command.Dir = workDir
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start packaged backend: %w", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	defer func() {
		if command.ProcessState != nil {
			return
		}
		_ = command.Process.Kill()
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resultDone := make(chan healthResult, 1)
	go func() {
		connection, dialErr := grpc.DialContext(
			ctx,
			address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		)
		if dialErr != nil {
			resultDone <- healthResult{err: dialErr}
			return
		}
		defer func() { _ = connection.Close() }()

		// HealthMessage is empty on the wire. Empty also accepts the response's
		// forward-compatible Reply fields while still requiring a successful
		// invocation through the pinned Backend service method.
		invokeErr := connection.Invoke(ctx, healthMethod, &emptypb.Empty{}, &emptypb.Empty{})
		resultDone <- healthResult{err: invokeErr}
	}()

	select {
	case processErr := <-waitDone:
		if processErr == nil {
			return fmt.Errorf("packaged backend exited before health negotiation")
		}
		return fmt.Errorf("packaged backend exited before health negotiation: %w", processErr)
	case result := <-resultDone:
		if result.err != nil {
			return fmt.Errorf("health negotiation failed: %w", result.err)
		}
		fmt.Printf(
			"LOCALAI_BACKEND_STARTUP_OK binary=%s address=%s health=ok\n",
			binary,
			address,
		)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("health negotiation timed out after %s: %w", timeout, ctx.Err())
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "localai-backend-startup-smoke: "+format+"\n", args...)
	os.Exit(1)
}
