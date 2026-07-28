package service_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWorkersCompletedEffectInjectionsHaveNoOwnerFallbacks(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	workersDir := filepath.Clean(filepath.Join(filepath.Dir(filename), ".."))
	checks := map[string][][]byte{
		"construction/service.go":                         {[]byte("time.Now")},
		"execution/recording/model.go":                    {[]byte("time.Now")},
		"provider/recording_provider.go":                  {[]byte("time.Now")},
		"executor/agentrun/executor.go":                   {[]byte("time.Now"), []byte("time.Since")},
		"executor/workstation_executor.go":                {[]byte("time.Now"), []byte("time.Since"), []byte("os.Getwd")},
		"executor/script.go":                              {[]byte("time.Now"), []byte("time.Since"), []byte("os.Environ")},
		"provider/commandenv/environment.go":              {[]byte("os.Environ")},
		"prompting/prompt_docs.go":                        {[]byte("os.Stat"), []byte("os.ReadFile"), []byte("filepath.WalkDir")},
		"worktree/prepare.go":                             {[]byte("time.Since")},
	}

	for relative, forbidden := range checks {
		content, err := os.ReadFile(filepath.Join(workersDir, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		for _, token := range forbidden {
			if bytes.Contains(content, token) {
				t.Errorf("%s contains retired owner fallback %q", relative, token)
			}
		}
	}
}
