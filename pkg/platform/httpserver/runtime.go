package httpserver

import (
	"encoding/json"
	"net/http"
	"runtime"

	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
)

const runtimeDiagnosticsPath = "/debug/runtime"

// RuntimeSnapshot is the point-in-time diagnostic view returned by the local
// runtime endpoint. ProcessCommitBytes is meaningful only when
// ProcessCommitAvailable is true.
type RuntimeSnapshot struct {
	HeapAllocBytes         uint64 `json:"heapAllocBytes"`
	HeapInuseBytes         uint64 `json:"heapInuseBytes"`
	SysBytes               uint64 `json:"sysBytes"`
	NumGC                  uint32 `json:"numGC"`
	Goroutines             int    `json:"goroutines"`
	ProcessCommitBytes     uint64 `json:"processCommitBytes"`
	ProcessCommitAvailable bool   `json:"processCommitAvailable"`
}

// HandlerWithRuntime adds the always-on runtime snapshot route to an already
// composed HTTP handler. The snapshot is collected per request and retains no
// history or sampler state.
func HandlerWithRuntime(
	handler http.Handler,
	commitReader platformprocessmemory.CommitReader,
) http.Handler {
	if handler == nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(runtimeDiagnosticsPath, runtimeSnapshotHandler(commitReader))
	mux.Handle("/", handler)
	return mux
}

// HandlerWithDiagnostics composes the local runtime snapshot and optional
// standard pprof routes around one API handler.
func HandlerWithDiagnostics(
	handler http.Handler,
	pprofEnabled bool,
	commitReader platformprocessmemory.CommitReader,
	commandLineReader CommandLineReader,
) http.Handler {
	return HandlerWithRuntime(HandlerWithPprof(handler, pprofEnabled, commandLineReader), commitReader)
}

func runtimeSnapshotHandler(commitReader platformprocessmemory.CommitReader) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		snapshot := RuntimeSnapshot{
			HeapAllocBytes: stats.HeapAlloc,
			HeapInuseBytes: stats.HeapInuse,
			SysBytes:       stats.Sys,
			NumGC:          stats.NumGC,
			Goroutines:     runtime.NumGoroutine(),
		}
		if commitReader != nil {
			if commitBytes, err := commitReader(); err == nil && commitBytes > 0 {
				snapshot.ProcessCommitBytes = commitBytes
				snapshot.ProcessCommitAvailable = true
			}
		}

		writer.Header().Set("Content-Type", "application/json")
		// All snapshot fields are fixed-width numeric values, so this encode is
		// not expected to fail. Returning on a response-writer failure avoids
		// writing a second response after the transport has rejected the body.
		if err := json.NewEncoder(writer).Encode(snapshot); err != nil {
			return
		}
	}
}
