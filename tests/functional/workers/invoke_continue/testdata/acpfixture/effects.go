package acpfixture

import (
	"io"
	"os/exec"
	"runtime"
	"sync"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

// CommandFactory supplies only an inert host process for the
// supervisor's concrete exec.Cmd Start/Wait contract. Provider protocol and
// output come exclusively from StdioFactory. It never invokes a
// provider or a test executable, and provides no OS control evidence.
func CommandFactory() platformprocess.CommandFactory {
	return func(string, ...string) *exec.Cmd {
		if runtime.GOOS == "windows" {
			return exec.Command("cmd.exe", "/d", "/c", "exit", "0")
		}
		return exec.Command("/bin/sh", "-c", ":")
	}
}

// StdioFactory gives each attempt independent in-memory streams and
// selects an immutable peer route by the customer's working directory. The
// callback must return when requests is closed. Close joins its owned peer.
func StdioFactory(serve func(string, io.Reader, io.Writer)) platformprocess.StdioPipeFactory {
	return func() (platformprocess.StdioChannel, error) {
		requestReader, requestWriter := io.Pipe()
		responseReader, responseWriter := io.Pipe()
		return &acpInMemoryStdio{requestReader: requestReader, requestWriter: requestWriter,
			responseReader: responseReader, responseWriter: responseWriter, serve: serve, done: make(chan struct{})}, nil
	}
}

type acpInMemoryStdio struct {
	requestReader  *io.PipeReader
	requestWriter  *io.PipeWriter
	responseReader *io.PipeReader
	responseWriter *io.PipeWriter
	serve          func(string, io.Reader, io.Writer)
	directory      string
	started        bool
	done           chan struct{}
	once           sync.Once
}

func (channel *acpInMemoryStdio) Attach(cmd *exec.Cmd) { channel.directory = cmd.Dir }

func (channel *acpInMemoryStdio) Detach() {
	channel.started = true
	go func() {
		defer close(channel.done)
		defer channel.responseWriter.Close()
		channel.serve(channel.directory, channel.requestReader, channel.responseWriter)
	}()
}

func (channel *acpInMemoryStdio) Close() {
	channel.once.Do(func() {
		_ = channel.requestReader.Close()
		_ = channel.requestWriter.Close()
		_ = channel.responseReader.Close()
		_ = channel.responseWriter.Close()
		if channel.started {
			<-channel.done
		}
	})
}

func (channel *acpInMemoryStdio) Requests() io.WriteCloser { return channel.requestWriter }
func (channel *acpInMemoryStdio) Responses() io.ReadCloser { return channel.responseReader }
