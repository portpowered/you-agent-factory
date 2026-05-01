package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"go.uber.org/zap"
)

func TestBuildReplacementFactoryRuntime_ServiceModeStaysRunningUntilCanceled(t *testing.T) {
	rootDir := t.TempDir()
	alphaDir := writeNamedFactoryFixture(t, rootDir, "alpha")
	betaDir := writeNamedFactoryFixture(t, rootDir, "beta")
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		RuntimeMode:       interfaces.RuntimeModeService,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.cfg.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("service runtime mode = %q, want %q", svc.cfg.RuntimeMode, interfaces.RuntimeModeService)
	}
	if svc.cfg.Dir != alphaDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, alphaDir)
	}

	createReplacementWatchChannel(t, betaDir, "task", "activated")
	replacement, err := svc.buildReplacementFactoryRuntime(context.Background(), betaDir)
	if err != nil {
		t.Fatalf("buildReplacementFactoryRuntime: %v", err)
	}
	if replacement.dir != betaDir {
		t.Fatalf("replacement dir = %q, want %q", replacement.dir, betaDir)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- replacement.factory.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("replacement runtime returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("replacement runtime after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replacement runtime to stop")
	}
}

func createReplacementWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeNamedFactoryFixture(t *testing.T, rootDir, name string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id": name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":           "execute-" + name,
				"worker":         "executor",
				"inputs":         []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":        []map[string]string{{"workType": "task", "state": "complete"}},
				"type":           "MODEL_WORKSTATION",
				"promptTemplate": "Implement {{ .WorkID }}.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(named factory fixture): %v", err)
	}

	factoryDir, err := config.PersistNamedFactory(rootDir, name, payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}
