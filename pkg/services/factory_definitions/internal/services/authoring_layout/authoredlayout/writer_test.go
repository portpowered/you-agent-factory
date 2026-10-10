package authoredlayout

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type recordingInbox struct{ paths []string }

func (r *recordingInbox) EnsureInputInboxGitkeep(_ string, path string) error {
	r.paths = append(r.paths, filepath.ToSlash(path))
	return nil
}

func controlledWriter(inbox *recordingInbox) *Writer {
	return NewWriter(
		func(worker factorydefinitions.FactoryWorkerConfig) ([]byte, error) {
			return []byte("worker:" + worker.Type), nil
		},
		func(station factorydefinitions.FactoryWorkstationConfig) ([]byte, error) {
			return []byte("station:" + station.Type), nil
		},
		func(body string) []byte { return []byte(body) },
		NewAgentsFileWriter(localTestFileSystem{}),
		func(_, name string) (string, error) { return name, nil },
		func(dir, name string) (string, error) { return filepath.Join(dir, name), nil },
		localTestFileSystem{}, inbox,
	)
}

func TestWriterPreparedPrunesRetiredEntitiesAndWritesBodiesAndInboxes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, retired := range []string{"workers/retired", "workstations/retired"} {
		if err := os.MkdirAll(filepath.Join(dir, retired), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	config := &factorydefinitions.FactoryConfig{
		Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "model", Type: factorydefinitions.WorkerTypeModel, Body: "worker body"}, {Name: "placeholder"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "run", Type: factorydefinitions.WorkstationTypeModel, Body: "station body", PromptFile: "prompt.md", PromptTemplate: "prompt text"}, {Name: "logical"}},
		WorkTypes:    []factorydefinitions.WorkTypeConfig{{Name: "task"}, {Name: " "}},
	}
	inbox := &recordingInbox{}
	prepared := &factorydefinitions.PreparedFactoryLayoutPayload{Config: config, Canonical: []byte(`{"name":"prepared"}`), RootFileName: "factory.yaml"}
	if err := controlledWriter(inbox).WritePrepared(dir, prepared, "input.json", nil, nil); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"workers/model/AGENTS.md": "worker body", "workers/placeholder/AGENTS.md": "worker:MODEL_WORKER", "workstations/run/AGENTS.md": "station body", "workstations/run/prompt.md": "prompt text", "workstations/logical/AGENTS.md": "station:LOGICAL_MOVE", "factory.yaml": "name: prepared\n"} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
		}
	}
	for _, retired := range []string{"workers/retired", "workstations/retired"} {
		if _, err := os.Stat(filepath.Join(dir, retired)); !os.IsNotExist(err) {
			t.Fatalf("retired %s: %v", retired, err)
		}
	}
	if strings.Join(inbox.paths, ",") != "inputs/BATCH/default/.gitkeep,inputs/task/default/.gitkeep" {
		t.Fatalf("inboxes = %v", inbox.paths)
	}
}

func TestWriterExpandPreservesSplitDefinitionsAndCopiesReferencedScript(t *testing.T) {
	t.Parallel()
	source, target := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "scripts", "run.py"), []byte("print('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := &factorydefinitions.FactoryConfig{
		Workers:      []factorydefinitions.FactoryWorkerConfig{{Name: "script", Type: factorydefinitions.WorkerTypeScript, Command: "python", Args: []string{"-W", "ignore", "scripts/run.py"}}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{Name: "run", WorkerTypeName: "script", CopyReferencedScripts: true}},
	}
	writer := controlledWriter(&recordingInbox{})
	report, err := writer.Expand(target, source, "input", config, []byte(`{"name":"expanded"}`), nil, nil, nil)
	if err != nil || report.WorkerAgentPaths != 1 || report.WorkstationAgentPaths != 1 {
		t.Fatalf("expand = %+v, %v", report, err)
	}
	got, err := os.ReadFile(filepath.Join(target, "scripts", "run.py"))
	if err != nil || string(got) != "print('ok')" {
		t.Fatalf("script = %q, %v", got, err)
	}
	config.Workers[0].Type = ""
	report, err = writer.Expand(target, source, "input", config, []byte(`{}`), nil, nil, nil)
	if err != nil || report.WorkerAgentPaths != 0 || report.WorkstationAgentPaths != 0 {
		t.Fatalf("existing split definitions = %+v, %v", report, err)
	}
}

func TestWriterPortableFailureStopsPruningAndInboxCreation(t *testing.T) {
	t.Parallel()
	inbox := &recordingInbox{}
	cause := errors.New("materialization failed")
	pruned := false
	err := controlledWriter(inbox).WritePrepared(t.TempDir(), &factorydefinitions.PreparedFactoryLayoutPayload{Config: &factorydefinitions.FactoryConfig{}, Canonical: []byte(`{}`)}, "input", func(string, *factorydefinitions.FactoryConfig) ([]factorydefinitions.PortableBundledFileReplacement, error) {
		return nil, cause
	}, func(string, *factorydefinitions.FactoryConfig) error { pruned = true; return nil })
	if !errors.Is(err, cause) || pruned || len(inbox.paths) != 0 {
		t.Fatalf("failure = %v; pruned=%v inboxes=%v", err, pruned, inbox.paths)
	}
}

func TestWriteAgentsFileCreatesDirectoryAndWritesContent(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "workers", "reviewer")
	content := []byte("---\ntype: INFERENCE_WORKER\n---\n")

	writeAgentsFile := NewAgentsFileWriter(localTestFileSystem{})
	if err := writeAgentsFile(dir, content); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	written, err := os.ReadFile(
		filepath.Join(dir, factorydefinitions.FactoryAgentsFileName),
	)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(written) != string(content) {
		t.Fatalf("content = %q, want %q", written, content)
	}
}

func TestWriterFailsClosedWithoutFileSystemOrInboxEnsurer(t *testing.T) {
	newWriter := func(fileSystem factorydefinitions.AuthoredLayoutWriterFileSystem, ensureInbox factorydefinitions.InputInboxSentinelEnsurer) *Writer {
		return NewWriter(
			func(factorydefinitions.FactoryWorkerConfig) ([]byte, error) { return nil, nil },
			func(factorydefinitions.FactoryWorkstationConfig) ([]byte, error) { return nil, nil },
			func(string) []byte { return nil },
			func(string, []byte) error { return nil },
			func(_, value string) (string, error) { return value, nil },
			func(_, value string) (string, error) { return value, nil },
			fileSystem,
			ensureInbox,
		)
	}
	if err := newWriter(nil, nil).WritePrepared("unused", nil, "test", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "writer filesystem is required") {
		t.Fatalf("WritePrepared() without filesystem error = %v", err)
	}
	fileSystem := localTestFileSystem{}
	if err := newWriter(fileSystem, nil).WritePrepared("unused", nil, "test", nil, nil); err == nil ||
		!strings.Contains(err.Error(), "sentinel ensurer is required") {
		t.Fatalf("WritePrepared() without inbox ensurer error = %v", err)
	}
}
