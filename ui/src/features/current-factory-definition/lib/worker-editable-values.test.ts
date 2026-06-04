import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
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
      stopToken: null,
      timeout: null,
      type: "MODEL_WORKER",
      workerName: "reviewer",
      workstationNames: ["Review", "Plan"],
    });
  });

  it("initializes timeout from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          timeout: "5m",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.timeout).toBe("5m");
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values)).toMatchObject({
      timeoutAmount: "5",
      timeoutUnit: "m",
    });
  });

  it("initializes stopToken from the selected worker value", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelProvider: "CURSOR",
          name: "reviewer",
          stopToken: "<COMPLETE>",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.stopToken).toBe("<COMPLETE>");
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values).stopToken).toBe("<COMPLETE>");
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

    const values = resolveEditableWorkerValues(factory, "reviewer");
    expect(values?.skipPermissions).toBe(true);
    if (!values) {
      throw new Error("expected reviewer values");
    }
    expect(editableWorkerDraftFromValues(values).skipPermissions).toBe(true);
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

describe("parseWorkerArgsText", () => {
  it("splits args on newlines and drops blank lines", () => {
    expect(parseWorkerArgsText(" --verbose \n\n--dry-run\n")).toEqual([
      "--verbose",
      "--dry-run",
    ]);
  });
});
