// Package stdio owns policy-free adapters for finite standard-I/O streams.
package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
)

// DrainJSONRPCResponses keeps finite input open until responses for all
// already-read JSON-RPC requests have been written.
func DrainJSONRPCResponses(ctx context.Context, in io.Reader, out io.Writer) (io.ReadCloser, io.WriteCloser) {
	drain := &responseDrain{ctx: ctx, changed: make(chan struct{})}
	return &drainReader{ReadCloser: io.NopCloser(in), drain: drain}, &drainWriter{Writer: out, drain: drain}
}

type responseDrain struct {
	ctx     context.Context
	mu      sync.Mutex
	pending int
	changed chan struct{}
}

func (d *responseDrain) observe(payload []byte, input bool) {
	var envelope struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if json.Unmarshal(payload, &envelope) != nil || len(envelope.ID) == 0 || string(envelope.ID) == "null" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if input && envelope.Method != "" {
		d.pending++
	} else if !input && envelope.Method == "" && d.pending > 0 {
		d.pending--
	}
	close(d.changed)
	d.changed = make(chan struct{})
}

func (d *responseDrain) wait() error {
	for {
		d.mu.Lock()
		if d.pending == 0 {
			d.mu.Unlock()
			return nil
		}
		changed := d.changed
		d.mu.Unlock()
		select {
		case <-changed:
		case <-d.ctx.Done():
			return d.ctx.Err()
		}
	}
}

type drainReader struct {
	io.ReadCloser
	drain  *responseDrain
	buffer []byte
	eof    bool
}

func (r *drainReader) Read(payload []byte) (int, error) {
	if r.eof {
		return 0, r.finish()
	}
	n, err := r.ReadCloser.Read(payload)
	r.buffer = observeLines(r.buffer, payload[:n], func(line []byte) { r.drain.observe(line, true) })
	if !errors.Is(err, io.EOF) {
		return n, err
	}
	r.eof = true
	if n > 0 {
		return n, nil
	}
	return 0, r.finish()
}

func (r *drainReader) finish() error {
	if len(strings.TrimSpace(string(r.buffer))) > 0 {
		r.drain.observe(r.buffer, true)
		r.buffer = nil
	}
	if err := r.drain.wait(); err != nil {
		return err
	}
	return io.EOF
}

type drainWriter struct {
	io.Writer
	drain  *responseDrain
	buffer []byte
}

func (w *drainWriter) Write(payload []byte) (int, error) {
	n, err := w.Writer.Write(payload)
	w.buffer = observeLines(w.buffer, payload[:n], func(line []byte) { w.drain.observe(line, false) })
	return n, err
}

func (w *drainWriter) Close() error { return nil }

func observeLines(buffer, payload []byte, observe func([]byte)) []byte {
	buffer = append(buffer, payload...)
	for {
		newline := bytes.IndexByte(buffer, '\n')
		if newline < 0 {
			return buffer
		}
		line := bytes.TrimSpace(buffer[:newline])
		if len(line) > 0 {
			observe(line)
		}
		buffer = append(buffer[:0], buffer[newline+1:]...)
	}
}
