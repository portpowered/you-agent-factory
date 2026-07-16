return (async function () {
  phase("lead-research");

  const topic = args.topic;
  // Longer topics carry enough scope to benefit from the two complementary
  // specialist roles while short questions remain a lead-only investigation.
  const needsSpecialists = args.topic.length >= 40;

  let specialistFindings = [];
  if (needsSpecialists) {
    phase("specialist-research");
    specialistFindings = await parallel([
      {
        label: "research-specialist-technical",
        prompt: "Investigate technical mechanisms and supporting evidence for the research topic.",
      },
      {
        label: "research-specialist-tradeoffs",
        prompt: "Investigate practical trade-offs, risks, and counterarguments for the research topic.",
      },
    ]);
  }

  phase("lead-synthesis");
  return {
    topic: topic,
    role: "lead-researcher",
    synthesis: {
      summary: "Lead research synthesis for " + topic + ".",
      specialistFindings: specialistFindings,
    },
  };
})();
