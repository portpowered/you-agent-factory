package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

func TestRunGeneratesRepresentativeFamilyArtifacts(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{
  "formatVersion": "1.0.0",
  "rootPath": "you",
  "commands": {
    "you": {
      "id": "you",
      "name": "you",
      "path": "you",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "root"},
          "description": {"canonicalEnglish": "root"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "you"}
    },
    "you.session": {
      "id": "you.session",
      "name": "session",
      "path": "you session",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "session"},
          "description": {"canonicalEnglish": "session"}
        }
      },
      "visibility": "visible",
      "runnable": false,
      "usage": {"line": "session"}
    },
    "you.session.show": {
      "id": "you.session.show",
      "name": "show",
      "path": "you session show",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "show"},
          "description": {"canonicalEnglish": "show"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "show [session-id]"},
      "handler": {"id": "you.session.show.handler", "operationId": "getFactorySession"}
    },
    "you.work": {
      "id": "you.work",
      "name": "work",
      "path": "you work",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "work"},
          "description": {"canonicalEnglish": "work"}
        }
      },
      "visibility": "visible",
      "runnable": false,
      "usage": {"line": "work"}
    },
    "you.work.list": {
      "id": "you.work.list",
      "name": "list",
      "path": "you work list",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "list"},
          "description": {"canonicalEnglish": "list"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "list"},
      "handler": {"id": "you.work.list.handler", "operationId": "listWorkBySessionId"}
    },
    "you.work.show": {
      "id": "you.work.show",
      "name": "show",
      "path": "you work show",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "show"},
          "description": {"canonicalEnglish": "show"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "show [work-id]"},
      "handler": {"id": "you.work.show.handler", "operationId": "getWorkBySessionId"}
    },
    "you.work.move": {
      "id": "you.work.move",
      "name": "move",
      "path": "you work move",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "move"},
          "description": {"canonicalEnglish": "move"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "move [work-id] [state-name]"},
      "handler": {"id": "you.work.move.handler", "operationId": "moveWorkBySessionId"}
    },
    "you.work.visualize": {
      "id": "you.work.visualize",
      "name": "visualize",
      "path": "you work visualize",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "visualize"},
          "description": {"canonicalEnglish": "visualize"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "visualize [batch-path]"},
      "handler": {"id": "you.work.visualize.handler"}
    }
  }
}`)
	manifestPath := filepath.Join(root, filepath.FromSlash(climanifestgen.ProductionManifestPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, false, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("CLI metadata generated")) {
		t.Fatalf("stdout = %q, want success message", got)
	}

	drift, err := climanifestgen.Check(root)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("drift after generation = %#v", drift)
	}
}

func TestRunCheckFailsOnStaleArtifact(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, filepath.FromSlash(climanifestgen.RepresentativeFamilyJSONPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(root, true, stdout, stderr); status == 0 {
		t.Fatalf("run(check) = 0, want failure; stderr = %q", stderr.String())
	}
}
