/* @you-factory-meta
{
  "name": "@you/deep-research",
  "version": 1,
  "id": "builtin-deep-research",
  "description": {
    "type": "LOCALIZABLE_ASSET",
    "value": "Breaks a research question into bounded specialist investigations and synthesizes their findings."
  },
  "invocationSignature": {
    "parameters": [
      {
        "name": "topic",
        "description": "Research topic for the lead research workflow.",
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
        "name": "researchDepth",
        "description": "Investigation breadth from 1 (focused) through 3 (broad). Defaults to 2.",
        "externalName": "research-depth",
        "typeHint": "STRING",
        "defaultValue": "2",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "maxSubagents",
        "description": "Maximum specialist child agents for this invocation, from 0 through 2. Defaults to 2.",
        "externalName": "max-subagents",
        "typeHint": "STRING",
        "defaultValue": "2",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "modelProvider",
        "description": "Optional provider for specialist research execution; omitted values use operator defaults.",
        "externalName": "research-model-provider",
        "typeHint": "STRING",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "model",
        "description": "Optional model for specialist research execution; omitted values use operator defaults.",
        "externalName": "research-model",
        "typeHint": "STRING",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "reasoningEffort",
        "description": "Approved reasoning effort for specialist research execution. Defaults to medium.",
        "externalName": "reasoning-effort",
        "typeHint": "STRING",
        "defaultValue": "medium",
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
      "name": "positional-topic",
      "description": {
        "type": "LOCALIZABLE_ASSET",
        "value": "Compare two workflow architecture approaches."
      },
      "args": {
        "topic": "Compare event sourcing and state machines for workflow orchestration"
      }
    },
    {
      "name": "stdin-topic",
      "description": {
        "type": "LOCALIZABLE_ASSET",
        "value": "Research retrieval-augmented generation trade-offs."
      },
      "args": {
        "topic": "What are the trade-offs of retrieval-augmented generation?"
      }
    }
  ],
  "argsSchema": {
    "type": "object",
    "required": [
      "topic"
    ],
    "properties": {
      "topic": {
        "type": "string",
        "minLength": 1,
        "description": "The subject to investigate."
      },
      "researchDepth": {
        "type": "integer",
        "minimum": 1,
        "maximum": 3,
        "default": 2,
        "description": "Investigation breadth: 1 focused, 2 balanced, or 3 broad."
      },
      "maxSubagents": {
        "type": "integer",
        "minimum": 0,
        "maximum": 2,
        "default": 2,
        "description": "Per-invocation specialist cap within the factory policy ceiling."
      },
      "modelProvider": {
        "type": "string",
        "minLength": 1,
        "description": "Optional specialist execution provider."
      },
      "model": {
        "type": "string",
        "minLength": 1,
        "description": "Optional specialist execution model."
      },
      "reasoningEffort": {
        "type": "string",
        "enum": [
          "medium"
        ],
        "minLength": 1,
        "default": "medium",
        "description": "The requested specialist reasoning effort, subject to package policy."
      }
    },
    "additionalProperties": false
  },
  "defaultPolicy": {
    "mode": "READ_ONLY",
    "maxAgents": 3,
    "concurrency": 2,
    "maxDepth": 1,
    "maxRetries": 0,
    "allowNetwork": false,
    "allowConnectors": false,
    "allowDangerFullAccess": false,
    "writableRoots": [],
    "allowedReasoningEfforts": [
      "medium"
    ]
  }
}
*/

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
        prompt: "You are an independent technical research specialist with zero prior context. Investigate the mechanisms, terminology, primary evidence, implementation constraints, and disputed claims for the complete topic below at breadth level " + researchDepth + ". Distinguish verified facts from inference, explain source quality, and return a standalone evidence summary for a lead researcher.\n\nTopic:\n" + topic,
        modelProvider: modelProvider || "",
        model: model || "",
        reasoningEffort: reasoningEffort,
      },
      {
        label: "research-specialist-tradeoffs",
        prompt: "You are an independent practical research specialist with zero prior context. Investigate real-world trade-offs, alternatives, risks, failure modes, counterarguments, and decision criteria for the complete topic below at breadth level " + researchDepth + ". Distinguish verified facts from inference and return a standalone evidence summary for a lead researcher.\n\nTopic:\n" + topic,
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
    prompt: "You are the lead researcher with no shared conversation. Produce a complete answer to the topic below at breadth level " + researchDepth + ". Independently check the question's scope and terminology, integrate every relevant specialist finding, resolve or clearly preserve disagreements, distinguish evidence from inference, identify meaningful limitations, and make the response useful without access to these intermediate notes.\n\nTopic:\n" + topic +
      "\n\nCompleted specialist evidence:\n" + specialistEvidence,
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
