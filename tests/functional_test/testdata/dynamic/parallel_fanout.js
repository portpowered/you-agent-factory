return (async function () {
  const childResults = await parallel([
    {
      label: "alpha",
      prompt: "ALPHA_RESULT",
    },
    {
      label: "beta",
      prompt: "BETA_RESULT",
    },
  ]);

  const alpha = childResults[0];
  const beta = childResults[1];
  if (childResults.length !== 2 || alpha.label !== "alpha" || beta.label !== "beta") {
    throw new Error("parallel fanout returned missing, duplicate, or unexpected children");
  }
  if (
    alpha.output.text.indexOf(":alpha:ALPHA_RESULT:") === -1 ||
    beta.output.text.indexOf(":beta:BETA_RESULT:") === -1
  ) {
    throw new Error("parallel fanout cross-correlated deterministic child results");
  }

  const finalValue = "alpha=ALPHA_RESULT;beta=BETA_RESULT";
  return {
    finalValue: finalValue,
    children: [
      {
        name: alpha.label,
        dispatchId: alpha.dispatchId,
        resultStatus: alpha.status,
        response: "ALPHA_RESULT",
        rawResponse: alpha.output.text,
      },
      {
        name: beta.label,
        dispatchId: beta.dispatchId,
        resultStatus: beta.status,
        response: "BETA_RESULT",
        rawResponse: beta.output.text,
      },
    ],
  };
})();
