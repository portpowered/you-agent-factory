// Policy denial fixture: second child exceeds maxAgents budget.
agent.run({
  prompt: "first child",
  label: "first-child",
});
agent.run({
  prompt: "second child",
  label: "second-child",
});
return { ok: true };
