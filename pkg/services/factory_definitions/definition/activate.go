package factorydefinition

import (
	"context"
	"fmt"
)

// ActivateNamedFactory builds a replacement runtime from one persisted named
// factory and swaps it in only after the current runtime is idle.
func (s *Service) ActivateNamedFactory(ctx context.Context, name string) error {
	if s == nil || s.host == nil || s.activationGateway == nil {
		return fmt.Errorf("factory definition service is required")
	}
	return s.activationGateway.WithActivationLock(func() error {
		sessionID := s.activationGateway.RunSessionID()
		session := s.activationGateway.SessionForActivation(sessionID)
		persistRoot, folderPath := s.activationGateway.NamedFactoryActivationPaths(session)

		if err := s.activationGateway.RequireIdleBeforeNamedFactoryActivation(ctx, sessionID, session); err != nil {
			return err
		}

		factoryDir, err := resolveExistingFactoryDirFromHost(s.host, persistRoot, name)
		if err != nil {
			return err
		}

		return s.activationGateway.SwapPersistedNamedFactoryRuntime(
			ctx,
			sessionID,
			session,
			persistRoot,
			folderPath,
			factoryDir,
			name,
		)
	})
}
