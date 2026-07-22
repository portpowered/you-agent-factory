// Pipeline stage failure fixture: second stage fails for one item.
return (async function () {
  const items = ["alpha", "beta"];
  const results = await pipeline(
    items,
    function (item, index) {
      if (index === 0) {
        return agent.run({ prompt: "edit alpha", label: "edit-0" });
      }
      return agent.run({ prompt: "edit beta", label: "edit-1" });
    },
    function (editResult, item, index) {
      if (index === 1) {
        return agent.run({ prompt: "fail:review rejected", label: "review-1" });
      }
      const review = agent.run({ prompt: "review alpha", label: "review-0" });
      return {
        priorText: editResult.output.text,
        review: review,
      };
    }
  );
  return {
    label: meta.name,
    subject: args.subject,
    results: results,
  };
})();
