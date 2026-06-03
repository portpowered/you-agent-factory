import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableWorkerDraft,
  editableWorkerDraftFromValues,
  parseWorkerArgsText,
  resolveEditableWorkerValues,
} from "./worker-editable-values";

describe("resolveEditableWorkerValues", () => {
  it("joins the selected worker with referencing workstation names", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        { id: "review", name: "Review", worker: "reviewer" },
        { id: "plan", name: "Plan", worker: "reviewer" },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkerValues(factory, "reviewer")).toEqual({
      args: [],
      body: null,
      command: null,
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      provider: null,
      skipPermissions: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames: ["Review", "Plan"],
    });
  });

  it("initializes skipPermissions from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          skipPermissions: true,
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkerValues(factory, "reviewer")?.skipPermissions).toBe(
      true,
    );
    expect(
      editableWorkerDraftFromValues(
        resolveEditableWorkerValues(factory, "reviewer")!,
      ).skipPermissions,
    ).toBe(true);
  });

  it("returns null when the worker is missing from the factory document", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workTypes: [],
    };

    expect(resolveEditableWorkerValues(factory, "missing-worker")).toBeNull();
  });
});

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
          modelProvider: "CURSOR",
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

    const updatedFactory = applyEditableWorkerDraft(factory, "reviewer", {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.9",
      modelLocality: "CLOUD",
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions: false,
      type: "MODEL_WORKER",
    });

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

  it("writes skipPermissions when enabled and clears it when disabled", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const enabledFactory = applyEditableWorkerDraft(factory, "reviewer", {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CURSOR",
      name: "reviewer",
      provider: null,
      skipPermissions: true,
      type: "MODEL_WORKER",
    });

    expect(enabledFactory?.workers?.[0]?.skipPermissions).toBe(true);

    const disabledFactory = applyEditableWorkerDraft(
      {
        ...factory,
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CURSOR",
            name: "reviewer",
            skipPermissions: true,
            type: "MODEL_WORKER",
          },
        ],
      },
      "reviewer",
      {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        type: "MODEL_WORKER",
      },
    );

    expect(disabledFactory?.workers?.[0]).not.toHaveProperty("skipPermissions");
  });

  it("applies script worker fields without writing model provider overrides", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          modelProvider: "CURSOR",
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
        body: "Run the script worker body.",
        command: "node",
        executorProvider: null,
        model: null,
        modelLocality: null,
        modelProvider: null,
        provider: null,
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

  it("applies hosted worker provider without exposing workstation-owned fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "linear-token" },
          linear: { pollInterval: "30s" },
          name: "linear-sync",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(factory, "linear-sync", {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "",
      modelLocality: null,
      modelProvider: null,
      name: "linear-sync",
      provider: "LINEAR",
      type: "HOSTED_WORKER",
    });

    expect(updatedFactory?.workers?.[0]).toEqual({
      auth: { secretRef: "linear-token" },
      linear: { pollInterval: "30s" },
      name: "linear-sync",
      provider: "LINEAR",
      type: "HOSTED_WORKER",
    });
  });

  it("renames the selected worker and rewrites referencing workstation assignments", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
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
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        provider: null,
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

    const renamedFactory = applyEditableWorkerDraft(factory, "reviewer", {
      ...editableWorkerDraftFromValues({
        args: [],
        body: null,
        command: null,
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CURSOR",
        provider: null,
        type: "MODEL_WORKER",
        workerName: "reviewer",
        workstationNames: ["Review", "Plan"],
      }),
      name: "senior-reviewer",
    });

    expect(renamedFactory?.workers?.[0]).toMatchObject({
      name: "senior-reviewer",
      modelProvider: "CURSOR",
      type: "MODEL_WORKER",
    });
    expect(renamedFactory?.workstations).toEqual([
      { id: "review", name: "Review", worker: "senior-reviewer" },
      { id: "plan", name: "Plan", worker: "senior-reviewer" },
      { id: "run", name: "Run", worker: "runner" },
    ]);
  });
});

describe("parseWorkerArgsText", () => {
  it("splits args on newlines and drops blank lines", () => {
    expect(parseWorkerArgsText(" --verbose \n\n--dry-run\n")).toEqual([
      "--verbose",
      "--dry-run",
    ]);
  });
});
