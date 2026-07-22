// Policy denial fixture: requested child concurrency above policy limit.
agent.run({
  prompt: "summarize workflows",
  label: "denied-concurrency",
  concurrency: 4,
});
return { ok: true };
