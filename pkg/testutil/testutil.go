// Package testutil provides test helpers and harnesses.
package testutil

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// seedFileCounter provides unique filenames across concurrent test invocations.
var seedFileCounter atomic.Int64

// CopyFixtureDir copies the entire directory tree at srcDir into a new
// temporary directory and returns the path to the copy. The temporary
// directory is automatically removed when t finishes (via t.Cleanup).
//
// Each call produces an independent copy, so parallel subtests can safely
// use the same source fixture without interfering with each other.
func CopyFixtureDir(t *testing.T, srcDir string) string {
	t.Helper()

	dst := t.TempDir() // cleaned up automatically by testing.T

	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("CopyFixtureDir: failed to copy %s: %v", srcDir, err)
	}

	return dst
}

// WriteSeedFile writes a JSON payload as a seed file in the fixture directory's
// inputs/<workType>/default/ directory. The file watcher picks up these files
// during preseed on startup. Call this BEFORE constructing the harness so the
// files are present when BuildFactoryService runs.
func WriteSeedFile(t *testing.T, dir, workType string, payload []byte) {
	t.Helper()
	inputDir := filepath.Join(dir, interfaces.InputsDir, workType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("WriteSeedFile: create input dir: %v", err)
	}
	filename := fmt.Sprintf("seed-%d.json", seedFileCounter.Add(1))
	if err := os.WriteFile(filepath.Join(inputDir, filename), payload, 0o644); err != nil {
		t.Fatalf("WriteSeedFile: write file: %v", err)
	}
}

// WriteSeedMarkdownFile writes raw content as a .md seed file with the given
// filename (without extension). The file watcher derives the SubmitRequest Name
// from the filename, so this exercises the non-JSON submission path.
func WriteSeedMarkdownFile(t *testing.T, dir, workType, name string, content []byte) {
	t.Helper()
	inputDir := filepath.Join(dir, interfaces.InputsDir, workType, interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("WriteSeedMarkdownFile: create input dir: %v", err)
	}
	filename := fmt.Sprintf("%s.md", name)
	if err := os.WriteFile(filepath.Join(inputDir, filename), content, 0o644); err != nil {
		t.Fatalf("WriteSeedMarkdownFile: write file: %v", err)
	}
}

// WriteSeedRequest marshals a one-item FACTORY_REQUEST_BATCH as a seed file.
// Use this instead of WriteSeedFile when the test needs to preserve TraceID,
// Tags, state placement, or internal execution fields through the file watcher
// pipeline.
func WriteSeedRequest(t *testing.T, dir string, req interfaces.SubmitRequest) {
	t.Helper()
	data, err := json.Marshal(seedWorkRequestFromSubmitRequest(req))
	if err != nil {
		t.Fatalf("WriteSeedRequest: marshal: %v", err)
	}
	WriteSeedFile(t, dir, req.WorkTypeID, data)
}

// WriteSeedBatchFile writes a canonical FACTORY_REQUEST_BATCH watched-file input
// into inputs/BATCH/default so functional tests exercise the public mixed-work-
// type file-watcher boundary instead of direct API or runtime helpers.
func WriteSeedBatchFile(t *testing.T, dir string, request interfaces.WorkRequest) {
	t.Helper()

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("WriteSeedBatchFile: marshal: %v", err)
	}

	inputDir := filepath.Join(dir, interfaces.InputsDir, "BATCH", interfaces.DefaultChannelName)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("WriteSeedBatchFile: create input dir: %v", err)
	}

	filename := request.RequestID
	if filename == "" {
		filename = fmt.Sprintf("batch-%d", seedFileCounter.Add(1))
	}
	if err := os.WriteFile(filepath.Join(inputDir, filename+".json"), data, 0o644); err != nil {
		t.Fatalf("WriteSeedBatchFile: write file: %v", err)
	}
}

type seedWorkRequest struct {
	RequestID string     `json:"requestId,omitempty"`
	Type      string     `json:"type"`
	Works     []seedWork `json:"works"`
}

type seedWork struct {
	Name             string                `json:"name"`
	WorkID           string                `json:"workId,omitempty"`
	WorkTypeID       string                `json:"workTypeName"`
	State            string                `json:"state,omitempty"`
	TraceID          string                `json:"traceId,omitempty"`
	Payload          any                   `json:"payload,omitempty"`
	Tags             map[string]string     `json:"tags,omitempty"`
	ExecutionID      string                `json:"execution_id,omitempty"`
	RuntimeRelations []interfaces.Relation `json:"runtime_relations,omitempty"`
}

func seedWorkRequestFromSubmitRequest(req interfaces.SubmitRequest) seedWorkRequest {
	return seedWorkRequest{
		RequestID: req.RequestID,
		Type:      string(interfaces.WorkRequestTypeFactoryRequestBatch),
		Works: []seedWork{{
			Name:             seedWorkName(req),
			WorkID:           req.WorkID,
			WorkTypeID:       req.WorkTypeID,
			State:            req.TargetState,
			TraceID:          req.TraceID,
			Payload:          seedPayload(req.Payload),
			Tags:             req.Tags,
			ExecutionID:      req.ExecutionID,
			RuntimeRelations: req.Relations,
		}},
	}
}

func seedWorkName(req interfaces.SubmitRequest) string {
	if req.Name != "" {
		return req.Name
	}
	if req.Tags != nil && req.Tags["_work_name"] != "" {
		return req.Tags["_work_name"]
	}
	if req.WorkID != "" {
		return req.WorkID
	}
	return "seed"
}

func seedPayload(payload []byte) any {
	if len(payload) == 0 {
		return nil
	}
	return string(payload)
}

// ScaffoldFactoryDir writes a FactoryConfig as factory.json to a new temporary
// directory and returns the directory path. The temp directory is cleaned up
// via t.Cleanup. This allows tests to construct configs programmatically while
// still exercising the full service path (config loading → ConfigMapper.Map()).
func ScaffoldFactoryDir(t *testing.T, cfg *interfaces.FactoryConfig) string {
	t.Helper()
	dir := t.TempDir()
	clone := *cfg
	if strings.TrimSpace(clone.Name) == "" {
		if strings.TrimSpace(clone.Project) != "" {
			clone.Name = clone.Project
		} else {
			clone.Name = filepath.Base(dir)
		}
	}
	data, err := factoryconfig.MarshalCanonicalFactoryConfig(&clone)
	if err != nil {
		t.Fatalf("ScaffoldFactoryDir: marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), data, 0o644); err != nil {
		t.Fatalf("ScaffoldFactoryDir: write factory.json: %v", err)
	}
	return dir
}

// UpdateFactoryJSON applies an in-place mutation to a copied fixture's
// factory.json so tests can author focused topology or guard changes without
// bypassing the normal config loader.
func UpdateFactoryJSON(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("UpdateFactoryJSON: read %s: %v", path, err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("UpdateFactoryJSON: unmarshal %s: %v", path, err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("UpdateFactoryJSON: marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("UpdateFactoryJSON: write %s: %v", path, err)
	}
}

// AppendFactoryInferenceThrottleGuard authors a root-level inference throttle
// guard into a copied fixture so tests exercise the supported config path
// instead of retired runtime-only wiring.
func AppendFactoryInferenceThrottleGuard(
	t *testing.T,
	dir string,
	provider workers.ModelProvider,
	model string,
	refreshWindow time.Duration,
) {
	t.Helper()

	UpdateFactoryJSON(t, dir, func(cfg map[string]any) {
		guards, _ := cfg["guards"].([]any)
		guards = append(guards, map[string]any{
			"type":          "INFERENCE_THROTTLE_GUARD",
			"modelProvider": strings.ToUpper(string(provider)),
			"model":         model,
			"refreshWindow": refreshWindow.String(),
		})
		cfg["guards"] = guards
	})
}

// PipelineConfig returns a FactoryConfig for a linear N-stage pipeline:
// task:init → stage1 → stage2 → ... → stageN → complete, with a failed state.
// All transitions use the specified worker name.
func PipelineConfig(stages int, workerName string) *interfaces.FactoryConfig {
	states := []interfaces.StateConfig{
		{Name: "init", Type: interfaces.StateTypeInitial},
	}
	for i := 1; i <= stages; i++ {
		states = append(states, interfaces.StateConfig{
			Name: fmt.Sprintf("stage%d", i),
			Type: interfaces.StateTypeProcessing,
		})
	}
	states = append(states,
		interfaces.StateConfig{Name: "complete", Type: interfaces.StateTypeTerminal},
		interfaces.StateConfig{Name: "failed", Type: interfaces.StateTypeFailed},
	)

	workstations := make([]interfaces.FactoryWorkstationConfig, 0, stages+1)
	prev := "init"
	for i := 1; i <= stages; i++ {
		next := fmt.Sprintf("stage%d", i)
		workstations = append(workstations, interfaces.FactoryWorkstationConfig{
			Name:           fmt.Sprintf("step%d", i),
			WorkerTypeName: workerName,
			Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: prev}},
			Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: next}},
		})
		prev = next
	}
	workstations = append(workstations, interfaces.FactoryWorkstationConfig{
		Name:           "finish",
		WorkerTypeName: workerName,
		Inputs:         []interfaces.IOConfig{{WorkTypeName: "task", StateName: prev}},
		Outputs:        []interfaces.IOConfig{{WorkTypeName: "task", StateName: "complete"}},
	})

	return &interfaces.FactoryConfig{
		WorkTypes:    []interfaces.WorkTypeConfig{{Name: "task", States: states}},
		Workers:      []interfaces.WorkerConfig{{Name: workerName}},
		Workstations: workstations,
	}
}
