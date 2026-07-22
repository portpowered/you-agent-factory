// Preset-backed child fixture for durable dispatch and replay inspection.
return (async function () {
  const child = await agent.run({
    prompt: "summarize workflows",
    label: "summarize-findings",
    preset: "careful-review",
  });
  return { label: meta.name, subject: args.subject, child: child };
})();
