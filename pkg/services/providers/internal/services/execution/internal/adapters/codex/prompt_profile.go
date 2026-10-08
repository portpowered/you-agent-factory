package codex

import (
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
)

type promptPreparation struct {
	files       providerservice.CodexPromptFileSystem
	resolveHome func() (string, error)
}

// prepare preserves stdin and the developer role. A profile has lower
// precedence than project config, so incompatible configurations fail closed
// rather than quietly changing the effective instructions or selected profile.
func (p promptPreparation) prepare(command *providerservice.CommandRequest, request providers.ExecuteRequest) (func(), error) {
	length := platformprocess.ComposedCommandLineLength(command.Command, command.Args)
	if length < platformprocess.WindowsCommandLineLimit {
		return func() {}, nil
	}
	reject := func(reason string) (func(), error) {
		return nil, &platformprocess.CommandStartError{
			Command: command.Command, ArgsCount: len(command.Args), CommandLineLength: length,
			CommandLineLimit: platformprocess.WindowsCommandLineLimit, StdinBytes: len(command.Stdin), Cause: errors.New(reason),
		}
	}
	if p.files == nil || strings.TrimSpace(request.SystemPrompt) == "" || !utf8.ValidString(request.SystemPrompt) {
		return reject("no compatible instruction-file channel is available")
	}
	home, err := p.codexHome(command.Env)
	if err != nil {
		return reject("cannot resolve an absolute Codex home for the instruction profile")
	}
	if !p.compatible(command.Args, command.WorkDir, home) {
		return reject("selected profile or project configuration prevents equivalent instruction-file delivery")
	}
	file, err := p.files.CreateTemp(home, "you-prompt-*.config.toml")
	if err != nil {
		return reject("cannot exclusively create a private Codex instruction profile")
	}
	cleanup := func() {
		if err := p.files.Remove(file.Name()); err != nil {
			// Never turn an already completed inference into another execution attempt.
			logging.EnsureLogger(request.ExecutionLogger).Warn("Codex instruction profile cleanup failed", "event_name", "codex.prompt_profile_cleanup_failed")
		}
	}
	prompt, _ := json.Marshal(request.SystemPrompt)
	content := "developer_instructions=" + string(prompt) + "\n"
	written, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil || written != len(content) || closeErr != nil {
		cleanup()
		return reject("cannot completely write and close the private Codex instruction profile")
	}
	// Replace only the generated developer override, in place. Worker-authored
	// runtime overrides keep their original ordering and higher precedence.
	inline := "developer_instructions=" + string(prompt)
	profile := strings.TrimSuffix(filepath.Base(file.Name()), ".config.toml")
	args, ok := profileCommandArgs(*command, inline, profile)
	if !ok {
		cleanup()
		return reject("fixed arguments or developer override prevent bounded instruction-file delivery")
	}
	command.Args = args
	return cleanup, nil
}

func (p promptPreparation) codexHome(environment []string) (string, error) {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, "CODEX_HOME") {
			if !filepath.IsAbs(value) {
				return "", fs.ErrInvalid
			}
			return value, nil
		}
	}
	if p.resolveHome == nil {
		return "", fs.ErrInvalid
	}
	home, err := p.resolveHome()
	if err != nil || !filepath.IsAbs(home) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(home, ".codex"), nil
}

func (p promptPreparation) compatible(args []string, directory, home string) bool {
	if incompatibleProfileArgs(args) {
		return false
	}
	// A custom root-marker policy can include config above .git. Without a
	// supported project-root resolver, preserve that policy by rejecting spill.
	base, err := p.files.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	var config map[string]any
	if err := toml.Unmarshal(base, &config); err != nil {
		return false
	}
	if _, customized := config["project_root_markers"]; customized {
		return false
	}
	// Without an absolute working root, the project configuration search cannot
	// be reproduced using only the injected effects.
	if !filepath.IsAbs(directory) {
		return false
	}
	directory, err = p.files.EvalSymlinks(directory)
	if err != nil || !filepath.IsAbs(directory) {
		return false
	}
	for {
		for _, name := range []string{filepath.Join(directory, ".codex", "config.toml"), filepath.Join(directory, "config.toml")} {
			_, err := p.files.ReadFile(name)
			if !errors.Is(err, fs.ErrNotExist) {
				return false
			}
		}
		// Codex's default project root marker is .git, including worktree files.
		// User config above that root has lower precedence than our profile.
		_, err := p.files.Stat(filepath.Join(directory, ".git"))
		if err == nil {
			return true
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return false
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return true
		}
		directory = parent
	}
}

func incompatibleProfileArgs(args []string) bool {
	for i := 0; i < len(args); i++ {
		option, value, attached := strings.Cut(args[i], "=")
		switch option {
		case "--ignore-user-config", "--profile", "-p", "--cd", "-C":
			return true
		case "--config", "-c":
			if !attached && i+1 < len(args) {
				i++
				value = args[i]
			}
			if incompatibleRootOverride(value) {
				return true
			}
		default:
			if strings.HasPrefix(option, "-c") && !strings.HasPrefix(option, "--") {
				if incompatibleRootOverride(strings.TrimPrefix(args[i], "-c")) {
					return true
				}
			} else if (strings.HasPrefix(option, "-p") || strings.HasPrefix(option, "-C")) && !strings.HasPrefix(option, "--") {
				return true
			}
		}
	}
	return false
}

func incompatibleRootOverride(value string) bool {
	key, _, ok := strings.Cut(value, "=")
	if !ok {
		return false
	}
	// Decode only the key: arbitrary instruction/option values are never policy.
	var config map[string]any
	if err := toml.Unmarshal([]byte(key+"=[]"), &config); err != nil {
		return true
	}
	_, customized := config["project_root_markers"]
	return customized
}

// profileCommandArgs changes only the generated override pair and remeasures
// the complete line; model, authored options, resume and stdin keep their order.
func profileCommandArgs(command providerservice.CommandRequest, inline, profile string) ([]string, bool) {
	for i := 0; i+1 < len(command.Args); i++ {
		if command.Args[i] == "--config" && command.Args[i+1] == inline {
			args := append([]string(nil), command.Args...)
			args[i], args[i+1] = "--profile", profile
			return args, platformprocess.ComposedCommandLineLength(command.Command, args) < platformprocess.WindowsCommandLineLimit
		}
	}
	return nil, false
}
