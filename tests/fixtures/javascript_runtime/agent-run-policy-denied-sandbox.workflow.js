// Policy denial fixture: workspace-write sandbox under READ_ONLY policy.
agent.run({
  prompt: "summarize workflows",
  label: "denied-sandbox",
  sandbox: "workspace-write",
});
return { ok: true };
