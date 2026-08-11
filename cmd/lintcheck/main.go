package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	defaultCacheDir      = ".cache/lint-checkers"
	execStatusFileEnvVar = "LINT_CHECKER_EXEC_STATUS_FILE"
)

type config struct {
	cacheDir   string
	goTool     string
	packageRef string
	fallback   bool
	args       []string
}

func main() {
	if statusFile := os.Getenv(execStatusFileEnvVar); statusFile != "" && isExecWrapperInvocation(os.Args[1:]) {
		os.Exit(runExecWrapper(os.Args[1:], statusFile))
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func isExecWrapperInvocation(args []string) bool {
	return len(args) > 0 && !strings.HasPrefix(args[0], "-")
}

func runExecWrapper(args []string, statusFile string) int {
	command := exec.Command(args[0], args[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = withoutEnvironment(os.Environ(), execStatusFileEnvVar)
	err := command.Run()
	code := processExitCode(err)
	if err == nil {
		code = 0
	}
	if writeErr := os.WriteFile(statusFile, []byte(strconv.Itoa(code)), 0o600); writeErr != nil {
		return 1
	}
	return code
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	if err := execute(cfg, stdout, stderr); err != nil {
		if !isProcessExit(err) {
			fmt.Fprintln(stderr, err)
		}
		return processExitCode(err)
	}
	return 0
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("lintcheck", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cacheDir := flags.String("cache-dir", defaultCacheDir, "directory for compiled checker executables")
	goTool := flags.String("go", "go", "Go tool used to list, build, or run the checker")
	packageRef := flags.String("package", "", "checker package to execute")
	fallback := flags.Bool("fallback", false, "run the checker through go run instead of the cache")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if *goTool == "" {
		return config{}, errors.New("go tool must not be empty")
	}
	if *packageRef == "" {
		return config{}, errors.New("checker package must not be empty")
	}
	return config{
		cacheDir:   *cacheDir,
		goTool:     *goTool,
		packageRef: *packageRef,
		fallback:   *fallback,
		args:       flags.Args(),
	}, nil
}

func execute(cfg config, stdout, stderr io.Writer) error {
	if cfg.fallback {
		return runGoRun(cfg, stdout, stderr)
	}

	checkerPath, err := prepareChecker(cfg, stderr)
	if err != nil {
		return err
	}
	return runExecutable(checkerPath, cfg.args, stdout, stderr)
}

func runGoRun(cfg config, stdout, stderr io.Writer) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve lint checker fallback executable: %w", err)
	}
	statusFile, err := os.CreateTemp("", "lint-checker-status-*")
	if err != nil {
		return fmt.Errorf("create lint checker fallback status file: %w", err)
	}
	statusPath := statusFile.Name()
	if err := statusFile.Close(); err != nil {
		_ = os.Remove(statusPath)
		return fmt.Errorf("close lint checker fallback status file: %w", err)
	}
	defer os.Remove(statusPath)

	args := append([]string{"run", "-exec", self, cfg.packageRef}, cfg.args...)
	command := exec.Command(cfg.goTool, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = withEnvironment(os.Environ(), execStatusFileEnvVar, statusPath)
	err = command.Run()
	if err == nil {
		return nil
	}
	if code, ok := readProcessExitCode(statusPath); ok {
		return processExitError{code: code, err: commandFailure{err: err}}
	}
	return commandFailure{err: err}
}

func runExecutable(path string, args []string, stdout, stderr io.Writer) error {
	return runCommand(path, args, stdout, stderr)
}

func runCommand(name string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command(name, args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return commandFailure{err: err}
	}
	return nil
}

type commandFailure struct {
	err error
}

func (e commandFailure) Error() string {
	return e.err.Error()
}

func (e commandFailure) Unwrap() error {
	return e.err
}

type processExitError struct {
	code int
	err  error
}

func (e processExitError) Error() string {
	return e.err.Error()
}

func (e processExitError) Unwrap() error {
	return e.err
}

func (e processExitError) ExitCode() int {
	return e.code
}

type exitCodeCarrier interface {
	ExitCode() int
}

func isProcessExit(err error) bool {
	var exitErr exitCodeCarrier
	return errors.As(err, &exitErr)
}

func processExitCode(err error) int {
	var exitErr exitCodeCarrier
	if !errors.As(err, &exitErr) {
		return 1
	}
	if code := exitErr.ExitCode(); code >= 0 {
		return code
	}
	return 1
}

func readProcessExitCode(path string) (int, bool) {
	value, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(value)))
	return code, err == nil && code >= 0
}

func withEnvironment(environ []string, key, value string) []string {
	return append(withoutEnvironment(environ, key), key+"="+value)
}

func withoutEnvironment(environ []string, key string) []string {
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name != key {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
