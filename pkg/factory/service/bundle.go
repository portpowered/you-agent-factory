package service

import (
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	factoryingest "github.com/portpowered/infinite-you/pkg/factory/ingest"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/factory/replay"
	"go.uber.org/zap"
)

// Bundle is the runtime wiring produced by Build and referenced from live handles.
type Bundle struct {
	Dir                  string
	FolderPath           string
	RuntimeInstanceID    string
	BackendScopeID       string
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
	MetricsSink          *platformmetrics.RuntimeMetricsSink
	Recording            *replay.Recorder
	RecordPath           string
	dispatchMetricFields sync.Map
	dispatchCompleted    func(string)
}
