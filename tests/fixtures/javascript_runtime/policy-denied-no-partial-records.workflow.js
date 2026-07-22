// Policy denial fixture: progress records before denied child should remain intact.
phase("prepare");
log("before denied child");
agent.run({
  prompt: "summarize workflows",
  label: "denied-model",
  model: "gpt-denied",
});
return { ok: true };
