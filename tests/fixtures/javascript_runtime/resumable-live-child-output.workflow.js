// Resumable live-child output fixture: resumed result depends on replayed first child output.text.
return (async function () {
  const first = await agent.run({
    prompt: "step-one",
    label: "step-one",
  });
  if (!workflow.resumeState()) {
    workflow.checkpoint({
      label: "after-step-one",
    });
  }
  const second = await agent.run({
    prompt: "step-two",
    label: "step-two",
  });
  return {
    firstOutputText: first.output.text,
    secondLabel: second.label,
  };
})();
