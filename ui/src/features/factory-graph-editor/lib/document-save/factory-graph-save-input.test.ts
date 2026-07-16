// biome-ignore lint/style/noExcessiveLinesPerFile: save-input scenarios share one rich factory fixture.
import { describe, expect, it } from "vitest";
import { baseFactoryDefinition } from "../draft/factory-graph-draft.test-helpers";
import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
} from "../draft/factory-graph-draft-types";
import { createEmptyFactoryGraphDraft } from "../draft/factory-graph-draft-types";
import {
  addFactoryGraphNode,
  applyFactoryGraphPendingEdits,
  connectFactoryGraphNodes,
  disconnectFactoryGraphEdge,
} from "../operations/factory-graph-operations";

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
  layout: {
    schemaVersion: 1,
    nodes: [
      {
        id: "workstation:draft",
        position: { x: 120, y: 80 },
      },
    ],
    viewport: {
      x: 0,
      y: 0,
      zoom: 1,
    },
  },
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: save-input scenarios share one rich factory fixture.
describe("factory graph save input", () => {
  it("preserves unrelated canonical factory fields while mapping graph edges into save fields", () => {
    const draft = connect(
      connect(
        disconnect(
          connect(disconnectRoutesAndResources(), {
            sourceAnchorId: "workstation-output-source",
            sourceNodeId: "workstation:draft",
            targetAnchorId: "work-state-input-target",
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

    const saveInput = applyFactoryGraphPendingEdits({
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
      layout: richFactoryDefinition.layout,
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

  it("persists a newly added script worker into the save payload", () => {
    const withScriptWorker = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        argsText: " --watch \n--verbose\n",
        command: "  node  ",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "poller-runner",
        operations: [],
        workerType: "SCRIPT_WORKER",
      },
    });
    expect(withScriptWorker.ok).toBe(true);
    if (!withScriptWorker.ok) {
      return;
    }

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: withScriptWorker.value,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value.workers).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: "writer",
          name: "writer",
          type: "MODEL_WORKER",
        }),
        {
          args: ["--watch", "--verbose"],
          command: "node",
          id: "poller-runner",
          name: "poller-runner",
          type: "SCRIPT_WORKER",
        },
      ]),
    );
  });

  it("adds a script worker and poller workstation that save together", () => {
    const withScriptWorker = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: createEmptyFactoryGraphDraft(),
      node: {
        argsText: "",
        command: "node",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "poller-runner",
        operations: [],
        workerType: "SCRIPT_WORKER",
      },
    });
    expect(withScriptWorker.ok).toBe(true);
    if (!withScriptWorker.ok) {
      return;
    }

    const withPollerWorkstation = addFactoryGraphNode({
      baseFactoryDefinition,
      draft: withScriptWorker.value,
      node: {
        behavior: "POLLER",
        body: "",
        kind: "workstation",
        name: "linear-poller",
        workerName: "poller-runner",
      },
    });
    expect(withPollerWorkstation.ok).toBe(true);
    if (!withPollerWorkstation.ok) {
      return;
    }

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition,
      draft: withPollerWorkstation.value,
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(saveInput.value.workers).toEqual(
      expect.arrayContaining([
        {
          command: "node",
          id: "poller-runner",
          name: "poller-runner",
          type: "SCRIPT_WORKER",
        },
      ]),
    );
    expect(saveInput.value.workstations).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          behavior: "POLLER",
          id: "linear-poller",
          name: "linear-poller",
          worker: "poller-runner",
        }),
      ]),
    );
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

    const saveInput = applyFactoryGraphPendingEdits({
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

  it("removes internal system-time topology from save input", () => {
    const factoryWithSystemTime: CanonicalFactoryDefinition = {
      ...richFactoryDefinition,
      layout: {
        schemaVersion: 1,
        edges: [
          {
            id: "workstation-output:workstation:draft->work-state:__system_time:pending",
            waypoints: [{ x: 10, y: 20 }],
          },
        ],
        nodes: [
          { id: "workstation:draft", position: { x: 120, y: 80 } },
          {
            id: "work-type:__system_time",
            position: { x: 260, y: 80 },
          },
          {
            id: "work-state:__system_time:pending",
            position: { x: 420, y: 80 },
          },
          {
            id: "workstation:__system_time:expire",
            position: { x: 580, y: 80 },
          },
        ],
      },
      workTypes: [
        ...(richFactoryDefinition.workTypes ?? []),
        {
          name: "__system_time",
          states: [{ name: "pending", type: "PROCESSING" }],
        },
      ],
      workstations: [
        {
          ...(richFactoryDefinition.workstations?.[0] ?? {
            inputs: [],
            name: "draft",
          }),
          outputs: [
            ...(richFactoryDefinition.workstations?.[0]?.outputs ?? []),
            { state: "pending", workType: "__system_time" },
          ],
        },
        {
          inputs: [{ state: "pending", workType: "__system_time" }],
          name: "__system_time:expire",
          outputs: [],
          type: "MODEL_WORKSTATION",
          worker: "",
        },
      ],
    };

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: factoryWithSystemTime,
      draft: createEmptyFactoryGraphDraft(),
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(JSON.stringify(saveInput.value)).not.toContain("__system_time");
    expect(
      saveInput.value.workTypes?.map((workType) => workType.name),
    ).not.toContain("__system_time");
    expect(
      saveInput.value.workstations?.map((workstation) => workstation.name),
    ).not.toContain("__system_time:expire");
    expect(saveInput.value.workstations?.[0]?.outputs).toEqual([
      { state: "drafted", workType: "story" },
    ]);
  });

  it("removes generated resource availability topology from the captured save payload", () => {
    const capturedSaveRequest = {
      mode: "REPLACE_CURRENT",
      factory: {
        layout: {
          nodes: [
            {
              id: "work-type:executor-slot",
              position: { x: 0, y: 80 },
            },
            {
              id: "work-state:executor-slot:available",
              position: { x: 120, y: 80 },
            },
          ],
          schemaVersion: 1,
        },
        name: "UNDEFINED",
        resources: [
          { capacity: 10, id: "executor-slot", name: "executor-slot" },
        ],
        workers: [
          {
            id: "processor",
            name: "processor",
            type: "MODEL_WORKER",
          },
        ],
        workTypes: [
          {
            id: "cron-triggers",
            name: "cron-triggers",
            states: [
              { id: "complete", name: "complete", type: "TERMINAL" },
              { id: "failed", name: "failed", type: "FAILED" },
            ],
          },
          {
            id: "task",
            name: "task",
            states: [
              { id: "init", name: "init", type: "INITIAL" },
              { id: "in-review", name: "in-review", type: "PROCESSING" },
            ],
          },
          {
            id: "executor-slot",
            name: "executor-slot",
            states: [{ id: "available", name: "available", type: "INITIAL" }],
          },
        ],
        workstations: [
          {
            behavior: "CRON",
            id: "cleaner",
            inputs: [{ state: "available", workType: "executor-slot" }],
            name: "cleaner",
            outputs: [
              { state: "complete", workType: "cron-triggers" },
              { state: "available", workType: "executor-slot" },
            ],
            type: "MODEL_WORKSTATION",
            worker: "processor",
          },
          {
            behavior: "REPEATER",
            id: "process",
            inputs: [
              { state: "init", workType: "task" },
              { state: "available", workType: "executor-slot" },
            ],
            name: "process",
            outputs: [
              { state: "in-review", workType: "task" },
              { state: "available", workType: "executor-slot" },
            ],
            type: "MODEL_WORKSTATION",
            worker: "processor",
          },
        ],
      } satisfies CanonicalFactoryDefinition,
    };

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: capturedSaveRequest.factory,
      draft: createEmptyFactoryGraphDraft(),
    });

    expect(saveInput.ok).toBe(true);
    if (!saveInput.ok) {
      return;
    }

    expect(JSON.stringify(saveInput.value)).not.toContain(
      '"workType":"executor-slot"',
    );
    expect(
      saveInput.value.resources?.map((resource) => resource.name),
    ).toContain("executor-slot");
    expect(saveInput.value.workTypes?.map((workType) => workType.name)).toEqual(
      ["cron-triggers", "task"],
    );
    expect(saveInput.value.layout?.nodes?.map((node) => node.id)).not.toEqual(
      expect.arrayContaining([
        "work-type:executor-slot",
        "work-state:executor-slot:available",
      ]),
    );
    expect(
      saveInput.value.workstations?.find(
        (workstation) => workstation.name === "cleaner",
      ),
    ).toMatchObject({
      inputs: [],
      outputs: [{ state: "complete", workType: "cron-triggers" }],
    });
    expect(
      saveInput.value.workstations?.find(
        (workstation) => workstation.name === "process",
      ),
    ).toMatchObject({
      inputs: [{ state: "init", workType: "task" }],
      outputs: [{ state: "in-review", workType: "task" }],
    });
  });

  it("rejects non-resource workstation routes that reference unknown work states", () => {
    const factoryWithUnknownRoute: CanonicalFactoryDefinition = {
      ...richFactoryDefinition,
      workstations: [
        {
          ...(richFactoryDefinition.workstations?.[0] ?? {
            inputs: [],
            name: "draft",
            worker: "writer",
          }),
          inputs: [
            ...(richFactoryDefinition.workstations?.[0]?.inputs ?? []),
            { state: "queued", workType: "missing-work-type" },
          ],
        },
      ],
    };

    const saveInput = applyFactoryGraphPendingEdits({
      baseFactoryDefinition: factoryWithUnknownRoute,
      draft: createEmptyFactoryGraphDraft(),
    });

    expect(saveInput).toMatchObject({
      ok: false,
      reason: "INVALID_SAVE",
      validationErrors: expect.arrayContaining([
        expect.objectContaining({
          code: "INVALID_WORKSTATION_ROUTE",
          message:
            'Workstation "draft" inputs references unknown work state "missing-work-type:queued".',
          target: {
            id: "workstation-input:work-state:missing-work-type:queued->workstation:draft",
            kind: "edge",
          },
        }),
      ]),
    });
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
