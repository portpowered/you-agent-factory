return (async function () {
  const entrantCount = Math.pow(2, args.rounds);
  const requiredCalls = (entrantCount * 2) - 1;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "tournament requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }
  phase("candidate-generation");

  const competitors = [];
  for (let index = 0; index < entrantCount; index += 1) {
    competitors.push({
      label: "tournament-competitor-" + (index + 1),
      prompt: "Produce candidate " + (index + 1) + " of " + entrantCount +
        " for this request. Work independently and return only the candidate answer.\n\nRequest:\n" + args.request,
      executorProvider: args.executorProvider || "",
      modelProvider: args.modelProvider || "",
      model: args.model || "",
    });
  }

  const generated = await parallel(competitors);
  let bracket = [];
  for (let index = 0; index < generated.length; index += 1) {
    if (generated[index].status !== "COMPLETED") {
      throw "tournament competitor " + (index + 1) + " failed";
    }
    bracket.push({
      entrant: index + 1,
      answer: generated[index].output.text,
      rationale: [],
    });
  }

  for (let round = 1; round <= args.rounds; round += 1) {
    phase("round-" + round);
    const matches = [];
    for (let match = 0; match < bracket.length / 2; match += 1) {
      const candidateA = bracket[match * 2];
      const candidateB = bracket[match * 2 + 1];
      matches.push({
        label: "tournament-judge-r" + round + "-m" + (match + 1),
        prompt: "Judge this 1v1 match for the original request. Select exactly candidate A or B; do not invent a replacement answer. " +
          "Return only JSON shaped {\"winner\":\"A\"|\"B\",\"rationale\":\"...\"}.\n\nRequest:\n" + args.request +
          "\n\nCandidate A:\n" + candidateA.answer + "\n\nCandidate B:\n" + candidateB.answer,
        executorProvider: args.judgeExecutorProvider || args.executorProvider || "",
        modelProvider: args.judgeModelProvider || args.modelProvider || "",
        model: args.judgeModel || args.model || "",
      });
    }

    const judgments = await parallel(matches);
    const advanced = [];
    for (let match = 0; match < judgments.length; match += 1) {
      if (judgments[match].status !== "COMPLETED") {
        throw "tournament judge failed at round " + round + " match " + (match + 1);
      }
      let decision;
      try {
        const judgmentText = judgments[match].output.text.trim();
        const judgmentStart = judgmentText.indexOf("{");
        const judgmentEnd = judgmentText.lastIndexOf("}");
        if (judgmentStart < 0 || judgmentEnd < judgmentStart) {
          throw "missing judgment object";
        }
        decision = JSON.parse(judgmentText.slice(judgmentStart, judgmentEnd + 1));
      } catch (_) {
        throw "tournament judge returned invalid JSON at round " + round + " match " + (match + 1);
      }
      if (decision.winner !== "A" && decision.winner !== "B") {
        throw "tournament judge must select A or B at round " + round + " match " + (match + 1);
      }
      const winner = decision.winner === "A" ? bracket[match * 2] : bracket[match * 2 + 1];
      winner.rationale.push({
        round: round,
        match: match + 1,
        selected: decision.winner,
        rationale: decision.rationale || "",
      });
      advanced.push(winner);
    }
    bracket = advanced;
  }

  return {
    request: args.request,
    rounds: args.rounds,
    entrantCount: entrantCount,
    champion: bracket[0],
  };
})();
