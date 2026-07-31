import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableWorkerDraft,
  EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS,
  EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
  editableWorkerDraftFromValues,
} from "./worker-editable-values";

function buildDraft(
  overrides: Partial<ReturnType<typeof editableWorkerDraftFromValues>> = {},
) {
  return {
    argsText: "",
    body: "",
    command: "",
    executorProvider: null,
    model: "",
    modelLocality: null,
    modelProvider: null,
    name: "worker",
    provider: null,
    skipPermissions: false,
    stopToken: "",
    timeoutAmount: "",
    timeoutUnit: "m" as const,
    type: "MODEL_WORKER" as const,
    ...EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS,
    ...overrides,
  };
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: applyEditableWorkerDraft coverage keeps model, script, and hosted regressions together.
describe("applyEditableWorkerDraft", () => {
  it("updates only the selected worker and preserves unrelated factory data", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      metadata: {
        description: "Factory metadata must survive worker edits.",
      },
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          modelProvider: "CODEX",
          name: "reviewer",
          stopToken: "STOP",
          type: "MODEL_WORKER",
        },
        {
          command: "echo",
          name: "runner",
          type: "SCRIPT_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review prompt",
          id: "review",
          name: "Review",
          worker: "reviewer",
        },
      ],
      workTypes: [{ name: "story", states: [] }],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "reviewer",
      buildDraft({
        model: "gpt-5.9",
        modelLocality: "CLOUD",
        modelProvider: "CODEX",
        name: "reviewer",
        stopToken: "STOP",
        type: "MODEL_WORKER",
      }),
    );

    expect(updatedFactory).toMatchObject({
      metadata: {
        description: "Factory metadata must survive worker edits.",
      },
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      workers: [
        {
          model: "gpt-5.9",
          modelLocality: "CLOUD",
          modelProvider: "CODEX",
          name: "reviewer",
          stopToken: "STOP",
          type: "MODEL_WORKER",
        },
        {
          command: "echo",
          name: "runner",
          type: "SCRIPT_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review prompt",
          id: "review",
          name: "Review",
          worker: "reviewer",
        },
      ],
      workTypes: [{ name: "story", states: [] }],
    });
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("body");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("command");
    expect(updatedFactory?.workers?.[1]).toBe(factory.workers?.[1]);
    expect(updatedFactory?.workTypes).toBe(factory.workTypes);
  });

  it("applies script worker fields without writing model provider overrides", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "reviewer",
      editableWorkerDraftFromValues({
        args: ["--verbose"],
        ...EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
        body: "Run the script worker body.",
        command: "node",
        executorProvider: null,
        model: null,
        modelLocality: null,
        modelProvider: null,
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "SCRIPT_WORKER",
        workerName: "reviewer",
        workstationNames: [],
      }),
    );

    expect(updatedFactory?.workers?.[0]).toEqual({
      args: ["--verbose"],
      body: "Run the script worker body.",
      command: "node",
      name: "reviewer",
      type: "SCRIPT_WORKER",
    });
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("model");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("modelProvider");
  });

  it("applies hosted Linear poller config and preserves unrelated worker-owned fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "linear-token" },
          linear: { pollInterval: "30s" },
          name: "linear-sync",
          resources: [{ capacity: 1, name: "linear-slot" }],
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "linear-sync",
      buildDraft({
        authSecretRef: "secrets/linear-api-key",
        linearClaimAssigneeField: "assignee.email",
        linearMappingState: "queued",
        linearMappingWorkType: "story",
        linearPollInterval: "45s",
        linearStateIdsText: "state-b",
        linearTeamIdsText: "team-a",
        name: "linear-sync",
        provider: "LINEAR",
        timeoutAmount: "1",
        timeoutUnit: "h",
        type: "HOSTED_WORKER",
      }),
    );

    expect(updatedFactory?.workers?.[0]).toEqual({
      auth: { secretRef: "secrets/linear-api-key" },
      linear: {
        claim: { assigneeField: "assignee.email" },
        mapping: { state: "queued", workType: "story" },
        pollInterval: "45s",
        stateIds: ["state-b"],
        teamIds: ["team-a"],
      },
      name: "linear-sync",
      provider: "LINEAR",
      resources: [{ capacity: 1, name: "linear-slot" }],
      timeout: "1h",
      type: "HOSTED_WORKER",
    });
  });

  it("removes linear.claim when the claim assignee field is cleared", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "secrets/linear-api-key" },
          linear: {
            claim: { assigneeField: "assignee.email" },
            mapping: { state: "queued", workType: "story" },
            pollInterval: "30s",
          },
          name: "linear-sync",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "linear-sync",
      buildDraft({
        authSecretRef: "secrets/linear-api-key",
        linearClaimAssigneeField: "",
        linearMappingState: "queued",
        linearMappingWorkType: "story",
        linearPollInterval: "30s",
        name: "linear-sync",
        provider: "LINEAR",
        type: "HOSTED_WORKER",
      }),
    );

    expect(updatedFactory?.workers?.[0]?.linear).toEqual({
      mapping: { state: "queued", workType: "story" },
      pollInterval: "30s",
    });
    expect(updatedFactory?.workers?.[0]?.linear).not.toHaveProperty("claim");
  });

  it("clears stale hosted Linear config when the worker is no longer a hosted Linear worker", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "secrets/linear-api-key" },
          linear: {
            mapping: { state: "queued", workType: "story" },
            pollInterval: "30s",
          },
          name: "linear-sync",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "linear-sync",
      buildDraft({
        argsText: "--verbose",
        command: "node",
        name: "linear-sync",
        type: "SCRIPT_WORKER",
      }),
    );

    expect(updatedFactory?.workers?.[0]).toEqual({
      args: ["--verbose"],
      command: "node",
      name: "linear-sync",
      type: "SCRIPT_WORKER",
    });
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("auth");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("linear");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("provider");
  });

  it("renames the selected worker and rewrites referencing workstation assignments", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
        {
          command: "echo",
          name: "runner",
          type: "SCRIPT_WORKER",
        },
      ],
      workstations: [
        { id: "review", name: "Review", worker: "reviewer" },
        { id: "plan", name: "Plan", worker: "reviewer" },
        { id: "run", name: "Run", worker: "runner" },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(
      factory,
      "reviewer",
      editableWorkerDraftFromValues({
        args: [],
        ...EMPTY_HOSTED_LINEAR_EDITABLE_VALUES,
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CODEX",
        provider: null,
        skipPermissions: null,
        stopToken: null,
        timeout: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review", "Plan"],
      }),
    );

    expect(updatedFactory?.workers?.[0]?.name).toBe("reviewer");
    expect(updatedFactory?.workstations).toEqual([
      { id: "review", name: "Review", worker: "reviewer" },
      { id: "plan", name: "Plan", worker: "reviewer" },
      { id: "run", name: "Run", worker: "runner" },
    ]);

    const renamedFactory = applyEditableWorkerDraft(
      factory,
      "reviewer",
      buildDraft({
        model: "gpt-5.5",
        modelProvider: "CODEX",
        name: "senior-reviewer",
        type: "MODEL_WORKER",
      }),
    );

    expect(renamedFactory?.workers?.[0]).toMatchObject({
      name: "senior-reviewer",
      modelProvider: "CODEX",
      type: "MODEL_WORKER",
    });
    expect(renamedFactory?.workstations).toEqual([
      { id: "review", name: "Review", worker: "senior-reviewer" },
      { id: "plan", name: "Plan", worker: "senior-reviewer" },
      { id: "run", name: "Run", worker: "runner" },
    ]);
  });
});
