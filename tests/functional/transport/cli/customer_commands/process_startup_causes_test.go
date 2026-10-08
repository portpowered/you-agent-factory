package customer_commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testCLIStartupLoadCauseWithoutDebug(t *testing.T) {
	t.Parallel()
	process := plainProcess(t)
	for _, flag := range []string{"--resume", "--replay"} {
		for _, debug := range []bool{false, true} {
			name := flag
			if debug {
				name += "-debug"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				home := newCLIHome(t)
				path := filepath.Join(home.work, "broken.json")
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
				args := []string{"run", flag, path, "--quiet"}
				if debug {
					args = append(args, "--debug")
				}
				result := home.run(t, process, nil, args...)
				if result.Err == nil {
					t.Fatalf("malformed recording succeeded: stdout=%q stderr=%q", result.Stdout, result.Stderr)
				}
				if strings.TrimSpace(result.Stdout) != "" {
					t.Fatalf("load failure polluted stdout: %q", result.Stdout)
				}
				if !strings.Contains(result.Stderr, "unexpected end of JSON input") || strings.Count(result.Stderr, "cause[0]=") != 1 {
					t.Fatalf("load failure needs its cause exactly once (debug=%t): %q", debug, result.Stderr)
				}
				if strings.Contains(result.Stderr, home.home) || strings.Contains(result.Stderr, "debug: cause[") {
					t.Fatalf("load failure leaked a private path or duplicated debug causes: %q", result.Stderr)
				}
			})
		}
	}
}
