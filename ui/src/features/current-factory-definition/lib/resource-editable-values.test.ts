import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  applyEditableResourceDraft,
  editableResourceDraftFromValues,
  parseResourceCapacityText,
  resolveEditableResourceValues,
} from "./resource-editable-values";

describe("resolveEditableResourceValues", () => {
  it("joins the selected resource with referencing worker and workstation names", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      resources: [
        {
          capacity: 2,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
      ],
      workers: [
        {
          name: "reviewer",
          resources: [{ capacity: 1, name: "agent-slot" }],
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        { id: "review", name: "Review", worker: "reviewer" },
        {
          id: "plan",
          name: "Plan",
          resources: [{ capacity: 1, name: "agent-slot" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableResourceValues(factory, "agent-slot")).toEqual({
      backend: null,
      capacity: 2,
      loadPolicy: null,
      model: null,
      provider: null,
      resourceName: "agent-slot",
      type: "INVOCATION_SLOT",
      workerNames: ["reviewer"],
      workstationNames: ["Plan"],
    });
  });

  it("returns null when the resource is missing from the factory document", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      resources: [],
      workTypes: [],
    };

    expect(
      resolveEditableResourceValues(factory, "missing-resource"),
    ).toBeNull();
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: applyEditableResourceDraft coverage keeps model, quota, and rename regressions together.
describe("applyEditableResourceDraft", () => {
  it("updates only the selected resource and preserves unrelated factory data", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      metadata: {
        description: "Factory metadata must survive resource edits.",
      },
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      resources: [
        {
          backend: "llama.cpp",
          capacity: 1,
          loadPolicy: "ON_DEMAND",
          model: "OMNIVOICE_Q4_K_M",
          name: "voice-model",
          type: "MODEL",
        },
        {
          capacity: 2,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
      ],
      workTypes: [{ name: "story", states: [] }],
    };

    const updatedFactory = applyEditableResourceDraft(factory, "agent-slot", {
      backend: "",
      capacityText: "4",
      loadPolicy: "",
      model: "",
      name: "agent-slot",
      provider: "",
      type: "INVOCATION_SLOT",
    });

    expect(updatedFactory).toMatchObject({
      metadata: {
        description: "Factory metadata must survive resource edits.",
      },
      version: {
        logical: "7",
        physical: "2026-05-23T15:52:00Z",
      },
      resources: [
        {
          backend: "llama.cpp",
          capacity: 1,
          loadPolicy: "ON_DEMAND",
          model: "OMNIVOICE_Q4_K_M",
          name: "voice-model",
          type: "MODEL",
        },
        {
          capacity: 4,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
      ],
      workTypes: [{ name: "story", states: [] }],
    });
    expect(updatedFactory?.resources?.[0]).toBe(factory.resources?.[0]);
    expect(updatedFactory?.workTypes).toBe(factory.workTypes);
  });

  it("applies model resource fields without writing provider quota overrides", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      resources: [
        {
          capacity: 1,
          name: "voice-model",
          provider: "anthropic",
          type: "MODEL",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableResourceDraft(
      factory,
      "voice-model",
      editableResourceDraftFromValues({
        backend: "llama.cpp",
        capacity: 1,
        loadPolicy: "ON_DEMAND",
        model: "OMNIVOICE_Q4_K_M",
        provider: null,
        resourceName: "voice-model",
        type: "MODEL",
        workerNames: [],
        workstationNames: [],
      }),
    );

    expect(updatedFactory?.resources?.[0]).toEqual({
      backend: "llama.cpp",
      capacity: 1,
      loadPolicy: "ON_DEMAND",
      model: "OMNIVOICE_Q4_K_M",
      name: "voice-model",
      type: "MODEL",
    });
    expect(updatedFactory?.resources?.[0]).not.toHaveProperty("provider");
  });

  it("applies provider quota fields without exposing model-only fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      resources: [
        {
          capacity: 5,
          model: "gpt-5",
          name: "anthropic-quota",
          type: "PROVIDER_QUOTA",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableResourceDraft(
      factory,
      "anthropic-quota",
      {
        backend: "",
        capacityText: "5",
        loadPolicy: "",
        model: "",
        name: "anthropic-quota",
        provider: "anthropic",
        type: "PROVIDER_QUOTA",
      },
    );

    expect(updatedFactory?.resources?.[0]).toEqual({
      capacity: 5,
      name: "anthropic-quota",
      provider: "anthropic",
      type: "PROVIDER_QUOTA",
    });
    expect(updatedFactory?.resources?.[0]).not.toHaveProperty("model");
  });

  it("renames the selected resource and rewrites referencing worker and workstation requirements", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      resources: [
        {
          capacity: 2,
          name: "agent-slot",
          type: "INVOCATION_SLOT",
        },
      ],
      workers: [
        {
          name: "reviewer",
          resources: [{ capacity: 1, name: "agent-slot" }],
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          id: "review",
          name: "Review",
          resources: [{ capacity: 1, name: "agent-slot" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    const renamedFactory = applyEditableResourceDraft(factory, "agent-slot", {
      ...editableResourceDraftFromValues({
        backend: null,
        capacity: 2,
        loadPolicy: null,
        model: null,
        provider: null,
        resourceName: "agent-slot",
        type: "INVOCATION_SLOT",
        workerNames: ["reviewer"],
        workstationNames: ["Review"],
      }),
      name: "executor-slot",
    });

    expect(renamedFactory?.resources?.[0]?.name).toBe("executor-slot");
    expect(renamedFactory?.workers?.[0]?.resources).toEqual([
      { capacity: 1, name: "executor-slot" },
    ]);
    expect(renamedFactory?.workstations?.[0]?.resources).toEqual([
      { capacity: 1, name: "executor-slot" },
    ]);
  });
});

describe("parseResourceCapacityText", () => {
  it("parses integer capacity values and rejects invalid input", () => {
    expect(parseResourceCapacityText("4")).toBe(4);
    expect(parseResourceCapacityText(" 4 ")).toBe(4);
    expect(parseResourceCapacityText("")).toBeNull();
    expect(parseResourceCapacityText("1.5")).toBeNull();
    expect(parseResourceCapacityText("abc")).toBeNull();
  });
});
