return (async function () {
  const requiredCalls = args.count + 2;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "spawn requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }
  phase("task-planning");
  const plan = await agent.run({
    label: "spawn-planner",
    prompt: "Decompose the request into exactly " + args.count + " distinct, non-empty, independent tasks. " +
      "Return only a JSON array of strings with exactly that length.\n\nRequest:\n" + args.request,
    executorProvider: args.executorProvider || "",
    modelProvider: args.modelProvider || "",
    model: args.model || "",
  });
  if (plan.status !== "COMPLETED") {
    throw "spawn planner failed";
  }

  let tasks;
  try {
    const planText = plan.output.text.trim();
    const planStart = planText.indexOf("[");
    const planEnd = planText.lastIndexOf("]");
    if (planStart < 0 || planEnd < planStart) {
      throw "missing task array";
    }
    tasks = JSON.parse(planText.slice(planStart, planEnd + 1));
  } catch (_) {
    throw "spawn planner returned invalid JSON";
  }
  if (!Array.isArray(tasks) || tasks.length !== args.count) {
    throw "spawn planner must return exactly " + args.count + " tasks";
  }
  const seen = {};
  for (let index = 0; index < tasks.length; index += 1) {
    if (typeof tasks[index] !== "string" || tasks[index].trim() === "") {
      throw "spawn planner task " + (index + 1) + " is empty";
    }
    const key = tasks[index].trim().toLowerCase();
    if (seen[key]) {
      throw "spawn planner returned duplicate tasks";
    }
    seen[key] = true;
    tasks[index] = tasks[index].trim();
  }

  phase("task-execution");
  const specs = [];
  for (let index = 0; index < tasks.length; index += 1) {
    specs.push({
      label: "spawn-task-" + (index + 1),
      prompt: "Complete only the assigned task and return concise findings for the merger.\n\nOverall request:\n" +
        args.request + "\n\nAssigned task " + (index + 1) + ":\n" + tasks[index],
      executorProvider: args.executorProvider || "",
      modelProvider: args.modelProvider || "",
      model: args.model || "",
    });
  }
  const results = await parallel(specs);
  const findings = [];
  for (let index = 0; index < results.length; index += 1) {
    if (results[index].status !== "COMPLETED") {
      throw "spawn task " + (index + 1) + " failed";
    }
    findings.push({
      index: index + 1,
      task: tasks[index],
      result: results[index].output.text,
    });
  }

  phase("result-merge");
  const merged = await agent.run({
    label: "spawn-merger",
    prompt: "Merge every ordered task result into one complete answer to the original request. " +
      "Do not omit a task and call out material disagreements.\n\nRequest:\n" + args.request +
      "\n\nOrdered results:\n" + JSON.stringify(findings),
    executorProvider: args.executorProvider || "",
    modelProvider: args.modelProvider || "",
    model: args.model || "",
  });
  if (merged.status !== "COMPLETED") {
    throw "spawn merger failed";
  }

  return {
    request: args.request,
    count: args.count,
    tasks: tasks,
    findings: findings,
    merged: merged.output,
  };
})();
