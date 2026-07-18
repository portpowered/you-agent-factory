return (async function () {
  const first = await agent.run({
    prompt: "stage-one-input",
    label: "stage-one",
  });
  const stageTwoPrompt =
    "stage-two-after:" + first.dispatchId + ":" + first.status + ":" + first.output.text;
  const second = await agent.run({
    prompt: stageTwoPrompt,
    label: "stage-two",
  });

  if (first.status !== "COMPLETED" || second.status !== "COMPLETED") {
    throw new Error("ordered pipeline did not complete both stages");
  }
  return {
    finalValue: "ordered-pipeline-complete",
    stages: [
      {
        stage: first.label,
        childIndex: first.childIndex,
        dispatchId: first.dispatchId,
        resultStatus: first.status,
        response: first.output.text,
      },
      {
        stage: second.label,
        childIndex: second.childIndex,
        dispatchId: second.dispatchId,
        resultStatus: second.status,
        response: second.output.text,
      },
    ],
    dependency: {
      priorDispatchId: first.dispatchId,
      observedByStageTwo: second.output.text.indexOf(stageTwoPrompt) !== -1,
    },
  };
})();
