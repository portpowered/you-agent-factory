return (async function () {
  phase("lead-research");

  const topic = args.topic;
  const researchDepth = args.researchDepth === undefined ? 2 : args.researchDepth;
  const maxSubagents = args.maxSubagents === undefined ? 2 : args.maxSubagents;
  const modelProvider = args.modelProvider;
  const model = args.model;
  const reasoningEffort = args.reasoningEffort === undefined ? "medium" : args.reasoningEffort;
  // Longer topics carry enough scope to benefit from the two complementary
  // specialist roles while short questions remain a lead-only investigation.
  const needsSpecialists = args.topic.length >= 40;
  const specialistCount = needsSpecialists ? Math.min(maxSubagents, 2) : 0;
  const requiredCalls = specialistCount + 1;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "deep research requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }

  let specialistFindings = [];
  if (needsSpecialists && maxSubagents > 0) {
    phase("specialist-research");
    specialistFindings = await parallel([
      {
        label: "research-specialist-technical",
        prompt: "Investigate technical mechanisms and supporting evidence for the research topic at breadth level " + researchDepth + ".",
        modelProvider: modelProvider || "",
        model: model || "",
        reasoningEffort: reasoningEffort,
      },
      {
        label: "research-specialist-tradeoffs",
        prompt: "Investigate practical trade-offs, risks, and counterarguments for the research topic at breadth level " + researchDepth + ".",
        modelProvider: modelProvider || "",
        model: model || "",
        reasoningEffort: reasoningEffort,
      },
    ].slice(0, maxSubagents));
    for (let index = 0; index < specialistFindings.length; index += 1) {
      if (specialistFindings[index].status !== "COMPLETED") {
        throw "research specialist " + (index + 1) + " failed";
      }
    }
  }

  phase("lead-synthesis");
  const specialistEvidence = specialistFindings.length === 0
    ? "No specialist findings were requested."
    : specialistFindings[0].output.text + (specialistFindings.length > 1 ? "\n" + specialistFindings[1].output.text : "");
  const leadSynthesis = await agent.run({
    label: "lead-research-synthesis",
    prompt: "You are the lead researcher. Investigate and synthesize a coherent answer for the topic: " + topic + ". " +
      "Use breadth level " + researchDepth + ". Incorporate these completed specialist findings when present: " +
      specialistEvidence,
    modelProvider: modelProvider || "",
    model: model || "",
    reasoningEffort: reasoningEffort,
  });
  if (leadSynthesis.status !== "COMPLETED") {
    throw "lead research synthesis failed";
  }

  return {
    topic: topic,
    role: "lead-researcher",
    researchDepth: researchDepth,
    maxSubagents: maxSubagents,
    execution: {
      modelProvider: modelProvider,
      model: model,
      reasoningEffort: reasoningEffort,
    },
    synthesis: {
      leadResult: leadSynthesis.output,
    },
  };
})();
