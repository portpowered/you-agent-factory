// Functional owner: sessions/chat_sessions/root_composition.
package root_composition_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	chatCleanupTimeout        = 5 * time.Second
	chatCleanupCloseRequestID = 9001
)

func chatIdentity(value any) string {
	if value == nil {
		return ""
	}
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return ""
	}
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
		return fmt.Sprintf("%T:%x", value, reflected.Pointer())
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func buildChatProcess(
	t testing.TB,
	_ string,
	edges serviceedges.Edges,
) (support.ApplicationProcess, error) {
	t.Helper()
	return support.BuildProcessWithContext(context.Background(), edges)
}

func closeChatProcess(process support.ApplicationProcess) error {
	if process == nil {
		return nil
	}
	server := process.ACPServer()
	if err := process.Close(context.Background()); err != nil {
		return err
	}
	chatACPServerHomes.Delete(chatIdentity(server))
	return nil
}

func serveChatRequest(server support.ACPServer, ctx context.Context, in io.Reader, out io.Writer) error {
	home := chatACPHomeForServer(server)
	if home == "" {
		return server.Serve(ctx, in, out)
	}
	return server.Serve(acp.WithInvocationProfile(ctx, acp.InvocationProfile{
		HomeDir: home, WorkerModelProvider: "codex", WorkerModel: "gpt-5",
	}), in, out)
}

type chatPipeEndpoint struct {
	file *os.File
	once sync.Once
	err  error
}

func newChatPipeEndpoint(file *os.File, _ string) *chatPipeEndpoint {
	return &chatPipeEndpoint{file: file}
}

func (p *chatPipeEndpoint) Close() error {
	if p == nil {
		return nil
	}
	p.once.Do(func() {
		p.err = normalizeChatPipeCloseError(p.file.Close())
	})
	return p.err
}

func normalizeChatPipeCloseError(err error) error {
	if err == nil || errors.Is(err, os.ErrClosed) {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "file already closed") {
		return nil
	}
	return err
}

func trackChatSessionOnServer(t testing.TB, server support.ACPServer, sessionID string) {
	t.Helper()
	trackChatSession(t, sessionID, func() error {
		return closeChatSessionViaServer(server, sessionID)
	})
}

func trackChatSessionOnConnection(t testing.TB, stdin *os.File, stdout *bufio.Reader, sessionID string) {
	t.Helper()
	trackChatSession(t, sessionID, func() error {
		return closeChatSessionOnConnection(stdin, stdout, sessionID)
	})
}

func trackChatSession(t testing.TB, sessionID string, closeSession func() error) {
	t.Helper()
	if strings.TrimSpace(sessionID) == "" {
		t.Fatal("Chat Session ID is blank")
	}
	t.Cleanup(func() {
		if err := closeSession(); err != nil && !chatTerminalSessionCloseFallback(err) {
			t.Errorf("Chat Session %q session/close failed: %v", sessionID, err)
		}
	})
}

func chatTerminalSessionCloseFallback(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "malformed_request")
}

func closeChatSessionViaServer(server support.ACPServer, sessionID string) error {
	params, err := json.Marshal(map[string]string{"sessionId": sessionID})
	if err != nil {
		return fmt.Errorf("marshal session/close params: %w", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/close","params":%s}`+"\n", chatCleanupCloseRequestID, params)
	var out bytes.Buffer
	if err := serveChatRequest(server, context.Background(), strings.NewReader(line), &out); err != nil {
		return fmt.Errorf("Serve(session/close): %w", err)
	}
	responses := responseLinesOnlyErr(&out)
	if len(responses) != 1 {
		return fmt.Errorf("session/close response count = %d, want exactly 1", len(responses))
	}
	if responses[0].Error != nil {
		return fmt.Errorf("session/close RPC error: %s", responses[0].Error.Error())
	}
	return nil
}

func closeChatSessionOnConnection(stdin *os.File, stdout *bufio.Reader, sessionID string) error {
	params, err := json.Marshal(map[string]string{"sessionId": sessionID})
	if err != nil {
		return fmt.Errorf("marshal session/close params: %w", err)
	}
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"session/close","params":%s}`+"\n", chatCleanupCloseRequestID, params)
	if _, err := stdin.Write([]byte(line)); err != nil {
		return fmt.Errorf("write session/close: %w", err)
	}
	result := make(chan error, 1)
	go func() {
		for {
			raw, err := stdout.ReadBytes('\n')
			if err != nil {
				result <- fmt.Errorf("read session/close response: %w", err)
				return
			}
			var decoded serveACPLine
			if err := json.Unmarshal(bytes.TrimSpace(raw), &decoded); err != nil {
				result <- fmt.Errorf("decode session/close response: %w", err)
				return
			}
			if decoded.Method != "" {
				continue
			}
			if strings.TrimSpace(string(decoded.ID)) != fmt.Sprint(chatCleanupCloseRequestID) {
				result <- fmt.Errorf("session/close response ID = %s, want %d", decoded.ID, chatCleanupCloseRequestID)
				return
			}
			if decoded.Error != nil {
				result <- fmt.Errorf("session/close RPC error: %s", decoded.Error.Error())
				return
			}
			result <- nil
			return
		}
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(chatCleanupTimeout):
		_ = stdin.Close()
		return errors.New("timed out waiting for session/close response")
	}
}

func chatTempDir(t testing.TB, _ string, prefix string) string {
	t.Helper()
	path, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("create temporary path %q: %v", prefix, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove temporary path %q: %v", path, err)
		}
	})
	return path
}

func chatMkdirTemp(t testing.TB, _ string, parent, prefix string) string {
	t.Helper()
	path, err := os.MkdirTemp(parent, prefix)
	if err != nil {
		t.Fatalf("create temporary path %q: %v", prefix, err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove temporary path %q: %v", path, err)
		}
	})
	return path
}

func chatPersistentMkdirTemp(_ string, prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

func chatRemoveRoot(root string) error { return os.RemoveAll(root) }
