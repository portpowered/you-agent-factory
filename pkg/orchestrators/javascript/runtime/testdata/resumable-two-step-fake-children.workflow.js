// Resumable two-step fake-child fixture for restart-resume runtime tests.
return (async function () {
  const first = await agent.run({
    prompt: "step-one",
    label: "step-one",
  });
  workflow.checkpoint({
    label: "after-step-one",
    state: { step: 1, firstLabel: first.label },
  });
  const second = await agent.run({
    prompt: "step-two",
    label: "step-two",
  });
  return {
    label: meta.name,
    subject: args.subject,
    first: first,
    second: second,
  };
})();
