package workerexecution

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

const (
	ProjectTagKey    = workers.ProjectTagKey
	DefaultProjectID = workers.DefaultProjectID
	DefaultSessionID = workers.DefaultSessionID
)

type Context = workers.Context

var ResolveProjectID = workers.ResolveProjectID
