package runtimeopening

import (
	"fmt"
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"go.uber.org/zap"
)

// RuntimeRoot contains the normalized process root and base logger selected
// while opening a Factory Session.
type RuntimeRoot struct {
	FactoryRootDir    string
	BaseLogger        *zap.Logger
	RuntimeInstanceID string
}

func ResolveRuntimeRoot(
	dir string,
	logger *zap.Logger,
	runtimeInstanceID string,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
) (RuntimeRoot, error) {
	factoryRootDir, err := factorysessions.AbsolutizeFactoryDirectory(dir, resolveHome)
	if err != nil {
		return RuntimeRoot{}, err
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if runtimeInstanceID == "" {
		if generateRuntimeInstanceID == nil {
			return RuntimeRoot{}, fmt.Errorf("Factory Session runtime instance ID generator is required")
		}
		runtimeInstanceID = strings.TrimSpace(generateRuntimeInstanceID())
		if runtimeInstanceID == "" {
			return RuntimeRoot{}, fmt.Errorf("Factory Session runtime instance ID generator returned an empty identity")
		}
	}
	return RuntimeRoot{
		FactoryRootDir:    factoryRootDir,
		BaseLogger:        logger,
		RuntimeInstanceID: runtimeInstanceID,
	}, nil
}
