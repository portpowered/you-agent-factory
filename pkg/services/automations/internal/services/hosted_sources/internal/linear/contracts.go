package linear

import "github.com/portpowered/infinite-you/pkg/services/workers"

// HTTPDoer performs the Linear adapter's external network request.
type HTTPDoer = workers.HostedPollerHTTPDoer

// SecretResolver resolves the external credential used by the Linear adapter.
type SecretResolver = workers.HostedPollerSecretResolver
