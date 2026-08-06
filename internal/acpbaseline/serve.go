package acpbaseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// clientHandlers implements the client half of ACP.
//
// Without it a capture deadlocks: an agent that asks permission, or asks to
// read a file, blocks forever if nobody answers. Every decision it makes is
// recorded, because how an agent asks is part of the baseline.
type clientHandlers struct {
	// root confines filesystem access to the scenario workspace. Serving a
	// path outside it would let a captured agent read the operator's machine.
	root string

	mu sync.Mutex
	// permissionChoices records which option was auto-selected each time.
	permissionChoices []string
	// unknownMethods records client methods we do not implement.
	unknownMethods []string
}

func newClientHandlers(root string) *clientHandlers {
	return &clientHandlers{root: root}
}

func (h *clientHandlers) handle(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "session/request_permission":
		return h.grantPermission(params)
	case "fs/read_text_file":
		return h.readTextFile(params)
	case "fs/write_text_file":
		return h.writeTextFile(params)
	}
	h.mu.Lock()
	h.unknownMethods = append(h.unknownMethods, method)
	h.mu.Unlock()
	return nil, &rpcError{Code: -32601, Message: "Method not found"}
}

// grantPermission auto-selects the first allow-shaped option so a capture can
// proceed unattended, and records the choice so a reader knows what was
// approved on their behalf.
func (h *clientHandlers) grantPermission(params json.RawMessage) (any, *rpcError) {
	var request struct {
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
			Name     string `json:"name"`
		} `json:"options"`
	}
	if err := json.Unmarshal(params, &request); err != nil || len(request.Options) == 0 {
		return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}, nil
	}
	chosen := request.Options[0]
	for _, option := range request.Options {
		if option.Kind == "allow_once" || option.Kind == "allow_always" {
			chosen = option
			break
		}
	}
	h.mu.Lock()
	h.permissionChoices = append(h.permissionChoices, chosen.OptionID+"/"+chosen.Kind)
	h.mu.Unlock()
	return map[string]any{
		"outcome": map[string]any{"outcome": "selected", "optionId": chosen.OptionID},
	}, nil
}

func (h *clientHandlers) readTextFile(params json.RawMessage) (any, *rpcError) {
	var request struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}
	resolved, err := h.confine(request.Path)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: "path outside the capture workspace"}
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, &rpcError{Code: -32603, Message: "read failed"}
	}
	return map[string]any{"content": string(content)}, nil
}

func (h *clientHandlers) writeTextFile(params json.RawMessage) (any, *rpcError) {
	var request struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(params, &request); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}
	resolved, err := h.confine(request.Path)
	if err != nil {
		return nil, &rpcError{Code: -32602, Message: "path outside the capture workspace"}
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, &rpcError{Code: -32603, Message: "write failed"}
	}
	if err := os.WriteFile(resolved, []byte(request.Content), 0o644); err != nil {
		return nil, &rpcError{Code: -32603, Message: "write failed"}
	}
	return map[string]any{}, nil
}

// confine resolves a requested path inside the workspace root, rejecting any
// escape. Symlinks are resolved first so a link cannot be used to step out.
func (h *clientHandlers) confine(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(h.root, candidate)
	}
	candidate = filepath.Clean(candidate)

	root := h.root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	probe := candidate
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		probe = resolved
	} else if resolved, err := filepath.EvalSymlinks(filepath.Dir(candidate)); err == nil {
		probe = filepath.Join(resolved, filepath.Base(candidate))
	}
	if probe != root && !strings.HasPrefix(probe, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes the workspace")
	}
	return candidate, nil
}

func (h *clientHandlers) snapshot() (permissions, unknown []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.permissionChoices...), append([]string(nil), h.unknownMethods...)
}
