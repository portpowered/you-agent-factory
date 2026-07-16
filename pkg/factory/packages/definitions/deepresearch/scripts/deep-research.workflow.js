return (async function () {
  phase("lead-research");

  const topic = args.topic;
  const researchDepth = args.researchDepth === undefined ? 2 : args.researchDepth;
  const maxSubagents = args.maxSubagents === undefined ? 2 : args.maxSubagents;
  // Longer topics carry enough scope to benefit from the two complementary
  // specialist roles while short questions remain a lead-only investigation.
  const needsSpecialists = args.topic.length >= 40;

  let specialistFindings = [];
  if (needsSpecialists && maxSubagents > 0) {
    phase("specialist-research");
    specialistFindings = await parallel([
      {
        label: "research-specialist-technical",
        prompt: "Investigate technical mechanisms and supporting evidence for the research topic at breadth level " + researchDepth + ".",
      },
      {
        label: "research-specialist-tradeoffs",
        prompt: "Investigate practical trade-offs, risks, and counterarguments for the research topic at breadth level " + researchDepth + ".",
      },
    ].slice(0, maxSubagents));
  }

  phase("lead-synthesis");
  return {
    topic: topic,
    role: "lead-researcher",
    researchDepth: researchDepth,
    maxSubagents: maxSubagents,
    synthesis: {
      summary: "Lead research synthesis for " + topic + ".",
      specialistFindings: specialistFindings,
    },
  };
})();
