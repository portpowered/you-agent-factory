// Package sessionfixtures provides small, inert Factory Sessions fixtures for
// tests that need to exercise the public Sessions composition boundary.
package sessionfixtures

import (
	"io/fs"
	"time"

	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
)

// NewService constructs the smallest inert Sessions root accepted by its
// public wire boundary. It is intentionally limited to construction fixtures;
// callers still exercise the published service through its public interfaces.
func NewService() (factorysessions.Service, error) {
	eventsService, err := eventswire.NewService()
	if err != nil {
		return nil, err
	}

	return factorysessionwire.NewService(
		nil,
		sessionResultProjection{},
		nil,
		nil,
		nil,
		func() string { return "functional-response-event" },
		nil,
		func() string { return "functional-session" },
		func() (string, error) { return "", nil },
		directoryInspection{},
		namedPathResolver{},
		factorysessionwire.InvocationInputReader(func(string) ([]byte, error) { return nil, nil }),
		factorysessionwire.InitialWorkReader(func(string) ([]byte, error) { return nil, nil }),
		func(path string) (string, error) { return path, nil },
		eventsService,
		clock{},
		factorysessionwire.NewLiveChangeCoordinator(),
	)
}

type sessionResultProjection struct{}

func (sessionResultProjection) ProjectSessionResults(factoryruntime.SessionResultInput) factoryruntime.SessionResultProjection {
	return factoryruntime.SessionResultProjection{}
}

type directoryInspection struct{}

func (directoryInspection) Stat(string) (fs.FileInfo, error) {
	return nil, fs.ErrNotExist
}

func (directoryInspection) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, nil
}

type namedPathResolver struct{}

func (namedPathResolver) ResolveCandidatePaths(string, string, string) (factorydefinitions.NamedFactoryCandidatePaths, error) {
	return factorydefinitions.NamedFactoryCandidatePaths{}, nil
}

func (namedPathResolver) ResolveExistingDir(string, string) (string, error) { return "", nil }
func (namedPathResolver) RequireDefinitionDir(string) error                 { return nil }
func (namedPathResolver) ResolveCurrentDir(string) (string, error)          { return "", nil }
func (namedPathResolver) ReadCurrentPointer(string) (string, error)         { return "", nil }
func (namedPathResolver) WriteCurrentPointer(string, string) error          { return nil }

type clock struct{}

func (clock) Now() time.Time { return time.Unix(0, 0) }
