package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	acpsdk "github.com/portpowered/infinite-you/third_party/acp-go-sdk"
)

// TestAttemptPromptWithWindow_PanicStillClosesWindowForLaterIdentityReuse
// proves the panic/unexpected-unwind fix: previously Window.End ran as a
// plain statement after connection.Prompt, so a panic during Prompt skipped
// it entirely and left the window's single active slot pointing at a dead
// session forever. promptWithWindow now defers window.End, so an unexpected
// unwind still closes it - and a later attempt that reuses the same
// attempt ID can bind and have a control reach its own fresh session instead
// of racing a stale one that would never unblock TryCancel.
func TestAttemptPromptWithWindow_PanicStillClosesWindowForLaterIdentityReuse(t *testing.T) {
	t.Parallel()

	owned := &attempt{}
	attemptID := "attempt-1"

	func() {
		defer func() { _ = recover() }()
		_, _ = owned.promptWithWindow(attemptID, acpsdk.SessionId("stale-session"), nil, func() (acpsdk.PromptResponse, error) {
			panic("simulated unexpected Prompt unwind")
		})
	}()

	// A control racing in before the next attempt's own Begin must observe
	// no live window for this identity - not hang on the dead stale session.
	if _, ok := owned.window.Claim(attemptID); ok {
		t.Fatal("Claim() on stale identity ok = true, want false: no live window remains for a closed session, including after a panicking prompt")
	}

	// A later attempt reusing the same attempt ID must be able to bind and
	// have controls reach its own fresh session, not the stale one.
	peer := newFakeSessionPeer()
	connection := newPipedConnection(t, peer)
	freshSession := owned.window.Begin(attemptID, acpsdk.SessionId("fresh-session"), connection)
	claimed, ok := owned.window.Claim(attemptID)
	if !ok || claimed != freshSession {
		t.Fatalf("Claim() on reused identity = (%v, %v), want the fresh session and true", claimed, ok)
	}

	type outcome struct {
		accepted bool
		err      error
	}
	tryCancelDone := make(chan outcome, 1)
	go func() {
		accepted, err := claimed.TryCancel(context.Background())
		tryCancelDone <- outcome{accepted: accepted, err: err}
	}()
	<-peer.received
	owned.window.End(freshSession, true)
	result := <-tryCancelDone
	if result.err != nil {
		t.Fatalf("TryCancel() on reused identity error = %v, want nil", result.err)
	}
	if !result.accepted {
		t.Fatal("TryCancel() on reused identity accepted = false, want true: the fresh session must be reachable")
	}
}

// TestAttemptPromptWithWindow_NormalReturnRecordsOutcomeAndClosesWindow proves
// promptWithWindow's normal (non-panic) path is unchanged by the refactor:
// the window closes and the real StopReason still grounds the recorded
// outcome a concurrent TryCancel would observe.
func TestAttemptPromptWithWindow_NormalReturnRecordsOutcomeAndClosesWindow(t *testing.T) {
	t.Parallel()

	owned := &attempt{}
	attemptID := "attempt-1"

	response, err := owned.promptWithWindow(attemptID, acpsdk.SessionId("session-1"), nil, func() (acpsdk.PromptResponse, error) {
		return acpsdk.PromptResponse{StopReason: acpsdk.StopReasonEndTurn}, nil
	})
	if err != nil {
		t.Fatalf("promptWithWindow() error = %v, want nil", err)
	}
	if response.StopReason != acpsdk.StopReasonEndTurn {
		t.Fatalf("promptWithWindow() response = %#v, want the prompt func's own response", response)
	}
	if _, ok := owned.window.Claim(attemptID); ok {
		t.Fatal("Claim() ok = true after a normal return, want the window closed")
	}
}

const attemptConcurrencyHelperEnvironment = "YOU_TEST_ACP_ATTEMPT_CONCURRENCY_HELPER"

// canceledCloseHelperEnvironment names the OS-process peer of the pre-publication
// Close regression below. It is separate from the concurrency peer's own
// environment so neither peer can answer the other's protocol.
const canceledCloseHelperEnvironment = "YOU_TEST_ACP_CANCELED_CLOSE_HELPER"

// attemptRendezvousDeadline bounds every wait in this file, so a service that
// serializes same-provider work fails its regression instead of hanging the
// suite. It is the only clock in these tests: nothing decides an outcome by
// waiting for a duration and inferring what happened.
const attemptRendezvousDeadline = 45 * time.Second

// attemptRendezvous is the only channel the spawned ACP peers and these tests
// use to talk to each other, and it gates every peer explicitly.
//
// A peer reports its arrival at its own session/prompt turn and then blocks: it
// receives no decision at all until the test releases that exact label. So an
// arrival the test has observed means that peer is genuinely parked inside its
// own turn, and "every expected peer has arrived" is a precondition this test
// enforces on the service rather than a timing observation. A service that
// serialized same-provider work behind one shared connection could not report
// the second arrival at all, so it fails by deadline instead of passing by
// luck.
type attemptRendezvous struct {
	listener net.Listener
	address  string

	mu sync.Mutex
	// gates is keyed by peer label and holds the withheld decision for that
	// peer. Every field below is read and written under mu, except arrived,
	// which is written under mu and consumed only by the test goroutine.
	gates   map[string]*attemptGate
	arrived chan string
}

type attemptGate struct {
	decision string
	open     chan struct{}
	released bool
}

func newAttemptRendezvous(t *testing.T, decisionOf func(string) string) *attemptRendezvous {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for attempt rendezvous: %v", err)
	}
	rendezvous := &attemptRendezvous{
		listener: listener,
		address:  listener.Addr().String(),
		gates:    map[string]*attemptGate{},
		arrived:  make(chan string, 16),
	}
	for _, label := range knownAttemptLabels {
		rendezvous.gates[label] = &attemptGate{decision: decisionOf(label), open: make(chan struct{})}
	}
	t.Cleanup(func() { _ = listener.Close() })
	go rendezvous.serve()
	return rendezvous
}

// knownAttemptLabels are the peers these tests launch. Every one has a gate
// from the start, so a release never races the peer's own registration.
var knownAttemptLabels = []string{
	"attempt-a", "attempt-b", "attempt-c",
	"cancelled-attempt", "surviving-attempt",
}

func (r *attemptRendezvous) serve() {
	for {
		connection, err := r.listener.Accept()
		if err != nil {
			return
		}
		go r.handle(connection)
	}
}

// handle reports this peer's arrival and withholds its decision until the test
// releases that label. Reporting strictly before the decision is withheld is
// what makes an observed arrival mean "parked inside its own prompt turn".
func (r *attemptRendezvous) handle(connection net.Conn) {
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(attemptRendezvousDeadline)); err != nil {
		return
	}
	scanner := bufio.NewScanner(connection)
	if !scanner.Scan() {
		return
	}
	label := strings.TrimSpace(scanner.Text())
	r.mu.Lock()
	gate := r.gates[label]
	r.mu.Unlock()
	if gate == nil {
		return
	}
	r.arrived <- label
	select {
	case <-gate.open:
	case <-time.After(attemptRendezvousDeadline):
		return
	}
	if _, err := fmt.Fprintf(connection, "%s\n", gate.decision); err != nil {
		return
	}
	// Drain until this peer closes its end so the decision is always delivered
	// in full before the server can drop the connection.
	scanner.Scan()
}

// awaitArrivals waits for exactly count distinct peers to report that they are
// parked in their own prompt turn, and returns their labels. It consumes only
// the arrival channel, so it never reads a shared slice a server goroutine is
// still appending to.
func (r *attemptRendezvous) awaitArrivals(t *testing.T, count int) []string {
	t.Helper()
	deadline := time.After(attemptRendezvousDeadline)
	arrived := make([]string, 0, count)
	for len(arrived) < count {
		select {
		case label := <-r.arrived:
			if slices.Contains(arrived, label) {
				t.Fatalf("peer %q reported its arrival twice: %v", label, arrived)
			}
			arrived = append(arrived, label)
		case <-deadline:
			t.Fatalf("only %d of %d same-provider attempts entered their turn; arrived=%v", len(arrived), count, arrived)
		}
	}
	return arrived
}

// release lets exactly this peer leave its turn by handing it the decision this
// test assigned to it. Releasing a peer that has not arrived, or twice, fails
// the test rather than silently proving nothing.
func (r *attemptRendezvous) release(t *testing.T, label string) {
	t.Helper()
	r.mu.Lock()
	gate := r.gates[label]
	released := gate != nil && gate.released
	if gate != nil {
		gate.released = true
	}
	r.mu.Unlock()
	switch {
	case gate == nil:
		t.Fatalf("release(%q): this test never launched that peer", label)
	case released:
		t.Fatalf("release(%q) called twice: a released peer proves nothing", label)
	}
	close(gate.open)
}

func newAttemptConcurrencyService(t *testing.T) (acp.ContinuationService, *sync.Mutex, *int) {
	t.Helper()
	var mu sync.Mutex
	starts := 0
	factory := platformprocess.CommandFactory(func(_ string, _ ...string) *exec.Cmd {
		mu.Lock()
		starts++
		mu.Unlock()
		command := exec.Command(os.Args[0], "-test.run=^TestACPAttemptConcurrencyHelperProcess$")
		command.Env = append(os.Environ(), attemptConcurrencyHelperEnvironment+"=1")
		return command
	})
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "shared-acp", Transport: "stdio", Command: "shared-agent acp",
	}}, factory, availableLocator{}, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = serviceValue.Close(context.Background()) })
	return serviceValue, &mu, &starts
}

func attemptConcurrencyEnvironment(label, session, rendezvous, workingDirectory string) []string {
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case attemptConcurrencyHelperEnvironment, "ACP_ATTEMPT_LABEL", "ACP_ATTEMPT_SESSION", "ACP_ATTEMPT_RENDEZVOUS", "ACP_ATTEMPT_CWD":
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		attemptConcurrencyHelperEnvironment+"=1",
		"ACP_ATTEMPT_LABEL="+label,
		"ACP_ATTEMPT_SESSION="+session,
		"ACP_ATTEMPT_RENDEZVOUS="+rendezvous,
		"ACP_ATTEMPT_CWD="+workingDirectory,
	)
}

type attemptOutcome struct {
	label  string
	result providers.ExecuteResult
	err    error
}

// startAttempt runs one independent request against the shared provider in its
// own working root and returns the one channel that carries its outcome. Each
// attempt gets its own channel as well as its own label, session id and
// environment, so no assertion about one attempt can consume or be satisfied by
// another's completion.
func startAttempt(
	serviceValue acp.ContinuationService,
	label string,
	root string,
	rendezvous string,
) <-chan attemptOutcome {
	completed := make(chan attemptOutcome, 1)
	go func() {
		session := "acp-session-" + label
		result, err := serviceValue.Execute(context.Background(), "shared-acp", providers.ExecuteRequest{
			Provider:           "shared-acp",
			AttemptID:          label,
			UserMessage:        "serve " + label,
			WorkingDirectory:   root,
			ProcessEnvironment: attemptConcurrencyEnvironment(label, session, rendezvous, root),
		})
		completed <- attemptOutcome{label: label, result: result, err: err}
	}()
	return completed
}

// awaitAttempt returns the completed attempt on one attempt's own channel.
func awaitAttempt(t *testing.T, completed <-chan attemptOutcome, label string) attemptOutcome {
	t.Helper()
	select {
	case outcome := <-completed:
		return outcome
	case <-time.After(attemptRendezvousDeadline):
		t.Fatalf("attempt %q did not return within the rendezvous deadline", label)
		return attemptOutcome{}
	}
}

// stillBlocked asserts that an attempt this test has not released has not
// returned. It is sound because a peer's decision - and therefore any way for
// its attempt to finish - arrives only after release, so this reads the absence
// of a completion rather than inferring one from elapsed time.
func stillBlocked(t *testing.T, completed <-chan attemptOutcome, label string) {
	t.Helper()
	select {
	case outcome := <-completed:
		t.Fatalf("Execute(%s) returned %q/%v while it was still parked in its own turn", label, outcome.result.Content, outcome.err)
	default:
	}
}

// assertOwnTurn verifies an attempt returned only the result its own peer
// produced: its own answer text and its own Provider Session.
func assertOwnTurn(t *testing.T, outcome attemptOutcome) {
	t.Helper()
	if outcome.err != nil {
		t.Fatalf("Execute(%s) error = %v, want nil", outcome.label, outcome.err)
	}
	if outcome.result.Content != "answer "+outcome.label {
		t.Fatalf("Execute(%s) content = %q, want only its own turn's notification", outcome.label, outcome.result.Content)
	}
	if outcome.result.SessionRef == nil || outcome.result.SessionRef.ID != "acp-session-"+outcome.label {
		t.Fatalf("Execute(%s) SessionRef = %#v, want its own session acp-session-%s", outcome.label, outcome.result.SessionRef, outcome.label)
	}
}

// assertCanceledTurn verifies the one attempt a control named ended by
// honoring its own session/cancel, reported as the established canceled failure.
func assertCanceledTurn(t *testing.T, outcome attemptOutcome) {
	t.Helper()
	var failure providers.ExecuteFailure
	if !errors.As(outcome.err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("cancelled Execute(%s) error = %#v, want ExecuteFailureKindCanceled", outcome.label, outcome.err)
	}
}

// TestSameProviderAttemptsEnterTheirTurnBeforeEitherIsReleased is the
// deterministic regression for same-provider serialization: two independent
// working roots against one canonical provider must both reach session/prompt,
// and both must be parked there, before either is allowed to finish.
//
// Neither peer receives a decision until this test releases it, so releasing the
// first attempt early is not merely unlikely to fail - the second attempt is
// still parked and the release order cannot be observed to overlap.
func TestSameProviderAttemptsEnterTheirTurnBeforeEitherIsReleased(t *testing.T) {
	labels := []string{"attempt-a", "attempt-b"}
	rendezvous := newAttemptRendezvous(t, func(string) string { return "complete" })
	serviceValue, _, _ := newAttemptConcurrencyService(t)

	completed := map[string]<-chan attemptOutcome{}
	for _, label := range labels {
		completed[label] = startAttempt(serviceValue, label, t.TempDir(), rendezvous.address)
	}

	if arrived := rendezvous.awaitArrivals(t, len(labels)); len(arrived) != len(labels) {
		t.Fatalf("attempts that entered their turn = %v, want exactly %d distinct same-provider attempts", arrived, len(labels))
	}
	for _, label := range labels {
		stillBlocked(t, completed[label], label)
	}

	for _, label := range labels {
		rendezvous.release(t, label)
		assertOwnTurn(t, awaitAttempt(t, completed[label], label))
	}
}

// TestCancellingOneSameProviderAttemptLeavesTheOtherIntact proves cancellation
// is scoped to the exact attempt that owns the cancelled turn. Both attempts are
// parked in their own turns when the control runs, and the survivor is released
// only after the cancellation has been observed, so the cancellation can only be
// observed by the attempt it names; the bystander stays parked until it is
// released afterwards and still returns its own result.
func TestCancellingOneSameProviderAttemptLeavesTheOtherIntact(t *testing.T) {
	const cancelledLabel, survivingLabel = "cancelled-attempt", "surviving-attempt"
	// The cancelled attempt's peer waits for its own session/cancel
	// notification; the bystander's peer is only completed after the
	// cancellation has been observed.
	rendezvous := newAttemptRendezvous(t, func(label string) string {
		if label == cancelledLabel {
			return "wait-for-cancel"
		}
		return "complete"
	})
	serviceValue, _, _ := newAttemptConcurrencyService(t)

	cancelledDone := startAttempt(serviceValue, cancelledLabel, t.TempDir(), rendezvous.address)
	survivingDone := startAttempt(serviceValue, survivingLabel, t.TempDir(), rendezvous.address)
	rendezvous.awaitArrivals(t, 2)
	// Only the cancelled peer is released here, into the wait that reads its own
	// session/cancel notification. The survivor stays unreleased, so it is still
	// parked in its own turn when the control is delivered.
	rendezvous.release(t, cancelledLabel)

	generation, ok := serviceValue.Claim("shared-acp", cancelledLabel)
	if !ok {
		t.Fatal("Claim() of the live cancelled attempt ok = false, want true")
	}
	accepted, err := serviceValue.TryCancel(context.Background(), generation)
	if err != nil {
		t.Fatalf("TryCancel() error = %v, want nil", err)
	}
	if !accepted {
		t.Fatal("TryCancel() accepted = false, want true: the cancelled attempt's own turn honored the notification")
	}
	// The bystander was never released, so it cannot have finished.
	stillBlocked(t, survivingDone, survivingLabel)

	rendezvous.release(t, survivingLabel)
	// The cancellation is observed through the cancelled attempt's own peer: its
	// turn ended only because it answered the session/cancel notification this
	// service delivered to the generation the control claimed.
	assertCanceledTurn(t, awaitAttempt(t, cancelledDone, cancelledLabel))
	assertOwnTurn(t, awaitAttempt(t, survivingDone, survivingLabel))
}

// TestSameProviderAttemptsOwnTheirOwnProcessAndWorkingRoot proves each request
// owns its own process for its whole attempt: two overlapping same-provider
// attempts start two processes, each peer verifies the working root of the
// attempt that launched it, and a later attempt starts a brand new process
// rather than inheriting either predecessor.
func TestSameProviderAttemptsOwnTheirOwnProcessAndWorkingRoot(t *testing.T) {
	labels := []string{"attempt-a", "attempt-b"}
	rendezvous := newAttemptRendezvous(t, func(string) string { return "complete" })
	serviceValue, mu, starts := newAttemptConcurrencyService(t)

	completed := map[string]<-chan attemptOutcome{}
	for _, label := range labels {
		completed[label] = startAttempt(serviceValue, label, t.TempDir(), rendezvous.address)
	}
	rendezvous.awaitArrivals(t, len(labels))
	for _, label := range labels {
		rendezvous.release(t, label)
		assertOwnTurn(t, awaitAttempt(t, completed[label], label))
	}

	if observed := currentStarts(mu, starts); observed != len(labels) {
		t.Fatalf("ACP processes started = %d, want one per request (%d)", observed, len(labels))
	}

	const laterLabel = "attempt-c"
	completed[laterLabel] = startAttempt(serviceValue, laterLabel, t.TempDir(), rendezvous.address)
	rendezvous.awaitArrivals(t, 1)
	rendezvous.release(t, laterLabel)
	assertOwnTurn(t, awaitAttempt(t, completed[laterLabel], laterLabel))
	if observed := currentStarts(mu, starts); observed != len(labels)+1 {
		t.Fatalf("ACP processes started after a third attempt = %d, want %d: no process may be reused across requests", observed, len(labels)+1)
	}
}

func currentStarts(mu *sync.Mutex, starts *int) int {
	mu.Lock()
	defer mu.Unlock()
	return *starts
}

// TestACPAttemptConcurrencyHelperProcess is the OS-process ACP peer for the
// same-provider overlap regressions above. It is not a Factory/process-boundary
// cell.
func TestACPAttemptConcurrencyHelperProcess(t *testing.T) {
	if os.Getenv(attemptConcurrencyHelperEnvironment) == "" {
		return
	}
	if err := runAttemptConcurrencyPeer(os.Stdin, os.Stdout); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(2)
	}
	os.Exit(0)
}

// runAttemptConcurrencyPeer answers initialize, session/new and session/load
// normally, then parks the session/prompt turn on the rendezvous named by
// ACP_ATTEMPT_RENDEZVOUS until the test releases this peer.
func runAttemptConcurrencyPeer(stdin io.Reader, stdout io.Writer) error {
	label, session := os.Getenv("ACP_ATTEMPT_LABEL"), os.Getenv("ACP_ATTEMPT_SESSION")
	if err := peerLaunchSetup(); err != nil {
		return err
	}
	lines := attemptRequestLines(stdin)
	writer := bufio.NewWriter(stdout)
	for {
		line, open := <-lines
		if !open {
			return fmt.Errorf("peer %q's request stream ended before its prompt turn", label)
		}
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal([]byte(line), &request) != nil {
			continue
		}
		switch request.Method {
		case "initialize":
			if err := writeAttemptRPCResult(writer, request.ID, `{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}`); err != nil {
				return err
			}
		case "session/new":
			payload, err := json.Marshal(map[string]any{"sessionId": session, "configOptions": []any{}})
			if err != nil {
				return err
			}
			if err := writeAttemptRPCResult(writer, request.ID, string(payload)); err != nil {
				return err
			}
		case "session/load":
			if err := writeAttemptRPCResult(writer, request.ID, `{"configOptions":[]}`); err != nil {
				return err
			}
		case "session/prompt":
			return serveAttemptPrompt(lines, writer, label, session, os.Getenv("ACP_ATTEMPT_RENDEZVOUS"), request.ID)
		case "session/cancel", "$/cancel_request":
			// A notification for a turn this peer is not serving cannot
			// belong to this peer: each attempt has its own process.
			return fmt.Errorf("peer %q received %s outside its own prompt turn", label, request.Method)
		}
	}
}

// peerLaunchSetup verifies this peer's own working root before it serves any
// protocol, so a peer that did not start in the requesting attempt's working
// directory fails instead of silently passing.
func peerLaunchSetup() error {
	want := os.Getenv("ACP_ATTEMPT_CWD")
	if want == "" {
		return nil
	}
	// Every attempt must have started in its own working root, which is
	// only true if each request launched its own process.
	cwd, err := os.Getwd()
	if err != nil || cwd != want {
		return fmt.Errorf("peer working directory = %q, want %q", cwd, want)
	}
	return nil
}

func attemptRequestLines(stdin io.Reader) <-chan string {
	lines := make(chan string, 16)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	return lines
}

// serveAttemptPrompt ends this peer's own prompt turn according to the decision
// the test released to this peer. A "wait-for-cancel" decision keeps reading
// this peer's own traffic until the session/cancel notification for this very
// turn arrives, so the cancelled outcome is produced by the real notification
// rather than asserted directly.
func serveAttemptPrompt(
	lines <-chan string,
	writer *bufio.Writer,
	label string,
	session string,
	rendezvous string,
	pending json.RawMessage,
) error {
	decision, err := awaitAttemptDecision(rendezvous, label)
	if err != nil {
		return err
	}
	if decision == "wait-for-cancel" {
		for {
			line, open := <-lines
			if !open {
				return fmt.Errorf("peer %q's request stream ended before its session/cancel notification", label)
			}
			var notification struct {
				Method string `json:"method"`
			}
			if json.Unmarshal([]byte(line), &notification) != nil {
				continue
			}
			if notification.Method == "session/cancel" || notification.Method == "$/cancel_request" {
				return writeAttemptRPCResult(writer, pending, `{"stopReason":"cancelled"}`)
			}
		}
	}
	if err := writeAttemptNotification(writer, session, "answer "+label); err != nil {
		return err
	}
	return writeAttemptRPCResult(writer, pending, `{"stopReason":"end_turn"}`)
}

// awaitAttemptDecision reports this peer's arrival at its prompt turn and
// returns the decision the test released to it. It blocks for as long as the
// test holds this peer, which is what makes the arrival a real rendezvous.
func awaitAttemptDecision(address, label string) (string, error) {
	connection, err := net.Dial("tcp", address)
	if err != nil {
		return "", fmt.Errorf("dial attempt rendezvous: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(attemptRendezvousDeadline)); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(connection, "%s\n", label); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(connection)
	if !scanner.Scan() {
		return "", errors.New("attempt rendezvous closed before a decision arrived")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

// writeAttemptRPCResult answers one request with a result payload this peer
// controls, so each attempt's session id and notification text are its own.
func writeAttemptRPCResult(writer *bufio.Writer, id json.RawMessage, result string) error {
	if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":%s,"result":%s}`+"\n", id, result); err != nil {
		return err
	}
	return writer.Flush()
}

func writeAttemptNotification(writer *bufio.Writer, session, text string) error {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": session,
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "%s\n", payload); err != nil {
		return err
	}
	return writer.Flush()
}

// TestCloseDuringPrePublicationStartupLeavesNothingOwned is the deterministic
// controlled-edge regression for a teardown that is handed back mid-startup.
//
// Startup is held open at the command edge - after this attempt claimed its
// launch, before it published any process - and Close is then given an
// already-cancelled context, so the Close that wins the attempt's teardown
// cannot wait for the publication it needs. Releasing startup afterwards makes
// a process exist that only this attempt owns, so the assertions are about the
// attempt's real cleanup completing on its caller's return path: the process it
// published is reaped and both standard-stream ends it published are released.
// Retirement itself is never inspected, because the regression is precisely
// that retirement alone decides nothing about who finishes the cleanup.
func TestCloseDuringPrePublicationStartupLeavesNothingOwned(t *testing.T) {
	blocked, released := make(chan struct{}), make(chan struct{})
	factory := platformprocess.CommandFactory(func(_ string, _ ...string) *exec.Cmd {
		// Block startup before any process exists: this edge runs after the
		// attempt claimed its launch and before cmd.Start publishes handles.
		close(blocked)
		<-released
		return exec.Command(os.Args[0], "-test.run=^TestACPCanceledCloseHelperProcess$")
	})
	serviceValue, err := New([]providers.ACPIntegration{{
		ID: "entry-1", Name: "shared-acp", Transport: "stdio", Command: "shared-agent acp",
	}}, factory, availableLocator{}, platformprocess.NewParentOwnedStdio)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	service := serviceValue.(*Service)

	completed := make(chan error, 1)
	go func() {
		_, err := serviceValue.Execute(context.Background(), "shared-acp", providers.ExecuteRequest{
			Provider: "shared-acp", AttemptID: "canceled-close-attempt",
			UserMessage: "hold startup open", WorkingDirectory: t.TempDir(),
			ProcessEnvironment: append(os.Environ(), canceledCloseHelperEnvironment+"=1"),
		})
		completed <- err
	}()
	select {
	case <-blocked:
	case <-time.After(attemptRendezvousDeadline):
		t.Fatal("startup never reached the controlled command edge, so no pre-publication Close could be attempted")
	}
	owned := soleRegisteredAttempt(t, service, "shared-acp")

	// Close elects this attempt's teardown and gives it back: its own context
	// is already cancelled while startup is still blocked before publication.
	closeCtx, cancelClose := context.WithCancel(context.Background())
	cancelClose()
	closed := make(chan error, 1)
	go func() { closed <- service.Close(closeCtx) }()
	select {
	case closeErr := <-closed:
		if !errors.Is(closeErr, context.Canceled) {
			t.Fatalf("Close(canceled context) error = %v, want context.Canceled: the teardown it could not finish is handed back, not abandoned", closeErr)
		}
	case <-time.After(attemptRendezvousDeadline):
		t.Fatal("Close(canceled context) never returned: it waited on a startup it could not terminate")
	}
	// Startup is still parked, so no owned process exists yet and nothing has
	// been cleaned up: whatever cleans up must do it after Close has returned.
	select {
	case err := <-completed:
		t.Fatalf("Execute() returned %v while its startup was still blocked at the command edge", err)
	default:
	}

	close(released)
	select {
	case err := <-completed:
		var failure providers.ExecuteFailure
		if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindCanceled {
			t.Fatalf("Execute() error = %#v, want ExecuteFailureKindCanceled: Close's retirement cancelled this attempt's own request context", err)
		}
	case <-time.After(attemptRendezvousDeadline):
		t.Fatal("Execute() never returned after its blocked startup was released")
	}

	assertAttemptReleasedOwnedResources(t, owned)
}

// soleRegisteredAttempt returns the one attempt a single Execute registered
// against this provider, captured while that Execute is still parked so the
// assertions below observe the very attempt Close had to hand back.
func soleRegisteredAttempt(t *testing.T, service *Service, id providers.ID) *attempt {
	t.Helper()
	target := service.providers[id]
	if target == nil {
		t.Fatalf("provider %q is not registered", id)
	}
	target.mu.Lock()
	defer target.mu.Unlock()
	if len(target.attempts) != 1 {
		t.Fatalf("registered attempts = %d, want exactly the one this Execute registered", len(target.attempts))
	}
	return target.attempts[0]
}

// assertAttemptReleasedOwnedResources proves this attempt owns no live process
// and no open stream end once its caller's return path has finished: the
// published process has been waited on, and both parent ends of the
// standard-stream channel it published reject further use. It reads only the
// published handles, never the retirement or teardown flags, so a handover
// cannot be mistaken for a cleanup.
//
// Every observation is already ordered behind the cleanup: Execute has returned,
// and teardown completes inside that return path, so nothing here waits on or
// infers from elapsed time.
func assertAttemptReleasedOwnedResources(t *testing.T, owned *attempt) {
	t.Helper()
	handles := owned.process()
	if handles == nil {
		t.Fatal("this attempt published no process, so the pre-publication Close regression proves nothing")
	}
	// Last-resort hygiene for a red run only: the assertions below decide the
	// outcome, and they never terminate this process themselves.
	t.Cleanup(func() {
		if handles.cmd.ProcessState == nil && handles.cmd.Process != nil {
			_ = handles.cmd.Process.Kill()
		}
	})
	if _, writeErr := handles.stdin.Write([]byte("teardown probe\n")); writeErr == nil {
		t.Fatal("this attempt's own standard input was never released: its process and streams are still owned after the caller's return")
	}
	if _, readErr := handles.stdout.Read(make([]byte, 1)); readErr == nil {
		t.Fatal("this attempt's standard output was never released: its response stream is still owned after the caller's return")
	}
	if handles.cmd.ProcessState == nil {
		t.Fatal("this attempt's process was never waited on: it escaped teardown and is still running")
	}
}

// TestACPCanceledCloseHelperProcess is the OS-process peer for the
// pre-publication Close regression above. It answers nothing and exits only when
// its parent releases the standard-stream channel, so a process whose teardown
// was abandoned stays alive exactly as long as that abandoned cleanup would
// leak one. It is not a Factory/process-boundary cell.
func TestACPCanceledCloseHelperProcess(t *testing.T) {
	if os.Getenv(canceledCloseHelperEnvironment) == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
	}
	os.Exit(0)
}
