package metrics

import factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

type Fields = factory.Fields
type MetricsEmitter = factory.MetricsEmitter
type NoopEmitter = factory.NoopEmitter

func EnsureEmitter(emitter MetricsEmitter) MetricsEmitter {
	return factory.EnsureEmitter(emitter)
}
