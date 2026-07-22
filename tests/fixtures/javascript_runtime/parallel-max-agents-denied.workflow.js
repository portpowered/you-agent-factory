// Parallel maxAgents denial fixture for runtime boundary tests.
parallel([
  { prompt: "summarize alpha", label: "child-0" },
  { prompt: "summarize beta", label: "child-1" },
  { prompt: "summarize gamma", label: "child-2" },
]);
return { label: meta.name };
