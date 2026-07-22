// Policy denial fixture: network request when policy disallows network.
agent.run({
  prompt: "summarize workflows",
  label: "denied-network",
  network: true,
});
return { ok: true };
