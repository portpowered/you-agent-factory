// Policy denial fixture: writable roots request under read-only policy.
agent.run({
  prompt: "summarize workflows",
  label: "denied-writable-roots",
  writableRoots: ["/tmp/out"],
});
return { ok: true };
