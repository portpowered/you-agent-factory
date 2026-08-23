package httpserver

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	runtimepprof "runtime/pprof"
	runtimetrace "runtime/trace"
	"sort"
	"strconv"
	"strings"
	"time"
)

const pprofPath = "/debug/pprof/"

// HandlerWithPprof adds the standard Go pprof routes to one HTTP handler when
// explicitly requested. The private mux keeps this server's registration
// local; the serving path does not use the process-wide DefaultServeMux.
//
// The net/http/pprof package cannot be imported here because its package
// initialization registers routes on http.DefaultServeMux. These adapters use
// the same standard runtime profile and trace implementations while keeping
// every HTTP route scoped to this server's mux.
func HandlerWithPprof(handler http.Handler, enabled bool) http.Handler {
	if !enabled || handler == nil {
		return handler
	}

	mux := http.NewServeMux()
	mux.HandleFunc(pprofPath, pprofIndex)
	mux.HandleFunc(pprofPath+"cmdline", pprofCmdline)
	mux.HandleFunc(pprofPath+"profile", pprofCPU)
	mux.HandleFunc(pprofPath+"symbol", pprofSymbol)
	mux.HandleFunc(pprofPath+"trace", pprofTrace)
	for _, profile := range []string{
		"allocs", "block", "goroutine", "heap", "mutex", "threadcreate",
	} {
		mux.Handle(pprofPath+profile, pprofNamed(profile))
	}
	mux.Handle("/", handler)
	return mux
}

func pprofIndex(writer http.ResponseWriter, request *http.Request) {
	if name, found := strings.CutPrefix(request.URL.Path, pprofPath); found && name != "" {
		pprofNamed(name).ServeHTTP(writer, request)
		return
	}

	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")

	profiles := make([]pprofEntry, 0, len(runtimepprof.Profiles())+4)
	for _, profile := range runtimepprof.Profiles() {
		name := profile.Name()
		profiles = append(profiles, pprofEntry{
			Name:  name,
			Href:  name,
			Desc:  pprofDescriptions[name],
			Count: profile.Count(),
		})
	}
	for _, name := range []string{"cmdline", "profile", "symbol", "trace"} {
		profiles = append(profiles, pprofEntry{
			Name: name,
			Href: name,
			Desc: pprofDescriptions[name],
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })

	if err := writePprofIndex(writer, profiles); err != nil {
		return
	}
}

type pprofEntry struct {
	Name  string
	Href  string
	Desc  string
	Count int
}

var pprofDescriptions = map[string]string{
	"allocs":       "A sampling of all past memory allocations",
	"block":        "Stack traces that led to blocking on synchronization primitives",
	"cmdline":      "The command line invocation of the current program",
	"goroutine":    "Stack traces of all current goroutines",
	"heap":         "A sampling of memory allocations of live objects",
	"mutex":        "Stack traces of holders of contended mutexes",
	"profile":      "CPU profile",
	"symbol":       "Maps program counters to function names",
	"threadcreate": "Stack traces that led to the creation of new OS threads",
	"trace":        "A trace of execution of the current program",
}

func writePprofIndex(writer io.Writer, profiles []pprofEntry) error {
	var body bytes.Buffer
	body.WriteString(`<html>
<head>
<title>/debug/pprof/</title>
<style>
.profile-name{
	display:inline-block;
	width:6rem;
}
</style>
</head>
<body>
/debug/pprof/
<br>
<p>Set debug=1 as a query parameter to export in legacy text format</p>
<br>
Types of profiles available:
<table>
<thead><td>Count</td><td>Profile</td></thead>
`)

	for _, profile := range profiles {
		link := &url.URL{Path: profile.Href, RawQuery: "debug=1"}
		if _, err := fmt.Fprintf(
			&body,
			"<tr><td>%d</td><td><a href='%s'>%s</a></td></tr>\n",
			profile.Count,
			link,
			html.EscapeString(profile.Name),
		); err != nil {
			return err
		}
	}

	body.WriteString(`</table>
<a href="goroutine?debug=2">full goroutine stack dump</a>
<br>
<p>
Profile Descriptions:
<ul>
`)
	for _, profile := range profiles {
		if _, err := fmt.Fprintf(
			&body,
			"<li><div class=profile-name>%s: </div> %s</li>\n",
			html.EscapeString(profile.Name),
			html.EscapeString(profile.Desc),
		); err != nil {
			return err
		}
	}
	body.WriteString(`</ul>
</p>
</body>
</html>`)
	_, err := writer.Write(body.Bytes())
	return err
}

func pprofNamed(name string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		profile := runtimepprof.Lookup(name)
		if profile == nil {
			pprofError(writer, http.StatusNotFound, "Unknown profile")
			return
		}

		if name == "heap" {
			gc, _ := strconv.Atoi(request.FormValue("gc"))
			if gc > 0 {
				runtime.GC()
			}
		}
		debug, _ := strconv.Atoi(request.FormValue("debug"))
		if debug != 0 {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
		} else {
			writer.Header().Set("Content-Type", "application/octet-stream")
			writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
		}
		_ = profile.WriteTo(writer, debug)
	})
}

func pprofCmdline(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(writer, strings.Join(os.Args, "\x00"))
}

func pprofCPU(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	seconds, err := strconv.ParseInt(request.FormValue("seconds"), 10, 64)
	if seconds <= 0 || err != nil {
		seconds = 30
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", `attachment; filename="profile"`)
	if err := runtimepprof.StartCPUProfile(writer); err != nil {
		pprofError(writer, http.StatusInternalServerError, fmt.Sprintf("Could not enable CPU profiling: %s", err))
		return
	}
	defer runtimepprof.StopCPUProfile()
	waitPprofDuration(request, time.Duration(seconds)*time.Second)
}

func pprofTrace(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	seconds, err := strconv.ParseFloat(request.FormValue("seconds"), 64)
	if seconds <= 0 || err != nil {
		seconds = 1
	}
	writer.Header().Set("Content-Type", "application/octet-stream")
	writer.Header().Set("Content-Disposition", `attachment; filename="trace"`)
	if err := runtimetrace.Start(writer); err != nil {
		pprofError(writer, http.StatusInternalServerError, fmt.Sprintf("Could not enable tracing: %s", err))
		return
	}
	defer runtimetrace.Stop()
	waitPprofDuration(request, time.Duration(seconds*float64(time.Second)))
}

func waitPprofDuration(request *http.Request, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-request.Context().Done():
	}
}

func pprofSymbol(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")

	var body bytes.Buffer
	_, _ = fmt.Fprint(&body, "num_symbols: 1\n")
	var reader *bufio.Reader
	if request.Method == http.MethodPost {
		reader = bufio.NewReader(request.Body)
	} else {
		reader = bufio.NewReader(strings.NewReader(request.URL.RawQuery))
	}
	for {
		word, err := reader.ReadSlice('+')
		if err == nil {
			word = word[:len(word)-1]
		}
		pc, _ := strconv.ParseUint(string(word), 0, 64)
		if pc != 0 {
			function := runtime.FuncForPC(uintptr(pc))
			if function != nil {
				_, _ = fmt.Fprintf(&body, "%#x %s\n", pc, function.Name())
			}
		}
		if err != nil {
			if err != io.EOF {
				_, _ = fmt.Fprintf(&body, "reading request: %v\n", err)
			}
			break
		}
	}
	_, _ = writer.Write(body.Bytes())
}

func pprofError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("X-Go-Pprof", "1")
	writer.Header().Del("Content-Disposition")
	writer.WriteHeader(status)
	_, _ = fmt.Fprintln(writer, message)
}
