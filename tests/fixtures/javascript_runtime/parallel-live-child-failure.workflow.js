// Parallel live child failure fixture for durable dispatch bridge tests.
return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-0" },
    { prompt: "summarize beta and force provider failure", label: "child-1" },
    { prompt: "summarize gamma", label: "child-2" },
  ]);
  return {
    label: meta.name,
    subject: args.subject,
    results: results,
  };
})();
