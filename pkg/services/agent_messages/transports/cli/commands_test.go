package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func messageCommandRoot(adapter *Adapter, config Config) (*cobra.Command, error) {
	command, err := adapter.Command(config)
	if err != nil {
		return nil, err
	}
	manifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, err
	}
	record := manifest.Commands["you"]
	manifest.Commands = map[string]climanifest.Command{"you": record}
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		Handlers: climanifestcobra.HandlerRegistry{record.Handler.ID: func(context.Context, map[string]any) error { return nil }},
	})
	if err != nil {
		return nil, err
	}
	root.AddCommand(command)
	return root, nil
}

func TestMessageCommandMapsSendAndReplyInputs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, stdin string
		args        []string
		to, reply   string
		body        string
	}{
		{name: "positional", args: []string{"lead", "two", "words"}, to: "lead", body: "two words"},
		{name: "reserved explicit recipient", args: []string{"--worker-session-id", "reply", "--body", "flag body"}, to: "reply", body: "flag body"},
		{name: "stdin", args: []string{"lead"}, stdin: "stdin body", to: "lead", body: "stdin body"},
		{name: "reply default recipient", args: []string{"reply", "msg-parent", "answer"}, reply: "msg-parent", body: "answer"},
		{name: "reply explicit recipient", args: []string{"reply", "msg-parent", "--worker-session-id", "lead", "--body", "answer"}, to: "lead", reply: "msg-parent", body: "answer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			protocol := protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
				calls++
				payload := decodeCommandSend(t, request)
				to := ""
				if payload.To != nil {
					to = payload.To.WorkerSessionID
				}
				if to != test.to || payload.InReplyTo != test.reply || payload.Body != test.body || payload.RequestID != "stable-request" {
					t.Fatalf("unexpected send: %+v", payload)
				}
				if payload.ExpiresInSeconds != nil || payload.IfEnded != "" || payload.ReplyIfEnded != "" {
					t.Fatal("omitted flags replaced server defaults or reply inheritance")
				}
				return clihttp.Response{HTTP: &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader(`{"message":` + messageJSON + `}`))}}, nil
			}}
			adapter := NewAdapter(protocol, protocol, nil)
			var output bytes.Buffer
			config := workerConfig(&output)
			command, err := messageCommandRoot(adapter, config)
			if err != nil {
				t.Fatal(err)
			}
			command.SetOut(&output)
			command.SetIn(strings.NewReader(test.stdin))
			command.SetArgs(append(append([]string{"message"}, test.args...), "--session", "explicit-scope", "--request-id", "stable-request", "--output", "json"))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !json.Valid(output.Bytes()) {
				t.Fatal("command did not emit one structured admitted response")
			}
		})
	}
}

func decodeCommandSend(t *testing.T, request *http.Request) agentmessages.SendRequest {
	t.Helper()
	if request.URL.Host != "selected.test" || request.URL.Query().Get("factorySessionId") != "explicit-scope" {
		t.Fatal("command lost explicit host or session")
	}
	var payload agentmessages.SendRequest
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestMessageCommandPreservesInboxFilterPresence(t *testing.T) {
	t.Parallel()
	protocol := protocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
		query := request.URL.Query()
		for key, expected := range map[string]string{"toMe": "true", "markRead": "true", "maxResults": "0", "threadId": "thread", "nextToken": "cursor"} {
			if query.Get(key) != expected {
				t.Fatalf("%s = %q, want %q", key, query.Get(key), expected)
			}
		}
		if !reflect.DeepEqual(query["status"], []string{"QUEUED", "READ"}) {
			t.Fatalf("repeated status = %v", query["status"])
		}
		return clihttp.Response{HTTP: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"messages":[]}`))}}, nil
	}}
	adapter := NewAdapter(protocol, protocol, nil)
	var output bytes.Buffer
	command, err := messageCommandRoot(adapter, workerConfig(&output))
	if err != nil {
		t.Fatal(err)
	}
	command.SetOut(&output)
	command.SetArgs([]string{"message", "list", "--to-me", "--mark-read", "--max-results", "0", "--status", "QUEUED", "--status", "READ", "--thread", "thread", "--next-token", "cursor", "--output", "json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != `{"messages":[],"nextToken":null}` {
		t.Fatalf("unexpected inbox output: %s", output.String())
	}
}

func TestMessageCommandRejectsInvalidInputsBeforeHTTP(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"lead", "--body", ""}, {"lead", "args", "--body", "flag"},
		{"lead", "body", "--expires", "bad"}, {"lead", "body", "--expires", "0s"},
		{"lead", "body", "--operator"}, {"lead", "body", "--to-operator"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			protocol := protocolStub{execute: func(*http.Request) (clihttp.Response, error) {
				t.Fatal("invalid command reached HTTP")
				return clihttp.Response{}, nil
			}}
			var output bytes.Buffer
			command, err := messageCommandRoot(NewAdapter(protocol, protocol, nil), workerConfig(&output))
			if err != nil {
				t.Fatal(err)
			}
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs(append([]string{"message"}, args...))
			if err := command.Execute(); err == nil {
				t.Fatal("invalid command succeeded")
			}
		})
	}
}
