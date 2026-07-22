// Parallel child failure fixture for runtime boundary tests.
return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-0" },
    { prompt: "fail:simulated child error", label: "child-1" },
    { prompt: "summarize gamma", label: "child-2" },
  ]);
  return {
    label: meta.name,
    subject: args.subject,
    results: results,
  };
})();
