package shell_completion_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	shellFactoryName = "shell-fixture"
	shellModeValue   = "json"
	shellFileName    = "shell-config.json"
)

// TestGeneratedCompletionScriptsReachBuiltExecutable proves generated completion works against the shipped command tree.
func TestGeneratedCompletionScriptsReachBuiltExecutable(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	binaryDir := t.TempDir()
	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(binaryDir, binaryName)
	runCommand(t, repositoryRoot, os.Environ(), "go", "build", "-o", binaryPath, "./cmd/factory")

	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	writeShellCompletionFactory(t, workingDirectory)
	if err := os.WriteFile(
		filepath.Join(workingDirectory, shellFileName),
		[]byte("{}"),
		0o600,
	); err != nil {
		t.Fatalf("write shell completion file: %v", err)
	}
	environment := cleanShellEnvironment(binaryDir, homeDirectory)

	t.Run("bash", func(t *testing.T) {
		testBashCompletion(t, binaryPath, workingDirectory, environment)
	})
	t.Run("zsh", func(t *testing.T) {
		testZshCompletion(t, binaryPath, workingDirectory, environment)
	})
	t.Run("powershell", func(t *testing.T) {
		testPowerShellCompletion(t, binaryPath, workingDirectory, environment)
	})
}

func testBashCompletion(
	t *testing.T,
	binaryPath string,
	workingDirectory string,
	environment []string,
) {
	t.Helper()
	shellPath, err := exec.LookPath("bash")
	if err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal("bash is required on non-Windows completion CI")
		}
		t.Skip("bash is not installed on this platform")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the Windows completion lane is owned by PowerShell")
	}
	scriptPath := generateCompletionScript(
		t, binaryPath, workingDirectory, environment, "bash",
	)
	script := fmt.Sprintf(`
for bash_completion in \
    /usr/share/bash-completion/bash_completion \
    /etc/bash_completion \
    /opt/homebrew/etc/profile.d/bash_completion.sh \
    /usr/local/etc/profile.d/bash_completion.sh; do
    if [[ -r "$bash_completion" ]]; then
        source "$bash_completion"
        break
    fi
done
if ! declare -F _get_comp_words_by_ref >/dev/null; then
    printf 'bash-completion runtime is unavailable\n' >&2
    exit 1
fi
source %s
function you { %s "$@"; }

COMP_LINE='you run --named shell-fi'
COMP_POINT=${#COMP_LINE}
COMP_WORDS=(you run --named shell-fi)
COMP_CWORD=3
__start_you
printf 'factory=%%s\n' "${COMPREPLY[@]}"

COMP_LINE='you run --named shell-fixture --mode j'
COMP_POINT=${#COMP_LINE}
COMP_WORDS=(you run --named shell-fixture --mode j)
COMP_CWORD=5
__start_you
printf 'mode=%%s\n' "${COMPREPLY[@]}"
`, quotePOSIX(scriptPath), quotePOSIX(binaryPath))
	output := runCommand(
		t, workingDirectory, environment, shellPath, "--noprofile", "--norc", "-c", script,
	)
	assertShellCandidates(t, output)
}

func testZshCompletion(
	t *testing.T,
	binaryPath string,
	workingDirectory string,
	environment []string,
) {
	t.Helper()
	shellPath, err := exec.LookPath("zsh")
	if err != nil {
		if runtime.GOOS == "darwin" {
			t.Fatal("zsh is required on macOS completion CI")
		}
		t.Skip("zsh is not installed on this platform")
	}
	scriptPath := generateCompletionScript(
		t, binaryPath, workingDirectory, environment, "zsh",
	)
	script := fmt.Sprintf(`
autoload -Uz compinit
compinit -D
source %s
function you { %s "$@"; }
function _describe { print -rl -- "${completions[@]}"; return 0; }

words=(you run --named shell-fi)
CURRENT=4
print -r -- factory-start
_you
print -r -- factory-end

words=(you run --named shell-fixture --mode j)
CURRENT=6
print -r -- mode-start
_you
print -r -- mode-end
`, quotePOSIX(scriptPath), quotePOSIX(binaryPath))
	output := runCommand(t, workingDirectory, environment, shellPath, "-f", "-c", script)
	assertDelimitedCandidate(t, output, "factory", shellFactoryName)
	assertDelimitedCandidate(t, output, "mode", shellModeValue)
}

func testPowerShellCompletion(
	t *testing.T,
	binaryPath string,
	workingDirectory string,
	environment []string,
) {
	t.Helper()
	shellPath, err := exec.LookPath("pwsh")
	if err != nil {
		if runtime.GOOS == "windows" {
			t.Fatal("pwsh is required on Windows completion CI")
		}
		t.Skip("pwsh is not installed on this platform")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the non-Windows completion lane is owned by Bash and Zsh")
	}
	scriptPath := generateCompletionScript(
		t, binaryPath, workingDirectory, environment, "powershell",
	)
	script := fmt.Sprintf(`
. %s
$script:GeneratedYouCompleter = ${__youCompleterBlock}
function global:you { & %s @args }
function Complete-You([string] $Line) {
    $tokens = $null
    $parseErrors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseInput(
        $Line,
        [ref] $tokens,
        [ref] $parseErrors
    )
    if ($parseErrors.Count -ne 0) {
        throw "completion request did not parse"
    }
    $commandAst = $ast.EndBlock.Statements[0].PipelineElements[0]
    $word = ($tokens | Where-Object Kind -ne EndOfInput | Select-Object -Last 1).Text
    & $script:GeneratedYouCompleter $word $commandAst $Line.Length |
        ForEach-Object CompletionText
}

'factory-start'
Complete-You 'you run --named shell-fi'
'factory-end'
'mode-start'
Complete-You 'you run --named shell-fixture --mode j'
'mode-end'
'file-start'
Complete-You 'you run --named shell-fixture --config shell-conf'
'file-end'
'inline-file-start'
Complete-You 'you run --named shell-fixture --config=shell-conf'
'inline-file-end'
`, quotePowerShell(scriptPath), quotePowerShell(binaryPath))
	output := runCommand(
		t,
		workingDirectory,
		environment,
		shellPath,
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		script,
	)
	assertDelimitedCandidate(t, output, "factory", shellFactoryName)
	assertDelimitedCandidate(t, output, "mode", shellModeValue)
	assertDelimitedCandidate(t, output, "file", shellFileName)
	assertDelimitedCandidate(
		t,
		output,
		"inline-file",
		"--config="+shellFileName,
	)
}

func generateCompletionScript(
	t *testing.T,
	binaryPath string,
	workingDirectory string,
	environment []string,
	shell string,
) string {
	t.Helper()
	output := runCommand(
		t, workingDirectory, environment, binaryPath, "completion", shell,
	)
	extension := shell
	if shell == "powershell" {
		extension = "ps1"
	}
	scriptPath := filepath.Join(t.TempDir(), "completion."+extension)
	if err := os.WriteFile(scriptPath, []byte(output), 0o600); err != nil {
		t.Fatalf("write %s completion script: %v", shell, err)
	}
	return scriptPath
}

func writeShellCompletionFactory(t *testing.T, workingDirectory string) {
	t.Helper()
	definition := map[string]any{
		"id":           shellFactoryName,
		"name":         shellFactoryName,
		"workTypes":    []any{},
		"resources":    []any{},
		"workers":      []any{},
		"workstations": []any{},
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{
				{
					"name":         "mode",
					"externalName": "mode",
					"description":  "output mode",
					"choices":      []string{shellModeValue, "text"},
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
				{
					"name":         "config",
					"externalName": "config",
					"description":  "configuration file",
					"typeHint":     "FILE_PATH",
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
			},
		},
	}
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("marshal shell completion Factory: %v", err)
	}
	factoryDirectory := filepath.Join(workingDirectory, "factory", shellFactoryName)
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create shell completion Factory directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDirectory, "factory.json"),
		payload,
		0o600,
	); err != nil {
		t.Fatalf("write shell completion Factory: %v", err)
	}
}

func runCommand(
	t *testing.T,
	workingDirectory string,
	environment []string,
	name string,
	arguments ...string,
) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, name, arguments...)
	command.Dir = workingDirectory
	command.Env = environment
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%s timed out: %v", name, ctx.Err())
	}
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, arguments, err, output)
	}
	return string(output)
}

func cleanShellEnvironment(binaryDirectory string, homeDirectory string) []string {
	path := binaryDirectory
	if inheritedPath := os.Getenv("PATH"); inheritedPath != "" {
		path += string(os.PathListSeparator) + inheritedPath
	}
	environment := []string{
		"HOME=" + homeDirectory,
		"USERPROFILE=" + homeDirectory,
		"PATH=" + path,
		"TEMP=" + homeDirectory,
		"TMP=" + homeDirectory,
		"XDG_CACHE_HOME=" + filepath.Join(homeDirectory, "cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(homeDirectory, "config"),
		"POWERSHELL_TELEMETRY_OPTOUT=1",
		"HTTP_PROXY=",
		"HTTPS_PROXY=",
		"ALL_PROXY=",
		"NO_PROXY=*",
	}
	if runtime.GOOS == "windows" {
		environment = append(environment,
			"PATHEXT=.COM;.EXE;.BAT;.CMD",
			"PSModulePath="+os.Getenv("PSModulePath"),
			"SystemRoot="+os.Getenv("SystemRoot"),
			"WINDIR="+os.Getenv("WINDIR"),
		)
	}
	return environment
}

func assertShellCandidates(t *testing.T, output string) {
	t.Helper()
	if !strings.Contains(output, "factory="+shellFactoryName) {
		t.Fatalf("Factory completion did not reach the built executable:\n%s", output)
	}
	if !strings.Contains(output, "mode="+shellModeValue) {
		t.Fatalf("signature completion did not reach the built executable:\n%s", output)
	}
}

func assertDelimitedCandidate(t *testing.T, output string, section string, candidate string) {
	t.Helper()
	start := section + "-start"
	end := section + "-end"
	after, found := strings.CutPrefix(output, start)
	if !found {
		index := strings.Index(output, start)
		if index < 0 {
			t.Fatalf("%s completion section is absent:\n%s", section, output)
		}
		after = output[index+len(start):]
	}
	body, found := strings.CutSuffix(after, end)
	if !found {
		index := strings.Index(after, end)
		if index < 0 {
			t.Fatalf("%s completion section is unterminated:\n%s", section, output)
		}
		body = after[:index]
	}
	if !strings.Contains(body, candidate) {
		t.Fatalf(
			"%s completion lacks %q:\nsection:\n%s\nfull output:\n%s",
			section,
			candidate,
			body,
			output,
		)
	}
}

func findRepositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("repository root containing go.mod was not found")
		}
		directory = parent
	}
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
