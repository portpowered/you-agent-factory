// Live child agent.run failure fixture for durable dispatch bridge tests.
return (async function () {
  const child = await agent.run({
    prompt: "fail:simulated live child error",
    label: "summarize-findings",
    model: "gpt-test",
  });
  return {
    label: meta.name,
    subject: args.subject,
    child: child,
  };
})();
