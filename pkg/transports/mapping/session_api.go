package apisurface

// sessionAPI is a compatibility aggregate composed from independently owned
// runtime, live-session, and Work services. It owns no state or behavior.
type sessionAPI struct {
	RuntimeAPI
	LiveSessionAPI
	WorkAPI
}

var _ SessionAPI = (*sessionAPI)(nil)

// ComposeSessionAPI provides the historical aggregate without routing calls
// through a generic runtime host.
func ComposeSessionAPI(runtime RuntimeAPI, live LiveSessionAPI, work WorkAPI) SessionAPI {
	return &sessionAPI{RuntimeAPI: runtime, LiveSessionAPI: live, WorkAPI: work}
}
