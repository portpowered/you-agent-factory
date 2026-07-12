return (async function () {
  phase("review");
  const reviews = await parallel([
    { label: "review-alpha", prompt: "Review alpha" },
    { label: "review-beta", prompt: "Review beta" },
    { label: "review-gamma", prompt: "Review gamma" },
  ]);
  phase("synthesize");
  const synthesis = await agent.run({
    label: "synthesize",
    prompt: "Synthesize the completed reviews",
  });
  return { reviews: reviews, synthesis: synthesis };
})();
