// Progress and state primitive fixture for host API runtime tests.
phase("setup");
log("starting workflow", { step: 1 });
workflow.log("workflow step");
phase("execute");
const artifactRef = workflow.artifact({
  kind: "log",
  label: "step-output",
  content: { message: "hello" },
});
workflow.checkpoint({
  label: "after-artifact",
  state: { artifactRef: artifactRef, step: 2 },
});
const budget = workflow.budget();
return {
  label: meta.name,
  artifactRef: artifactRef,
  budget: budget,
};
