// Resumable checkpoint-state branch fixture: resumed control flow depends on restored state.
return (async function () {
  const resumed = workflow.resumeState();
  if (resumed && resumed.step >= 1) {
    const second = await agent.run({
      prompt: "step-two",
      label: "step-two",
    });
    return {
      path: "from-checkpoint",
      step: resumed.step,
      firstLabel: resumed.firstLabel,
      second: second,
    };
  }

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
    path: "fresh",
    first: first,
    second: second,
  };
})();
