/* @you-factory-meta
{
  "name": "@you/tournament",
  "version": 1,
  "id": "builtin-tournament",
  "description": {
    "type": "LOCALIZABLE_ASSET",
    "value": "Runs candidates through bounded 1v1 matches, uses a judge to advance each winner, and returns the champion result."
  },
  "invocationSignature": {
    "parameters": [
      {
        "name": "request",
        "description": "Request that every tournament candidate attempts.",
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
        "name": "rounds",
        "description": "Single-elimination bracket depth from 1 through 3.",
        "externalName": "rounds",
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
        "description": "Optional executor provider used by competitor agents.",
        "externalName": "competitor-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "modelProvider",
        "description": "Optional model provider used by competitor agents.",
        "externalName": "competitor-model-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "model",
        "description": "Optional model used by competitor agents.",
        "externalName": "competitor-model",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "judgeExecutorProvider",
        "description": "Optional executor provider used by match judges.",
        "externalName": "judge-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "judgeModelProvider",
        "description": "Optional model provider used by match judges.",
        "externalName": "judge-model-provider",
        "bindings": [
          {
            "kind": "NAMED"
          }
        ]
      },
      {
        "name": "judgeModel",
        "description": "Optional model used by match judges.",
        "externalName": "judge-model",
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
      "name": "two-round-tournament",
      "description": {
        "type": "LOCALIZABLE_ASSET",
        "value": "Compare four candidate launch strategies through two judged rounds."
      },
      "args": {
        "request": "Propose a launch strategy",
        "rounds": "2"
      }
    }
  ],
  "argsSchema": {
    "type": "object",
    "required": [
      "request",
      "rounds"
    ],
    "properties": {
      "request": {
        "type": "string",
        "minLength": 1
      },
      "rounds": {
        "type": "integer",
        "minimum": 1,
        "maximum": 3
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
      },
      "judgeExecutorProvider": {
        "type": "string",
        "minLength": 1
      },
      "judgeModelProvider": {
        "type": "string",
        "minLength": 1
      },
      "judgeModel": {
        "type": "string",
        "minLength": 1
      }
    },
    "additionalProperties": false
  },
  "defaultPolicy": {
    "mode": "READ_ONLY",
    "maxAgents": 15,
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
  const entrantCount = Math.pow(2, args.rounds);
  const requiredCalls = (entrantCount * 2) - 1;
  const budget = workflow.budget();
  if (requiredCalls > budget.maxAgents) {
    throw "tournament requires " + requiredCalls + " agent calls but maxAgents is " + budget.maxAgents;
  }
  phase("candidate-generation");

  const competitors = [];
  for (let index = 0; index < entrantCount; index += 1) {
    competitors.push({
      label: "tournament-competitor-" + (index + 1),
      prompt: "You are competitor " + (index + 1) + " of " + entrantCount + " in a blind tournament and have zero context from other entrants. Read the complete request, independently investigate any available evidence or workspace context, address every requirement and important edge case, and produce your strongest self-contained answer with appropriate verification. Return only the candidate answer; do not discuss the tournament.\n\nRequest:\n" + args.request,
      executorProvider: args.executorProvider || "",
      modelProvider: args.modelProvider || "",
      model: args.model || "",
    });
  }

  const generated = await parallel(competitors);
  let bracket = [];
  for (let index = 0; index < generated.length; index += 1) {
    if (generated[index].status !== "COMPLETED") {
      throw "tournament competitor " + (index + 1) + " failed";
    }
    bracket.push({
      entrant: index + 1,
      answer: generated[index].output.text,
      rationale: [],
    });
  }

  for (let round = 1; round <= args.rounds; round += 1) {
    phase("round-" + round);
    const matches = [];
    for (let match = 0; match < bracket.length / 2; match += 1) {
      const candidateA = bracket[match * 2];
      const candidateB = bracket[match * 2 + 1];
      matches.push({
        label: "tournament-judge-r" + round + "-m" + (match + 1),
        prompt: "You are an independent judge with zero prior context. Evaluate this 1v1 match against the complete original request. Compare correctness, requirement coverage, evidence, reasoning quality, handling of edge and failure cases, usefulness, and unsupported claims. Select exactly candidate A or B; do not combine them or invent a replacement answer. The rationale must identify the decisive evidence and any material weakness in the winner. Return only JSON shaped {\"winner\":\"A\"|\"B\",\"rationale\":\"...\"}.\n\nRequest:\n" + args.request +
          "\n\nCandidate A:\n" + candidateA.answer + "\n\nCandidate B:\n" + candidateB.answer,
        executorProvider: args.judgeExecutorProvider || args.executorProvider || "",
        modelProvider: args.judgeModelProvider || args.modelProvider || "",
        model: args.judgeModel || args.model || "",
      });
    }

    const judgments = await parallel(matches);
    const advanced = [];
    for (let match = 0; match < judgments.length; match += 1) {
      if (judgments[match].status !== "COMPLETED") {
        throw "tournament judge failed at round " + round + " match " + (match + 1);
      }
      let decision;
      try {
        const judgmentText = judgments[match].output.text.trim();
        const judgmentStart = judgmentText.indexOf("{");
        const judgmentEnd = judgmentText.lastIndexOf("}");
        if (judgmentStart < 0 || judgmentEnd < judgmentStart) {
          throw "missing judgment object";
        }
        decision = JSON.parse(judgmentText.slice(judgmentStart, judgmentEnd + 1));
      } catch (_) {
        throw "tournament judge returned invalid JSON at round " + round + " match " + (match + 1);
      }
      if (decision.winner !== "A" && decision.winner !== "B") {
        throw "tournament judge must select A or B at round " + round + " match " + (match + 1);
      }
      const winner = decision.winner === "A" ? bracket[match * 2] : bracket[match * 2 + 1];
      winner.rationale.push({
        round: round,
        match: match + 1,
        selected: decision.winner,
        rationale: decision.rationale || "",
      });
      advanced.push(winner);
    }
    bracket = advanced;
  }

  return {
    request: args.request,
    rounds: args.rounds,
    entrantCount: entrantCount,
    champion: bracket[0],
  };
})();
