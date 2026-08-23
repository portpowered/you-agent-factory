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
        "defaultValue": "3",
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
      "request"
    ],
    "properties": {
      "request": {
        "type": "string",
        "minLength": 1
      },
      "count": {
        "type": "integer",
        "default": 3,
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
    "maxAgents": 16,
    "concurrency": 8,
    "maxDepth": 1,
    "maxRetries": 0,
    "allowedPermissions": [
      "SKIP_PERMISSIONS"
    ]
  }
}
*/

return (async function () {
  const taskPlanSchema = {
    type: "object",
    properties: {
      tasks: {
        type: "array",
        minItems: args.count,
        maxItems: args.count,
        items: { type: "string", minLength: 1 },
      },
    },
    required: ["tasks"],
    additionalProperties: false,
  };
  const taskResultSchema = {
    type: "object",
    properties: {
      result: { type: "string", minLength: 1 },
    },
    required: ["result"],
    additionalProperties: false,
  };
  const mergerResultSchema = {
    type: "object",
    properties: {
      answer: { type: "string", minLength: 1 },
    },
    required: ["answer"],
    additionalProperties: false,
  };
  const requiredCalls = args.count + 2;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "spawn requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }
  phase("task-planning");
  const plan = await agent.run({
    label: "spawn-planner",
    prompt: "You are a task planner with zero prior context. Read the complete request below and decompose it into exactly " + args.count + " distinct, non-empty tasks that can be executed independently. Each task string must be a standalone specification for a fresh agent: include its objective, necessary request context, boundaries, expected output, and success criteria. Avoid overlaps and hidden dependencies. Return only a JSON object with a tasks array of strings with exactly that length.\n\nRequest:\n" + args.request,
    executorProvider: args.executorProvider || "",
    modelProvider: args.modelProvider || "",
    model: args.model || "",
    permissions: "SKIP_PERMISSIONS",
    schema: taskPlanSchema,
  });
  if (plan.status !== "COMPLETED" || !plan.schemaValidated) {
    throw "spawn planner failed";
  }

  const tasks = plan.output && Array.isArray(plan.output.tasks) ? plan.output.tasks.slice() : [];
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
      permissions: "SKIP_PERMISSIONS",
      schema: taskResultSchema,
    });
  }
  const results = await parallel(specs);
  const findings = [];
  for (let index = 0; index < results.length; index += 1) {
    if (results[index].status !== "COMPLETED" || !results[index].schemaValidated) {
      throw "spawn task " + (index + 1) + " failed";
    }
    const result = results[index].output.result;
    if (typeof result !== "string" || result.trim() === "") {
      throw "spawn task " + (index + 1) + " returned an empty result";
    }
    findings.push({
      index: index + 1,
      task: tasks[index],
      result: result.trim(),
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
    permissions: "SKIP_PERMISSIONS",
    schema: mergerResultSchema,
  });
  if (merged.status !== "COMPLETED" || !merged.schemaValidated) {
    throw "spawn merger failed";
  }

  const mergedText = merged.output.answer.trim();
  if (!mergedText) {
    throw "spawn merger returned an empty result";
  }
  return mergedText;
})();
