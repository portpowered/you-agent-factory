# ACP capability comparison

Computed from captured transcripts by `acpbaseline compare`. Every **GAP** row is a work item: a capability at least one third-party agent exhibits and `you serve acp` does not.

Captures compared:

- `you` — 2026-08-06T19:57:52Z, scenarios: 01-initialize, 02-capabilities-full, 03-config-and-models, 04-thinking-and-tools, 05-subagent
- `cursor-agent` — 2026-08-06T19:58:51Z, scenarios: 01-initialize, 02-capabilities-full, 03-config-and-models, 04-thinking-and-tools, 05-subagent

Model and option identities are account-entitlement-scoped, so verdicts key on a capability's existence and category, never on the exact option ids.

> **Caveat (you):** prompts completed but produced no agent_message_chunk: the capture environment most likely had no model provider configured, so message-bearing rows are not evidence of a missing capability

| Capability | you | cursor-agent | Verdict |
|---|---|---|---|
| `agent method initialize` | yes | yes | PARITY |
| `agent method session/new` | yes | yes | PARITY |
| `agent method session/prompt` | yes | yes | PARITY |
| `agent method session/select_model` | no (-32601) | no (-32601) | PARITY |
| `agent method session/set_config_option` | yes | yes | PARITY |
| `agent method session/set_mode` | no (-32601) | yes | GAP |
| `agent method session/set_model` | no (-32601) | no (-32602) | PARITY |
| `capability auth` | yes | no | EXTRA |
| `capability authMethods[1]` | no | yes | GAP |
| `capability loadSession` | yes | yes | PARITY |
| `capability mcpCapabilities` | yes | no | EXTRA |
| `capability mcpCapabilities.http` | no | yes | GAP |
| `capability mcpCapabilities.sse` | no | yes | GAP |
| `capability promptCapabilities` | yes | no | EXTRA |
| `capability promptCapabilities.image` | no | yes | GAP |
| `capability sessionCapabilities.close` | yes | no | EXTRA |
| `capability sessionCapabilities.list` | no | yes | GAP |
| `capability sessionCapabilities.resume` | yes | no | EXTRA |
| `client method cursor/task` | no | yes | GAP |
| `config option category=mode` | no | yes (3 options) | GAP |
| `config option category=model` | yes (15 options) | no | EXTRA |
| `session/update -> agent_message_chunk` | no | yes (28) | GAP |
| `session/update -> agent_thought_chunk` | no | yes (25) | GAP |
| `session/update -> available_commands_update` | yes (5) | yes (4) | PARITY |
| `session/update -> current_mode_update` | no | yes (2) | GAP |
| `session/update -> session_info_update` | no | yes (3) | GAP |
| `session/update -> tool_call` | yes (3) | yes (3) | PARITY |
| `session/update -> tool_call_update` | yes (3) | yes (6) | PARITY |

**12 GAP row(s).**

## Work items

- `you serve acp` does not exhibit `agent method session/set_mode`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `capability authMethods[1]`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `capability mcpCapabilities.http`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `capability mcpCapabilities.sse`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `capability promptCapabilities.image`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `capability sessionCapabilities.list`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `client method cursor/task`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `config option category=mode`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `session/update -> agent_message_chunk`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `session/update -> agent_thought_chunk`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `session/update -> current_mode_update`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
- `you serve acp` does not exhibit `session/update -> session_info_update`, which cursor-agent does. Decide whether to implement it or record why it does not apply.
