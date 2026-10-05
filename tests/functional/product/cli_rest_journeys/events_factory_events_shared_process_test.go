package cli_rest_journeys_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var factoryEventsCLIProcess support.ApplicationProcess

func initializeEventsfactoryeventsFixture(t *testing.T) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{APIServerStarter: recordedWebhookEffects.serve, FactoryWebhookSecretResolver: recordedWebhookEffects.resolve, FactoryWebhookClock: recordedWebhookClock, FactoryWebhookDeadLetterAppender: recordedWebhookEffects.append, ProviderCommandRunner: recordedWebhookFailureRunner{}})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build Factory Events CLI process: %v\n", err)
		t.Fatal("customer fixture setup failed; see preceding diagnostic")
	}
	factoryEventsCLIProcess = process
	t.Cleanup(func() {
		exitCode := 0
		//nolint:testsleep // This deadline bounds process teardown after all scenario-owned commands have joined.
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := process.Close(closeContext); err != nil {
			fmt.Fprintf(os.Stderr, "close Factory Events CLI process: %v\n", err)
			exitCode = 1
		}
		if exitCode != 0 {
			t.Error("customer fixture cleanup failed; see preceding diagnostic")
		}
	})
}

type recordedWebhookEffectRouter struct {
	sync.Mutex
	nextPort  int
	servers   map[int]*support.ProcessAPIServer
	resolvers map[string]*functionalWebhookSecretResolver
	appenders map[string]webhooks.DeadLetterAppender
}

var recordedWebhookEffects = recordedWebhookEffectRouter{nextPort: 24000, servers: make(map[int]*support.ProcessAPIServer), resolvers: make(map[string]*functionalWebhookSecretResolver), appenders: make(map[string]webhooks.DeadLetterAppender)}

func (router *recordedWebhookEffectRouter) serve(ctx context.Context, request platformhttpserver.StartRequest) error {
	router.Lock()
	server := router.servers[request.Port]
	router.Unlock()
	if server == nil {
		return errors.New("unregistered recorded webhook listener")
	}
	return server.Start(ctx, request)
}
func (router *recordedWebhookEffectRouter) resolve(ctx context.Context, source factorydefinitions.LoadedFactorySource, ref string) (string, error) {
	router.Lock()
	resolver := router.resolvers[filepath.Clean(source.FactoryDir())]
	router.Unlock()
	if resolver == nil {
		return "", errors.New("unregistered recorded webhook source")
	}
	return resolver.resolve(ctx, source, ref)
}

var recordedWebhookClock = clockwork.NewFakeClockAt(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))

func (router *recordedWebhookEffectRouter) append(path string, line []byte) error {
	router.Lock()
	appender := router.appenders[filepath.Clean(path)]
	router.Unlock()
	if appender != nil {
		return appender(path, line)
	}
	return platformfilesystem.Local{}.AppendDurable(path, line)
}

type recordedWebhookFailureRunner struct{}

func (recordedWebhookFailureRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{ExitCode: 7, Stderr: []byte("controlled webhook provider failure")}, nil
}
func reseteventsfactoryevents5State() {
	var freshFactoryEventsCLIProcess support.ApplicationProcess
	factoryEventsCLIProcess = freshFactoryEventsCLIProcess
	var freshRecordedWebhookEffects = recordedWebhookEffectRouter{nextPort: 24000, servers: make(map[int]*support.ProcessAPIServer), resolvers: make(map[string]*functionalWebhookSecretResolver), appenders: make(map[string]webhooks.DeadLetterAppender)}
	recordedWebhookEffects = freshRecordedWebhookEffects
	var freshRecordedWebhookClock = clockwork.NewFakeClockAt(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))
	recordedWebhookClock = freshRecordedWebhookClock
}
