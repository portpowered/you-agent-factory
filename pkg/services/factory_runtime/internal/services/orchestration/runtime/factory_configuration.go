package runtime

import "github.com/portpowered/infinite-you/pkg/services/workers"

// SetProgressPublisher supplies the Runtime-owned observation bridge for
// detached attempts. The publisher is replaced with the runtime bundle and is
// never used to recover Factory Session state from Workers.
func (f *factoryImpl) SetProgressPublisher(publisher workers.ProgressPublisher) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.progressPublisher = publisher
}

// SetMockWorkersConfig attaches an immutable, request-scoped testing override
// to this runtime. The process Workers root remains unchanged; each detached
// Execute request receives its own cloned selection.
func (f *factoryImpl) SetMockWorkersConfig(config *workers.MockWorkersConfig) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.mockWorkersConfig = config.Clone()
}

// SetPromptSourceReader supplies the Runtime-owned read-only filesystem edge
// used to refresh authored prompt sources at dispatch time. The reader is an
// effect port; Runtime retains only the function and never a filesystem object.
func (f *factoryImpl) SetPromptSourceReader(reader func(string) ([]byte, error)) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.promptSourceReader = reader
}
