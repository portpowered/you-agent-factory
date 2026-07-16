package application

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"go.uber.org/zap"
)

type recordingRunner struct{ calls int }

func (r *recordingRunner) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.calls++
	return workerprocess.CommandResult{}, nil
}

func TestComponentsRetainSelectedCommandEdges(t *testing.T) {
	provider := &recordingRunner{}
	script := &recordingRunner{}
	components, err := New(zap.NewNop(), Edges{ProviderCommandRunner: provider, ScriptCommandRunner: script})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !components.Valid() || components.ProviderCommandRunner != provider || components.ScriptCommandRunner != script {
		t.Fatalf("components = %+v, want retained typed command edges", components)
	}
}

func TestWithCommandRunnersPreservesUnchangedEdges(t *testing.T) {
	provider := &recordingRunner{}
	components, err := New(zap.NewNop(), Edges{ProviderCommandRunner: provider})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	script := components.ScriptCommandRunner

	replacement := workers.CommandRunner(&recordingRunner{})
	configured, err := components.WithCommandRunners(replacement, nil)
	if err != nil {
		t.Fatalf("WithCommandRunners: %v", err)
	}
	if configured.ProviderCommandRunner != replacement || configured.ScriptCommandRunner != script {
		t.Fatal("runtime wrapper replaced an unrelated composition-selected command edge")
	}
	if !configured.ProviderCommandInjected || configured.ScriptCommandInjected {
		t.Fatal("runtime wrapper changed the wrong command-edge selection marker")
	}
}
