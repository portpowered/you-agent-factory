import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { applyEditableWorkerDraft } from "./worker-editable-values";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: runtime apply coverage groups timeout, skipPermissions, stopToken, and type-change regressions.
describe("applyEditableWorkerDraft runtime fields", () => {
  it("writes timeout when configured and clears it when empty", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          command: "node",
          name: "runner",
          timeout: "30s",
          type: "SCRIPT_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(factory, "runner", {
      argsText: "",
      body: "",
      command: "node",
      executorProvider: null,
      model: "",
      modelLocality: null,
      modelProvider: null,
      name: "runner",
      provider: null,
      skipPermissions: false,
      stopToken: "",
      timeoutAmount: "5",
      timeoutUnit: "m",
      type: "SCRIPT_WORKER",
    });

    expect(updatedFactory?.workers?.[0]?.timeout).toBe("5m");

    if (!updatedFactory) {
      throw new Error("expected updated factory");
    }

    const clearedFactory = applyEditableWorkerDraft(updatedFactory, "runner", {
      argsText: "",
      body: "",
      command: "node",
      executorProvider: null,
      model: "",
      modelLocality: null,
      modelProvider: null,
      name: "runner",
      provider: null,
      skipPermissions: false,
      stopToken: "",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "SCRIPT_WORKER",
    });

    expect(clearedFactory?.workers?.[0]).not.toHaveProperty("timeout");
  });

  it("writes skipPermissions when enabled and clears it when disabled", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
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
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions: true,
      stopToken: "",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "MODEL_WORKER",
    });

    expect(enabledFactory?.workers?.[0]?.skipPermissions).toBe(true);

    const disabledFactory = applyEditableWorkerDraft(
      {
        ...factory,
        workers: [
          {
            model: "gpt-5.5",
            modelProvider: "CODEX",
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
        modelProvider: "CODEX",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
    );

    expect(disabledFactory?.workers?.[0]).not.toHaveProperty("skipPermissions");
  });

  it("writes stopToken when set and clears it when empty or whitespace-only", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          id: "review",
          name: "Review",
          stopWords: ["DONE"],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(factory, "reviewer", {
      argsText: "",
      body: "",
      command: "",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions: false,
      stopToken: "<COMPLETE>",
      timeoutAmount: "",
      timeoutUnit: "m",
      type: "MODEL_WORKER",
    });

    expect(updatedFactory?.workers?.[0]?.stopToken).toBe("<COMPLETE>");
    expect(updatedFactory?.workstations?.[0]?.stopWords).toEqual(["DONE"]);

    if (!updatedFactory) {
      throw new Error("expected updated factory");
    }

    const clearedFactory = applyEditableWorkerDraft(
      updatedFactory,
      "reviewer",
      {
        argsText: "",
        body: "",
        command: "",
        executorProvider: null,
        model: "gpt-5.5",
        modelLocality: null,
        modelProvider: "CODEX",
        name: "reviewer",
        provider: null,
        skipPermissions: false,
        stopToken: "   ",
        timeoutAmount: "",
        timeoutUnit: "m",
        type: "MODEL_WORKER",
      },
    );

    expect(clearedFactory?.workers?.[0]).not.toHaveProperty("stopToken");
    expect(clearedFactory?.workstations?.[0]?.stopWords).toEqual(["DONE"]);
  });

  it("preserves runtime fields when saving a model worker type change to script worker", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CODEX",
          name: "reviewer",
          skipPermissions: true,
          stopToken: "<COMPLETE>",
          timeout: "30m",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(factory, "reviewer", {
      argsText: "--verbose",
      body: "",
      command: "node",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: null,
      modelProvider: "CODEX",
      name: "reviewer",
      provider: null,
      skipPermissions: true,
      stopToken: "<COMPLETE>",
      timeoutAmount: "30",
      timeoutUnit: "m",
      type: "SCRIPT_WORKER",
    });

    expect(updatedFactory?.workers?.[0]).toEqual({
      args: ["--verbose"],
      command: "node",
      name: "reviewer",
      skipPermissions: true,
      stopToken: "<COMPLETE>",
      timeout: "30m",
      type: "SCRIPT_WORKER",
    });
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("model");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("modelProvider");
  });

  it("preserves runtime fields when saving a script worker type change to model worker", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          command: "node",
          name: "runner",
          stopToken: "STOP",
          timeout: "1h",
          type: "SCRIPT_WORKER",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkerDraft(factory, "runner", {
      argsText: "",
      body: "",
      command: "node",
      executorProvider: null,
      model: "gpt-5.5",
      modelLocality: "CLOUD",
      modelProvider: "CODEX",
      name: "runner",
      provider: null,
      skipPermissions: true,
      stopToken: "STOP",
      timeoutAmount: "1",
      timeoutUnit: "h",
      type: "MODEL_WORKER",
    });

    expect(updatedFactory?.workers?.[0]).toEqual({
      model: "gpt-5.5",
      modelLocality: "CLOUD",
      modelProvider: "CODEX",
      name: "runner",
      skipPermissions: true,
      stopToken: "STOP",
      timeout: "1h",
      type: "MODEL_WORKER",
    });
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("command");
    expect(updatedFactory?.workers?.[0]).not.toHaveProperty("args");
  });
});
