---
author: Agent Factory Team
last-modified: 2026-09-27
doc-id: agent-factory/guides/mcp
---

# MCP Host Setup

Use the MCP server to invoke a subagent and discover its configuration.
`you docs mcp` is the packaged setup topic.

## Start the server

An MCP host launches `you` as a child process:

```bash
you server mcp
```

The server speaks MCP JSON-RPC over stdin and stdout. Keep stdout reserved for
protocol messages; diagnostics use stderr. Set the working directory to the
project the subagent should inspect. A generic host configuration is:

```json
{
  "mcpServers": {
    "you-agent-factory": {
      "command": "/absolute/path/to/you",
      "args": ["server", "mcp"],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

For OpenCode, add the local server with its installed CLI:

```text
opencode mcp add you-agent-factory --global -- /absolute/path/to/you server mcp
opencode mcp list
```

Confirm the list shows `you-agent-factory` connected and set the OpenCode
workspace to the project. Restart or reload a host after changing its MCP
configuration.

## Run a subagent

The MCP tool catalog contains `you.subagent`. Call it with a short prompt:

```json
{"prompt":"Summarize the purpose of this repository in one sentence."}
```

The tool runs one packaged `@you/subagent` invocation and returns its answer as
`result.text`. Optional `provider`, `model`, and `reasoningEffort` fields select
the route for that call. Omit them to use the operator's configured defaults.
The packaged subagent requests permission skipping by default. Built-in
OpenCode ACP supports this through permission requests; custom ACP integrations
without a declared bypass capability ignore the request and retain their
normal permission handling.

The optional `timeoutMillis` is a wait budget in milliseconds. For example,
`3600000` requests one hour; the MCP host must also allow a call of that
duration. If the host ends the request early, the same call cannot return the
eventual answer.

If no provider default is configured, run `you init --provider codex` or pass
`provider` in the tool call.

## Discover configuration and providers

The server publishes the `subagent-configuration` skill through the MCP Skills
extension. A supporting host can call `skills/list` and `skills/get`, then
read `skill://subagent-configuration/SKILL.md` with `resources/read`. The skill
explains the configuration file, provider names, and custom ACP setup.

Use `resources/list` to discover these resources:

| Resource URI | Content |
|--------------|---------|
| `skill://subagent-configuration/SKILL.md` | Subagent configuration instructions |
| `you://operator/config/schema` | JSON Schema derived from the operator OpenAPI contract |
| `you://operator/config/current` | Current operator configuration file, or `{}` when absent |
| `you://providers/catalog` | Current provider names, models, reasoning efforts, and readiness |

The current configuration is read on each request. Its file is
`~/.you-agent-factory/config.json` on macOS and Linux, or
`%USERPROFILE%\.you-agent-factory\config.json` on Windows. Keep sensitive
values from that file out of shared transcripts. The skill describes how to
modify the file or use the `you` CLI to add a custom provider.

The published MCP manifest in `@you-agent-factory/api/mcp` defines the tool,
resources, and skill available from the server.

## Verify the connection

After saving the host configuration, reload the host. Confirm that
`tools/list` contains `you.subagent`, `resources/list` contains the four
resources above, and `skills/list` contains `subagent-configuration`. Read the
catalog and configuration, then call `you.subagent` with a small prompt. For a
custom provider, configure it as described by the skill and run another small
call using its name.

Run repository checks with:

```bash
go test ./pkg/transports/cli/mcp/... ./pkg/transports/mcp/...
go test ./tests/functional/transport/mcp/...
```

## Troubleshoot

| Symptom | Action |
|---------|--------|
| Host cannot start `you` | Use an absolute executable path and pass `server` and `mcp` as separate arguments. |
| No tools or resources appear | Reload the host, inspect child-process stderr, and confirm stdout has only protocol messages. |
| Provider is unavailable | Read `you://providers/catalog` for its identity and readiness, then check its installation and authentication. |
| Call times out | Increase `timeoutMillis` and the host's own timeout for work expected to run longer. |
| Host expects an HTTP URL | Configure a stdio child process; HTTP and SSE MCP transports are unsupported. |

## Related topics

- `you docs providers` — provider identities and integration setup
- `you docs sessions` — inspect live Factory Sessions from the CLI
