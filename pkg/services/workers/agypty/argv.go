package agypty

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ArgvSpec describes typed argv fields for one Agy headless invocation.
// Prompt is always emitted as a separate argv element, never concatenated into
// a shell command string (T1 control in agy-pty-threat-review.md).
type ArgvSpec struct {
	Executable string
	Subcommand []string
	Flags      []string
	Prompt     string
}

// BuildArgv constructs a typed argv slice for exec.Command(executable, args...).
// The returned slice never includes a shell interpreter wrapper.
func BuildArgv(spec ArgvSpec) ([]string, error) {
	executable := strings.TrimSpace(spec.Executable)
	if executable == "" {
		return nil, fmt.Errorf("agypty: executable is required")
	}

	argv := make([]string, 0, 1+len(spec.Subcommand)+len(spec.Flags)+1)
	argv = append(argv, executable)
	argv = append(argv, spec.Subcommand...)
	argv = append(argv, spec.Flags...)
	if strings.TrimSpace(spec.Prompt) != "" {
		argv = append(argv, spec.Prompt)
	}
	return argv, nil
}

// ValidateArgv enforces the T1 argv contract: non-empty typed argv with no shell wrapper.
func ValidateArgv(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("agypty: argv is empty")
	}
	if strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("agypty: executable is required")
	}
	return RejectShellWrapper(argv)
}

// RejectShellWrapper returns an error when argv would invoke a shell interpreter
// with a command-string flag such as sh -c, cmd /C, or powershell -Command.
func RejectShellWrapper(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("agypty: argv is empty")
	}

	program := normalizeProgramName(argv[0])
	if !isShellInterpreter(program) {
		return nil
	}

	for i := 1; i < len(argv); i++ {
		if isShellWrapperFlag(argv[i]) {
			return fmt.Errorf("agypty: shell wrapper %q with %q is forbidden", argv[0], argv[i])
		}
	}
	return nil
}

func normalizeProgramName(program string) string {
	program = strings.TrimSpace(program)
	program = filepath.Base(program)
	program = strings.ToLower(program)
	return strings.TrimSuffix(program, ".exe")
}

func isShellInterpreter(program string) bool {
	switch program {
	case "sh", "bash", "zsh", "dash", "ksh", "cmd", "powershell", "pwsh":
		return true
	default:
		return false
	}
}

func isShellWrapperFlag(flag string) bool {
	switch strings.ToLower(strings.TrimSpace(flag)) {
	case "-c", "/c", "-command":
		return true
	default:
		return false
	}
}

// ResolveWorkspaceDir normalizes and validates a workspace directory under
// factoryRoot (T2 control in agy-pty-threat-review.md). The returned path is
// suitable for cmd.Dir and argv path fields after Story 17 lands execution.
func ResolveWorkspaceDir(factoryRoot, rawPath string) (string, error) {
	factoryRoot = strings.TrimSpace(factoryRoot)
	if factoryRoot == "" {
		return "", fmt.Errorf("agypty: factory root is required")
	}
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("agypty: workspace path is required")
	}

	factoryRoot = filepath.Clean(factoryRoot)
	normalized := filepath.Clean(filepath.FromSlash(rawPath))

	var resolved string
	if filepath.IsAbs(normalized) {
		resolved = normalized
	} else {
		if err := rejectRelativeTraversal(normalized); err != nil {
			return "", err
		}
		resolved = filepath.Clean(filepath.Join(factoryRoot, normalized))
	}

	if !pathContainedIn(factoryRoot, resolved) {
		return "", fmt.Errorf("agypty: workspace path must remain under factory root")
	}
	return resolved, nil
}

func rejectRelativeTraversal(normalized string) error {
	if normalized == ".." {
		return fmt.Errorf("agypty: workspace path must not traverse outside factory root")
	}
	if strings.HasPrefix(normalized, ".."+string(filepath.Separator)) {
		return fmt.Errorf("agypty: workspace path must not traverse outside factory root")
	}
	return nil
}

func pathContainedIn(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}
