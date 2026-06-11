// Policy denial fixture: disallowed reasoning effort.
agent.run({
  prompt: "summarize workflows",
  label: "denied-reasoning",
  reasoningEffort: "high",
});
return { ok: true };
