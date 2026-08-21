package testutil

import (
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func (m *MockFactory) sessionFactory(sessionID string) (*MockFactory, error) {
	if m == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	if sessionID == "" || sessionID == "~default" {
		if m.SessionFactories == nil {
			return m, nil
		}
		if session, ok := m.SessionFactories["~default"]; ok && session != nil {
			return session, nil
		}
		return m, nil
	}
	if m.SessionFactories == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	session, ok := m.SessionFactories[sessionID]
	if !ok || session == nil {
		return nil, apisurface.ErrFactorySessionNotFound
	}
	return session, nil
}
