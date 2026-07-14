package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
)

const minimalProductionManifest = `{
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
    "you.docs": {
      "id": "you.docs",
      "name": "docs",
      "path": "you docs",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "docs"},
          "description": {"canonicalEnglish": "docs"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "docs [topic]"},
      "handler": {"id": "you.docs.handler"}
    },
    "you.models": {
      "id": "you.models",
      "name": "models",
      "path": "you models",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "models"},
          "description": {"canonicalEnglish": "models"}
        }
      },
      "visibility": "visible",
      "runnable": false,
      "usage": {"line": "models"}
    },
    "you.models.list": {
      "id": "you.models.list",
      "name": "list",
      "path": "you models list",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "list"},
          "description": {"canonicalEnglish": "list"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "list"},
      "handler": {"id": "you.models.list.handler", "operationId": "listModels"}
    },
    "you.models.inspect": {
      "id": "you.models.inspect",
      "name": "inspect",
      "path": "you models inspect",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "inspect"},
          "description": {"canonicalEnglish": "inspect"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "inspect <model-name>"},
      "handler": {"id": "you.models.inspect.handler", "operationId": "getModel"}
    },
    "you.models.invoke": {
      "id": "you.models.invoke",
      "name": "invoke",
      "path": "you models invoke",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "invoke"},
          "description": {"canonicalEnglish": "invoke"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "invoke <model-name>"},
      "handler": {"id": "you.models.invoke.handler", "operationId": "invokeModel"}
    },
    "you.models.pull": {
      "id": "you.models.pull",
      "name": "pull",
      "path": "you models pull",
      "documentation": {
        "documentation": {
          "title": {"canonicalEnglish": "pull"},
          "description": {"canonicalEnglish": "pull"}
        }
      },
      "visibility": "visible",
      "runnable": true,
      "usage": {"line": "pull <model-name>"},
      "handler": {"id": "you.models.pull.handler", "operationId": "pullModel"}
    }
  }
}`

func writeMinimalProductionManifest(t *testing.T, root string) {
	t.Helper()
	manifestPath := filepath.Join(root, filepath.FromSlash(climanifestgen.ProductionManifestPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create manifest directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(minimalProductionManifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
