// Resumable checkpoint-state branch fixture: resumed control flow depends on restored state.
return (async function () {
  const first = await agent.run({
    prompt: "step-one",
    label: "step-one",
  });
})();
