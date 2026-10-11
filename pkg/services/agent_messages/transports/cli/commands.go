package cli

import (
	"context"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

// Command projects the authored message grammar. The adapter is shared; each
// invocation gets its own parser, inputs, environment lookup and output stream.
func (a *Adapter) Command(config Config) (*cobra.Command, error) {
	manifest, err := generated.MessagesFamilyManifest()
	if err != nil {
		return nil, err
	}
	rootManifest, err := generated.RepresentativeFamilyManifest()
	if err != nil {
		return nil, err
	}
	rootRecord := rootManifest.Commands["you"]
	manifest.Commands["you"] = rootRecord
	handlers := climanifestcobra.ResolvedCobraHandlerRegistry{}
	for _, id := range []string{"you.message", "you.message.reply", "you.message.list"} {
		handlers[id+".handler"] = func(cmd *cobra.Command, local, inherited resolvedinput.Inputs) error {
			if a == nil {
				return a.commandError(agentmessages.ErrDisabled)
			}
			values := commandInputs{inputs: local, prefix: id}
			invocation := config
			invocation.Context, invocation.Output = cmd.Context(), cmd.OutOrStdout()
			invocation.SessionID, invocation.OutputFormat = values.text("session"), values.text("output")
			if state, ok := inherited.State("you.flag.server"); ok && !state.Default {
				server, serverErr := inherited.String("you.flag.server")
				if serverErr != nil {
					return serverErr
				}
				invocation.Server = server
			}
			if id == "you.message.list" {
				return a.commandList(invocation, values)
			}
			return a.commandSend(cmd, invocation, values, id == "you.message.reply")
		}
	}
	root, err := climanifestcobra.NewCommandTree(manifest, climanifestcobra.GenericBindings{
		ResolvedCobraHandlers: handlers,
		Handlers:              climanifestcobra.HandlerRegistry{rootRecord.Handler.ID: func(context.Context, map[string]any) error { return nil }},
	})
	if err != nil {
		return nil, err
	}
	command, _, err := root.Find([]string{manifest.Commands["you.message"].Name})
	if err != nil {
		return nil, err
	}
	root.RemoveCommand(command)
	return command, nil
}

func (a *Adapter) commandSend(cmd *cobra.Command, config Config, values commandInputs, reply bool) error {
	var args []string
	if _, present := values.inputs.Lookup(values.prefix + ".arg.0"); present {
		var err error
		args, err = values.inputs.StringArray(values.prefix + ".arg.0")
		if err != nil {
			return a.commandError(agentmessages.ErrBadRequest)
		}
	}
	send := SendConfig{
		Config: config, WorkerSessionID: values.text("worker-session-id"), RequestID: values.text("request-id"),
		BodySecret: values.boolean("body-secret"), IfEnded: values.text("if-ended"),
		ReplyHold: values.boolean("reply-hold"), Interrupt: values.boolean("interrupt"),
		Correlation: values.correlation(), Stdin: cmd.InOrStdin(), StdinIsTTY: startupcli.StdinIsTTY(cmd.Context()),
	}
	if reply {
		if len(args) == 0 {
			return a.commandError(agentmessages.ErrBadRequest)
		}
		send.InReplyTo, args = args[0], args[1:]
	} else if send.WorkerSessionID == "" {
		if len(args) == 0 {
			return a.commandError(agentmessages.ErrBadRequest)
		}
		send.WorkerSessionID, args = args[0], args[1:]
	}
	send.BodyArgs = args
	if values.changed("body") {
		body := values.text("body")
		send.Body = &body
	}
	if values.changed("expires") {
		duration, parseErr := time.ParseDuration(values.text("expires"))
		if parseErr != nil {
			return a.commandError(agentmessages.ErrBadRequest)
		}
		send.Expires = &duration
	}
	return a.Send(send)
}

func (a *Adapter) commandList(config Config, values commandInputs) error {
	list := ListConfig{
		Config: config, ToMe: values.boolean("to-me"), ToWorkerSessionID: values.text("to-worker-session-id"),
		FromWorkerSessionID: values.text("from-worker-session-id"), ThreadID: values.text("thread"),
		Correlation: values.correlation(), NextToken: values.text("next-token"),
		MarkRead: values.boolean("mark-read"), Follow: values.boolean("follow"),
	}
	var err error
	list.Statuses, err = values.inputs.StringArray(values.prefix + ".flag.status")
	if err != nil {
		return a.commandError(agentmessages.ErrBadRequest)
	}
	if values.changed("max-results") {
		limit, limitErr := values.inputs.Int(values.prefix + ".flag.max-results")
		if limitErr != nil {
			return a.commandError(agentmessages.ErrBadRequest)
		}
		list.MaxResults = &limit
	}
	return a.List(list)
}

// The generated manifest fixes these kinds before the handler runs. Presence
// is retained separately so an explicit empty body/zero limit is not omitted.
type commandInputs struct {
	inputs resolvedinput.Inputs
	prefix string
}

func (values commandInputs) text(name string) string {
	value, _ := values.inputs.String(values.prefix + ".flag." + name)
	return value
}

func (values commandInputs) boolean(name string) bool {
	value, _ := values.inputs.Bool(values.prefix + ".flag." + name)
	return value
}

func (values commandInputs) changed(name string) bool {
	state, ok := values.inputs.State(values.prefix + ".flag." + name)
	return ok && state.Changed
}

func (values commandInputs) correlation() agentmessages.Correlation {
	return agentmessages.Correlation{WorkID: values.text("correlation-work-id"), FactorySessionID: values.text("correlation-factory-session")}
}
