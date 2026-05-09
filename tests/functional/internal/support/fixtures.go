package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func ScaffoldFactory(t *testing.T, cfg map[string]any) string {
	t.Helper()

	dir := t.TempDir()
	if _, ok := cfg["name"]; !ok {
		cfg["name"] = "factory"
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	workstations, ok := cfg["workstations"].([]map[string]any)
	if !ok {
		return dir
	}
	for _, ws := range workstations {
		name, _ := ws["name"].(string)
		if name == "" {
			continue
		}
		WriteWorkstationConfig(t, dir, name, "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n")
	}

	return dir
}

func ScaffoldFactoryFromExamplePNG(t *testing.T, relPath string) string {
	t.Helper()

	pngBytes, err := os.ReadFile(AgentFactoryPath(t, relPath))
	if err != nil {
		t.Fatalf("read example PNG %s: %v", relPath, err)
	}

	metadataText, err := factoryMetadataTextFromPNG(pngBytes)
	if err != nil {
		t.Fatalf("read factory metadata from %s: %v", relPath, err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(metadataText), &cfg); err != nil {
		t.Fatalf("unmarshal factory metadata from %s: %v", relPath, err)
	}
	delete(cfg, "schemaVersion")

	return ScaffoldFactory(t, cfg)
}

func AgentFactoryPath(t *testing.T, rel string) string {
	t.Helper()
	return testutil.MustRepoPath(t, rel)
}

func LegacyFixtureDir(t *testing.T, name string) string {
	t.Helper()
	return testutil.MustRepoPath(t, filepath.Join("tests", "functional_test", "testdata", name))
}

func ClearSeedInputs(t *testing.T, dir string) {
	t.Helper()

	if err := os.RemoveAll(filepath.Join(dir, interfaces.InputsDir)); err != nil {
		t.Fatalf("clear seed inputs: %v", err)
	}
}

func WriteAgentConfig(t *testing.T, dir, workerName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workers", workerName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create worker config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func WriteWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func AssertArgsContainSequence(t *testing.T, args, want []string) {
	t.Helper()

	for i := 0; i <= len(args)-len(want); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return
		}
	}

	t.Fatalf("expected args %v to contain sequence %v", args, want)
}

func WriteWorkRequestFile(t *testing.T, path string, request interfaces.SubmitRequest) {
	t.Helper()

	workName := request.Name
	if workName == "" {
		workName = request.WorkID
	}
	if workName == "" {
		workName = "work-1"
	}
	data, err := json.Marshal(interfaces.WorkRequest{
		RequestID: request.RequestID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       workName,
			WorkID:     request.WorkID,
			RequestID:  request.RequestID,
			WorkTypeID: request.WorkTypeID,
			State:      request.TargetState,
			TraceID:    request.TraceID,
			Payload:    append([]byte(nil), request.Payload...),
			Tags:       request.Tags,
		}},
	})
	if err != nil {
		t.Fatalf("marshal work request file: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write work request file: %v", err)
	}
}

func UpdateFactoryConfig(t *testing.T, dir string, mutate func(map[string]any)) {
	t.Helper()

	path := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	mutate(cfg)

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
}

var pngSignature = []byte{137, 80, 78, 71, 13, 10, 26, 10}

func factoryMetadataTextFromPNG(pngBytes []byte) (string, error) {
	if len(pngBytes) < len(pngSignature) || !bytes.Equal(pngBytes[:len(pngSignature)], pngSignature) {
		return "", fmt.Errorf("invalid PNG signature")
	}

	for offset := len(pngSignature); offset < len(pngBytes); {
		chunkType, chunkData, nextOffset, err := readPNGChunkAtOffset(pngBytes, offset)
		if err != nil {
			return "", err
		}
		offset = nextOffset

		switch chunkType {
		case "tEXt":
			keyword, text, err := readPNGTextChunk(chunkData)
			if err != nil {
				return "", err
			}
			if keyword == "portos.agent-factory" {
				return text, nil
			}
		case "iTXt":
			keyword, text, err := readPNGInternationalTextChunk(chunkData)
			if err != nil {
				return "", err
			}
			if keyword == "portos.agent-factory" {
				return text, nil
			}
		}
	}

	return "", fmt.Errorf("portos.agent-factory metadata missing")
}

func readPNGChunkAtOffset(pngBytes []byte, offset int) (string, []byte, int, error) {
	if offset+8 > len(pngBytes) {
		return "", nil, 0, fmt.Errorf("truncated PNG chunk header at %d", offset)
	}

	length := int(uint32(pngBytes[offset])<<24 |
		uint32(pngBytes[offset+1])<<16 |
		uint32(pngBytes[offset+2])<<8 |
		uint32(pngBytes[offset+3]))
	dataOffset := offset + 8
	dataEnd := dataOffset + length
	chunkEnd := dataEnd + 4
	if length < 0 || chunkEnd > len(pngBytes) {
		return "", nil, 0, fmt.Errorf("truncated PNG chunk %q at %d", string(pngBytes[offset+4:offset+8]), offset)
	}

	return string(pngBytes[offset+4 : offset+8]), pngBytes[dataOffset:dataEnd], chunkEnd, nil
}

func readPNGTextChunk(data []byte) (string, string, error) {
	separator := bytes.IndexByte(data, 0)
	if separator <= 0 {
		return "", "", fmt.Errorf("invalid PNG tEXt chunk")
	}
	return string(data[:separator]), string(data[separator+1:]), nil
}

func readPNGInternationalTextChunk(data []byte) (string, string, error) {
	separator := bytes.IndexByte(data, 0)
	if separator <= 0 {
		return "", "", fmt.Errorf("invalid PNG iTXt keyword")
	}

	keyword := string(data[:separator])
	cursor := separator + 1
	if cursor+2 > len(data) {
		return "", "", fmt.Errorf("truncated PNG iTXt metadata")
	}
	if data[cursor] != 0 || data[cursor+1] != 0 {
		return "", "", fmt.Errorf("compressed PNG iTXt metadata unsupported")
	}
	cursor += 2

	languageEnd := bytes.IndexByte(data[cursor:], 0)
	if languageEnd < 0 {
		return "", "", fmt.Errorf("invalid PNG iTXt language tag")
	}
	cursor += languageEnd + 1

	translatedEnd := bytes.IndexByte(data[cursor:], 0)
	if translatedEnd < 0 {
		return "", "", fmt.Errorf("invalid PNG iTXt translated keyword")
	}
	cursor += translatedEnd + 1

	return keyword, string(data[cursor:]), nil
}
