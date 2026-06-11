// Staged pipeline fixture: edit then review per item with fake child execution.
return (async function () {
  const items = ["alpha", "beta", "gamma"];
  const results = await pipeline(
    items,
    function (item, index) {
      if (index === 0) {
        return agent.run({ prompt: "edit alpha", label: "edit-0" });
      }
      if (index === 1) {
        return agent.run({ prompt: "edit beta", label: "edit-1" });
      }
      return agent.run({ prompt: "edit gamma", label: "edit-2" });
    },
    function (editResult, item, index) {
      if (index === 0) {
        return agent.run({ prompt: "review alpha", label: "review-0" });
      }
      if (index === 1) {
        return agent.run({ prompt: "review beta", label: "review-1" });
      }
      return agent.run({ prompt: "review gamma", label: "review-2" });
    }
  );
  return {
    label: meta.name,
    subject: args.subject,
    results: results,
  };
})();
