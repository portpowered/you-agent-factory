package service

import (
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/localmodels"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/modelhost"
	factoryingest "github.com/portpowered/infinite-you/pkg/factory/ingest"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

// Bundle is the runtime wiring produced by Build and referenced from live handles.
type Bundle struct {
	Dir                  string
	FolderPath           string
	RuntimeInstanceID    string
	StartedAtUTC         time.Time
	EventHistory         *factoryevents.FactoryEventHistory
	Factory              factory.Factory
	Listener             *factoryingest.FileWatcher
	Net                  *state.Net
	RuntimeCfg           *factoryconfig.LoadedFactoryConfig
	ModelResources       *localmodels.ResourceLimiter
	ModelAssets          localmodels.AssetPuller
	LocalModels          *localmodels.Manager
	LocalModelRuntime    localmodels.Runtime
	ModelHost            modelhost.Host
	LeaseExecution       *modelhost.LeaseExecution
	Logger               *zap.Logger
	LogSink              *logging.RuntimeLogSink
	MetricsSink          *logging.RuntimeMetricsSink
	Recording            *replay.Recorder
	RecordPath           string
	dispatchMetricFields sync.Map
	dispatchCompleted    func(string)
}
