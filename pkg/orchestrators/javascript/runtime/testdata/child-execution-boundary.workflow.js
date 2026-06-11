return (async function () {
  const single = await agent.run({ prompt: "single", label: "agent-run-boundary" });

  const parallelResults = await parallel([
    { prompt: "parallel-a", label: "parallel-boundary-a" },
    { prompt: "parallel-b", label: "parallel-boundary-b" },
  ]);

  const pipelineResults = await pipeline(
    ["item-a"],
    function (_item, index) {
      if (index === 0) {
        return agent.run({ prompt: "edit item-a", label: "pipeline-edit-boundary" });
      }
      return agent.run({ prompt: "edit fallback", label: "pipeline-edit-fallback" });
    },
    function (_prior, _item, index) {
      if (index === 0) {
        return agent.run({ prompt: "review item-a", label: "pipeline-review-boundary" });
      }
      return agent.run({ prompt: "review fallback", label: "pipeline-review-fallback" });
    }
  );

  return workflow.final({
    label: "child-execution-boundary",
    single,
    parallel: parallelResults,
    pipeline: pipelineResults,
  });
})();
