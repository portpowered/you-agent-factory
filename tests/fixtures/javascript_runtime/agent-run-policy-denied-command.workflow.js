// Policy denial fixture: disallowed command.
agent.run({
  prompt: "summarize workflows",
  label: "denied-command",
  command: "deploy",
});
return { ok: true };
