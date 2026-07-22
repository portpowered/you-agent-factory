// Fake child agent.run fixture for runtime boundary tests.
return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    preset: "careful",
    modelProvider: "codex",
    model: "gpt-test",
    reasoningEffort: "medium",
  });
  return {
    label: meta.name,
    subject: args.subject,
    child: child,
  };
})();
