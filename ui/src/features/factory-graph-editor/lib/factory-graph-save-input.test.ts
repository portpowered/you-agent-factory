import { describe, expect, it } from "vitest";

import {
  buildFactoryGraphSaveInput,
  connectFactoryGraphNodes,
  createEmptyFactoryGraphDraft,
  disconnectFactoryGraphEdge,
} from "../public";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "./factory-graph-draft-types";

const richFactoryDefinition: CanonicalFactoryDefinition = {
  guards: [
    {
      modelProvider: "CODEX",
      refreshWindow: "10m",
      type: "INFERENCE_THROTTLE",
    },
  ],
  inputTypes: [
    {
      name: "json-input",
      type: "DEFAULT",
    },
  ],
  metadata: {
    owner: "operations",
  },
  name: "Current Factory",
  resources: [
    {
      capacity: 4,
      model: "gpt-5",
      name: "model-slot",
      provider: "codex",
      type: "INVOCATION_SLOT",
    },
    {
      capacity: 2,
      name: "review-slot",
    },
  ],
  runner: "codex",
  supportingFiles: {
    requiredTools: [
      {
        command: "jq",
        name: "jq",
      },
    ],
  },
  workers: [
    {
      model: "gpt-5",
      modelProvider: "CODEX",
      name: "writer",
      resources: [
        {
          capacity: 1,
          name: "model-slot",
        },
      ],
      type: "MODEL_WORKER",
    },
    {
      command: "node",
      name: "reviewer",
      resources: [
        {
          capacity: 1,
          name: "review-slot",
        },
      ],
      type: "SCRIPT_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
        {
          name: "drafted",
          type: "PROCESSING",
        },
        {
          name: "needs-review",
          type: "PROCESSING",
        },
        {
          name: "rejected",
          type: "FAILED",
        },
        {
          name: "done",
          type: "TERMINAL",
        },
      ],
    },
  ],
  workstations: [
    {
      body: "Draft the story.",
      env: {
        MODE: "draft",
      },
      inputs: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      limits: {
        maxExecutionTime: "5m",
        maxRetries: 2,
      },
      name: "draft",
      onContinue: [
        {
          state: "queued",
          workType: "story",
        },
      ],
      onFailure: [
        {
          state: "rejected",
          workType: "story",
        },
      ],
      onRejection: [
        {
          state: "rejected",
          workType: "story",
        },
      ],
      outputs: [
        {
          state: "drafted",
          workType: "story",
        },
      ],
      promptFile: "prompts/draft.md",
      resources: [
        {
          capacity: 1,
          name: "model-slot",
        },
      ],
      runner: "codex",
      stopWords: ["DONE"],
      type: "MODEL_WORKSTATION",
      worker: "writer",
      workingDirectory: "{{work.id}}",
      worktree: "{{work.id}}",
    },
  ],
};

describe("factory graph save input", () => {
  it("preserves unrelated canonical factory fields while mapping graph edges into save fields", () => {
    const draft = connect(
      connect(
        disconnect(
          connect(disconnectRoutesAndResources(), {
            sourceAnchorId: "workstation-output-source",
            sourceNodeId: "workstation:draft",
            targetAnchorId: "workstation-output-target",
            targetNodeId: "work-state:story:done",
          }),
          ["worker-assignment:worker:writer->workstation:draft"],
        ),
        {
          sourceAnchorId: "worker-assignment-source",
          sourceNodeId: "worker:reviewer",
          targetAnchorId: "worker-assignment-target",
          targetNodeId: "workstation:draft",
        },
      ),
      {
        sourceAnchorId: "workstation-resource-source",
        sourceNodeId: "resource:review-slot",
        targetAnchorId: "workstation-resource-target",
        targetNodeId: "workstation:draft",
      },
    );

    const saveInput = buildFactoryGraphSaveInput({
      baseFactoryDefinition: richFactoryDefinition,
      draft,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value).toMatchObject({
      guards: richFactoryDefinition.guards,
      inputTypes: richFactoryDefinition.inputTypes,
      metadata: richFactoryDefinition.metadata,
      runner: richFactoryDefinition.runner,
      supportingFiles: richFactoryDefinition.supportingFiles,
    });
    const savedWriter = saveInput.value.workers?.find(
      (worker) => worker.name === "writer",
    );
    expect(savedWriter).toMatchObject({
      model: "gpt-5",
      modelProvider: "CODEX",
      type: "MODEL_WORKER",
    });
    expect(savedWriter?.resources).toBeUndefined();
    expect(saveInput.value.workstations?.[0]).toMatchObject({
      body: "Draft the story.",
      env: {
        MODE: "draft",
      },
      limits: {
        maxExecutionTime: "5m",
        maxRetries: 2,
      },
      name: "draft",
      outputs: [
        {
          state: "done",
          workType: "story",
        },
      ],
      promptFile: "prompts/draft.md",
      resources: [
        {
          capacity: 1,
          name: "review-slot",
        },
      ],
      runner: "codex",
      stopWords: ["DONE"],
      type: "MODEL_WORKSTATION",
      worker: "reviewer",
      workingDirectory: "{{work.id}}",
      worktree: "{{work.id}}",
    });
    expect(saveInput.value.workstations?.[0]?.onContinue).toBeUndefined();
    expect(saveInput.value.workstations?.[0]?.onRejection).toBeUndefined();
    expect(saveInput.value.workstations?.[0]?.onFailure).toBeUndefined();
  });

  it("rejects invalid save input without mutating the base factory definition", () => {
    const invalidDraft: FactoryGraphDraft = {
      ...createEmptyFactoryGraphDraft(),
      additions: {
        ...createEmptyFactoryGraphDraft().additions,
        workstations: [
          {
            inputs: [],
            name: "orphan",
            worker: "",
          },
        ],
      },
    };

    const saveInput = buildFactoryGraphSaveInput({
      baseFactoryDefinition: richFactoryDefinition,
      draft: invalidDraft,
    });

    expect(saveInput).toMatchObject({
      ok: false,
      reason: "INVALID_SAVE",
      validationErrors: [
        expect.objectContaining({
          code: "MISSING_REQUIRED_FIELD",
        }),
      ],
    });
    expect(richFactoryDefinition.workstations?.[0]?.outputs).toEqual([
      {
        state: "drafted",
        workType: "story",
      },
    ]);
  });
});

function disconnectRoutesAndResources(): FactoryGraphDraft {
  return disconnect(createEmptyFactoryGraphDraft(), [
    "workstation-output:workstation:draft->work-state:story:drafted",
    "workstation-on-continue:workstation:draft->work-state:story:queued",
    "workstation-on-rejection:workstation:draft->work-state:story:rejected",
    "workstation-on-failure:workstation:draft->work-state:story:rejected",
    "worker-resource:resource:model-slot->worker:writer",
    "workstation-resource:resource:model-slot->workstation:draft",
  ]);
}

function connect(
  draft: FactoryGraphDraft,
  connection: Parameters<typeof connectFactoryGraphNodes>[0] extends {
    sourceAnchorId: infer SourceAnchorId;
    sourceNodeId: infer SourceNodeId;
    targetAnchorId: infer TargetAnchorId;
    targetNodeId: infer TargetNodeId;
  }
    ? {
        sourceAnchorId: SourceAnchorId;
        sourceNodeId: SourceNodeId;
        targetAnchorId: TargetAnchorId;
        targetNodeId: TargetNodeId;
      }
    : never,
): FactoryGraphDraft {
  const result = connectFactoryGraphNodes({
    baseFactoryDefinition: richFactoryDefinition,
    draft,
    ...connection,
  });
  expect(result.ok).toBe(true);
  return result.ok ? result.value : draft;
}

function disconnect(
  draft: FactoryGraphDraft,
  edgeIds: string[],
): FactoryGraphDraft {
  return edgeIds.reduce((currentDraft, edgeId) => {
    const result = disconnectFactoryGraphEdge({
      baseFactoryDefinition: richFactoryDefinition,
      draft: currentDraft,
      edgeId,
    });
    expect(result.ok).toBe(true);
    return result.ok ? result.value : currentDraft;
  }, draft);
}
