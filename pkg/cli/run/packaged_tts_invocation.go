package run

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
	"go.uber.org/zap"
)

func isPackagedTTSRun(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.NamedFactoryName) == tts.PackagedFactoryName
}

func logPackagedTTSInvocationStart(cfg RunConfig) {
	if !isPackagedTTSRun(cfg) {
		return
	}
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	fields := []zap.Field{
		zap.String("packaged_factory_name", tts.PackagedFactoryName),
		zap.String("tts_backend", tts.BackendRuntimeLabel()),
		zap.String("readiness_outcome", tts.FailureClassLoading),
	}
	if resolution := cfg.NamedFactoryResolution; resolution != nil {
		fields = append(fields,
			zap.String("named_factory_resolution_source", string(resolution.Source)),
			zap.String("named_factory_dir", resolution.FactoryDir),
		)
	}
	logger.Info("packaged tts invocation started", fields...)
}
