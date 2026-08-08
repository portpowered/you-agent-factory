package acpbaseline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// message is a JSON-RPC frame decoded only as far as routing needs.
//
// Params and Result stay raw: the whole point of a raw driver is that a field
// we have never heard of still reaches the transcript and the matrix.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Observation is what one capture learned, independent of the transcript.
type Observation struct {
	// AgentMethodsAccepted are agent methods that returned a result.
	AgentMethodsAccepted map[string]int `json:"agentMethodsAccepted"`
	// AgentMethodsRejected maps a rejected agent method to its error code.
	// A -32601 here is a finding, not a failure.
	AgentMethodsRejected map[string]int `json:"agentMethodsRejected"`
	// ClientMethodsInvoked are methods the agent called back on us.
	ClientMethodsInvoked map[string]int `json:"clientMethodsInvoked"`
	// SessionUpdateVariants counts each session/update discriminator seen.
	SessionUpdateVariants map[string]int `json:"sessionUpdateVariants"`
	// Results retains selected raw responses for structural analysis.
	Results map[string]json.RawMessage `json:"results"`
	// Snapshots records named points a scenario marked.
	Snapshots []string `json:"snapshots"`
}

func newObservation() *Observation {
	return &Observation{
		AgentMethodsAccepted:  map[string]int{},
		AgentMethodsRejected:  map[string]int{},
		ClientMethodsInvoked:  map[string]int{},
		SessionUpdateVariants: map[string]int{},
		Results:               map[string]json.RawMessage{},
	}
}

// driver speaks raw JSON-RPC to one agent over its stdio.
type driver struct {
	out  *bufio.Writer
	in   *bufio.Reader
	mu   sync.Mutex
	next int

	observation *Observation
	handlers    *clientHandlers
	// pending correlates in-flight requests by id.
	pending map[string]chan message
}

func newDriver(stdin io.Writer, stdout io.Reader, handlers *clientHandlers, observation *Observation) *driver {
	return &driver{
		out:         bufio.NewWriter(stdin),
		in:          bufio.NewReader(stdout),
		observation: observation,
		handlers:    handlers,
		pending:     map[string]chan message{},
		next:        1,
	}
}

// readLoop dispatches every inbound frame until the stream ends. It is the
// only reader, so responses and agent-initiated requests cannot race.
func (d *driver) readLoop(done chan<- struct{}) {
	defer close(done)
	for {
		line, err := d.in.ReadBytes('\n')
		if len(line) > 0 {
			d.dispatch(line)
		}
		if err != nil {
			return
		}
	}
}

func (d *driver) dispatch(line []byte) {
	var frame message
	if err := json.Unmarshal(line, &frame); err != nil {
		return
	}
	switch {
	case frame.Method != "" && len(frame.ID) > 0:
		d.answerAgentRequest(frame)
	case frame.Method != "":
		d.recordNotification(frame)
	default:
		d.completeResponse(frame)
	}
}

func (d *driver) recordNotification(frame message) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.observation.ClientMethodsInvoked[frame.Method]++
	if frame.Method != "session/update" {
		return
	}
	var envelope struct {
		Update map[string]json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(frame.Params, &envelope); err != nil {
		return
	}
	d.observation.SessionUpdateVariants[sessionUpdateVariant(envelope.Update)]++
}

// sessionUpdateVariant reads the discriminator, falling back to the sole
// populated key. Agents disagree on whether the discriminator is present, and
// a variant we cannot name is worse than one named structurally.
func sessionUpdateVariant(update map[string]json.RawMessage) string {
	if raw, ok := update["sessionUpdate"]; ok {
		var name string
		if err := json.Unmarshal(raw, &name); err == nil && name != "" {
			return name
		}
	}
	for key := range update {
		if key != "sessionUpdate" && key != "_meta" {
			return key
		}
	}
	return "unknown"
}

func (d *driver) completeResponse(frame message) {
	d.mu.Lock()
	waiter, ok := d.pending[string(frame.ID)]
	if ok {
		delete(d.pending, string(frame.ID))
	}
	d.mu.Unlock()
	if ok {
		waiter <- frame
	}
}

// answerAgentRequest replies to a request the agent made of us. An
// unrecognized method is answered with method-not-found and recorded:
// discovering a client method we have never heard of is a headline result.
func (d *driver) answerAgentRequest(frame message) {
	d.mu.Lock()
	d.observation.ClientMethodsInvoked[frame.Method]++
	d.mu.Unlock()

	result, rpcErr := d.handlers.handle(frame.Method, frame.Params)
	reply := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(frame.ID)}
	if rpcErr != nil {
		reply["error"] = rpcErr
	} else {
		reply["result"] = result
	}
	_ = d.write(reply)
}

func (d *driver) write(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.out.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return d.out.Flush()
}

// call sends one request and waits for its response, bounded by timeout so a
// scenario can never hang a capture.
func (d *driver) call(method string, params json.RawMessage, timeout time.Duration) (message, error) {
	d.mu.Lock()
	id := d.next
	d.next++
	key := fmt.Sprintf("%d", id)
	waiter := make(chan message, 1)
	d.pending[key] = waiter
	d.mu.Unlock()

	request := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if len(params) > 0 {
		request["params"] = params
	}
	if err := d.write(request); err != nil {
		return message{}, err
	}

	select {
	case reply := <-waiter:
		return reply, nil
	case <-time.After(timeout):
		d.mu.Lock()
		delete(d.pending, key)
		d.mu.Unlock()
		return message{}, fmt.Errorf("timed out waiting for %s", method)
	}
}
