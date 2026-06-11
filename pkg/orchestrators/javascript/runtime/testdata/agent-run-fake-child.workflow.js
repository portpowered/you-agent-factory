// Fake child agent.run fixture for runtime boundary tests.
return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    model: "gpt-test",
    reasoningEffort: "medium",
    command: "review",
    sandbox: "read-only",
    outputSchema: { type: "object", properties: { text: { type: "string" } } },
  });
  return {
    label: meta.name,
    subject: args.subject,
    child: child,
  };
})();
