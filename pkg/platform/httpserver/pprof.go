package httpserver

import (
	"net/http"
	"net/http/pprof"
)

// HandlerWithPprof adds the standard Go pprof routes to one HTTP handler when
// explicitly requested. The private mux keeps this server's registration
// local; the serving path does not use the process-wide DefaultServeMux.
func HandlerWithPprof(handler http.Handler, enabled bool) http.Handler {
	if !enabled || handler == nil {
		return handler
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	for _, profile := range []string{
		"allocs", "block", "goroutine", "heap", "mutex", "threadcreate",
	} {
		mux.Handle("/debug/pprof/"+profile, pprof.Handler(profile))
	}
	mux.Handle("/", handler)
	return mux
}
