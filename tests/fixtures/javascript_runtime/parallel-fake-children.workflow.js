// Parallel fake child fanout fixture for runtime boundary tests.
return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-0" },
    { prompt: "summarize beta", label: "child-1" },
    { prompt: "summarize gamma", label: "child-2" },
    { prompt: "summarize delta", label: "child-3" },
  ]);
  return {
    label: meta.name,
    subject: args.subject,
    results: results,
  };
})();
