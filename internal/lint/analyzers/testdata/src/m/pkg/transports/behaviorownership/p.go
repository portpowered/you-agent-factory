package behaviorownership

import (
	ctx "context"
	f "fmt"
	"net"
	h "net/http"
	o "os"
	"os/exec"
	"os/signal"
	s "sync"
	clock "time"
)

var seam = func() {} // want `transport-mutable-function-seam:.*seam`
var _ h.Client       // want `transport-external-effect:.*net/http.Client`
var _ *o.File        // want `transport-external-effect:.*os.File`
var _ s.Mutex        // want `transport-concurrency:.*sync.Mutex`
func run(parent ctx.Context) {
	_ = ctx.Background()                // want `transport-lifecycle:.*context.Background`
	_, cancel := ctx.WithCancel(parent) // want `transport-lifecycle:.*context.WithCancel`
	cancel()
	clock.Sleep(0)               // want `transport-lifecycle:.*time.Sleep`
	_ = clock.Now()              // want `transport-external-effect:.*time.Now`
	_ = o.Getenv("x")            // want `transport-external-effect:.*os.Getenv`
	_, _ = o.ReadFile("x")       // want `transport-external-effect:.*os.ReadFile`
	f.Print("x")                 // want `transport-external-effect:.*fmt.Print`
	_ = exec.Command("x")        // want `transport-external-effect:.*os/exec.Command`
	signal.Reset()               // want `transport-lifecycle:.*os/signal.Reset`
	_, _ = net.Listen("tcp", "") // want `transport-external-effect:.*net.Listen`
	_ = h.Server{}               // want `transport-lifecycle:.*net/http.Server`
	_ = make(chan int)           // want `transport-concurrency:.*make\(chan\)`
	go func() {}()               // want `transport-concurrency:.*go`
}
func BuildWorkflowSessionResult() {} // want `transport-alternate-service-entrypoint:.*BuildWorkflowSessionResult`
