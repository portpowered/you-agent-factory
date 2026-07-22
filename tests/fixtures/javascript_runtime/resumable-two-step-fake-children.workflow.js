// Resumable two-step fake-child fixture for restart-resume runtime tests.
return (async function () {
  const resumed = workflow.resumeState();
  let first;
  if (!resumed || resumed.step < 1) {
    first = await agent.run({
      prompt: "step-one",
      label: "step-one",
    });
    workflow.checkpoint({
      label: "after-step-one",
      state: { step: 1, firstLabel: first.label },
    });
  }
  const second = await agent.run({
    prompt: "step-two",
    label: "step-two",
  });
  return {
    label: meta.name,
    subject: args.subject,
    first: first || { label: resumed.firstLabel },
    second: second,
  };
})();
