/* @you-factory-meta
{
  "name": "@you/spawn",
  "version": 1,
  "id": "builtin-spawn",
  "description": {
    "type": "LOCALIZABLE_ASSET",
    "value": "Plans an exact number of independent tasks, runs them concurrently, and merges their results into one answer."
  },
  "invocationSignature": {
    "parameters": [
      {
        "name": "request",
        "description": "Request to decompose, execute, and merge.",
        "externalName": "to",
        "required": true,
        "bindings": [
          {
            "kind": "POSITIONAL",
            "position": 1
          },
          {
            "kind": "STDIN"
          },
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "count",
        "description": "Exact number of independently executed tasks, from 1 through 14.",
        "externalName": "count",
        "typeHint": "NUMBER_STRING",
        "required": true,
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "executorProvider",
        "description": "Optional executor provider for planner, task, and merge agents.",
        "externalName": "worker-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "modelProvider",
        "description": "Optional model provider for planner, task, and merge agents.",
        "externalName": "model-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "model",
        "description": "Optional model for planner, task, and merge agents.",
        "externalName": "worker-model",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      }
    ]
  },
  "examples": [
    {
      "name": "ten-travel-researchers",
      "description": {
        "type": "LOCALIZABLE_ASSET",
        "value": "Plan ten travel research tasks, execute them concurrently, and merge their findings."
      },
      "args": {
        "request": "research the best places to travel",
        "count": "10"
      }
    }
  ],
  "argsSchema": {
    "type": "object",
    "required": [
      "request",
      "count"
    ],
    "properties": {
      "request": {
        "type": "string",
        "minLength": 1
      },
      "count": {
        "type": "integer",
        "minimum": 1,
        "maximum": 14
      },
      "executorProvider": {
        "type": "string",
        "minLength": 1
      },
      "modelProvider": {
        "type": "string",
        "minLength": 1
      },
      "model": {
        "type": "string",
        "minLength": 1
      }
    },
    "additionalProperties": false
  },
  "defaultPolicy": {
    "mode": "READ_ONLY",
    "maxAgents": 16,
    "concurrency": 8,
    "maxDepth": 1,
    "maxRetries": 0,
    "allowNetwork": false,
    "allowConnectors": false,
    "allowDangerFullAccess": false,
    "writableRoots": []
  }
}
*/

return (async function () {
  const requiredCalls = args.count + 2;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "spawn requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }
  phase("task-planning");
  const plan = await agent.run({
    label: "spawn-planner",
    prompt: "You are a task planner with zero prior context. Read the complete request below and decompose it into exactly " + args.count + " distinct, non-empty tasks that can be executed independently. Each task string must be a standalone specification for a fresh agent: include its objective, necessary request context, boundaries, expected output, and success criteria. Avoid overlaps and hidden dependencies. Return only a JSON array of strings with exactly that length.\n\nRequest:\n" + args.request,
    executorProvider: args.executorProvider || "",
    modelProvider: args.modelProvider || "",
    model: args.model || "",
    skipPermissions: true,
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
      prompt: "You are an independent executor with zero shared context. Read the overall request and assigned standalone task below in full. Complete only that task, validate important assumptions using available evidence, cover material edge cases, and return a self-contained result with findings, sources or verification, caveats, and the exact outcome needed by a final merger. Do not defer to another task or assume the merger can infer missing reasoning.\n\nOverall request:\n" +
        args.request + "\n\nAssigned task " + (index + 1) + " of " + tasks.length + ":\n" + tasks[index],
      executorProvider: args.executorProvider || "",
      modelProvider: args.modelProvider || "",
      model: args.model || "",
      skipPermissions: true,
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
    prompt: "You are the final merger with no context beyond the complete original request and ordered task results below. Read every result, reconcile overlaps, preserve material evidence, caveats, and disagreements, and produce one coherent self-contained answer that directly satisfies every part of the request. Do not omit a task, invent missing evidence, or present an unverified claim as fact.\n\nRequest:\n" + args.request +
      "\n\nOrdered results:\n" + JSON.stringify(findings),
    executorProvider: args.executorProvider || "",
    modelProvider: args.modelProvider || "",
    model: args.model || "",
    skipPermissions: true,
  });
  if (merged.status !== "COMPLETED") {
    throw "spawn merger failed";
  }

  const mergedText = merged.output.text.trim();
  if (!mergedText) {
    throw "spawn merger returned an empty result";
  }
  return mergedText;
})();
