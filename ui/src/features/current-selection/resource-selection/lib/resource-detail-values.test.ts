import {
  findResourceInFactoryDefinition,
  resourceAvailablePlaceId,
  resourceShowsModelFields,
  resourceShowsProviderQuotaFields,
  resourceTokenCountFromSnapshot,
  workerNamesReferencingResourceInFactoryDefinition,
  workstationNamesReferencingResourceInFactoryDefinition,
} from "./resource-detail-values";

describe("resource-detail-values", () => {
  const factory = {
    name: "factory",
    resources: [
      {
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT" as const,
      },
      {
        backend: "llama.cpp",
        capacity: 1,
        loadPolicy: "ON_DEMAND",
        model: "OMNIVOICE_Q4_K_M",
        name: "voice-model",
        type: "MODEL" as const,
      },
    ],
    workers: [
      {
        model: "gpt-5",
        modelProvider: "CODEX" as const,
        name: "reviewer",
        resources: [{ capacity: 1, name: "agent-slot" }],
        type: "MODEL_WORKER" as const,
      },
    ],
    workstations: [
      {
        id: "review",
        name: "Review",
        resources: [{ capacity: 1, name: "agent-slot" }],
        worker: "reviewer",
      },
      {
        id: "voice",
        name: "Voice",
        resources: [{ capacity: 1, name: "voice-model" }],
        worker: "reviewer",
      },
    ],
    workTypes: [],
  };

  it("finds resources by authored name in the factory document", () => {
    expect(findResourceInFactoryDefinition(factory, "agent-slot")).toEqual({
      capacity: 2,
      name: "agent-slot",
      type: "INVOCATION_SLOT",
    });
  });

  it("lists workers and workstations that reference the resource", () => {
    expect(
      workerNamesReferencingResourceInFactoryDefinition(factory, "agent-slot"),
    ).toEqual(["reviewer"]);
    expect(
      workstationNamesReferencingResourceInFactoryDefinition(
        factory,
        "agent-slot",
      ),
    ).toEqual(["Review"]);
  });

  it("returns undefined when the resource is missing from the factory document", () => {
    expect(findResourceInFactoryDefinition(factory, "missing")).toBeUndefined();
  });

  it("classifies model and provider quota resources for conditional field display", () => {
    expect(
      resourceShowsModelFields({
        capacity: 1,
        model: "OMNIVOICE_Q4_K_M",
        name: "voice-model",
        type: "MODEL",
      }),
    ).toBe(true);
    expect(
      resourceShowsProviderQuotaFields({
        capacity: 1,
        name: "anthropic-quota",
        provider: "anthropic",
        type: "PROVIDER_QUOTA",
      }),
    ).toBe(true);
    expect(
      resourceShowsModelFields({
        capacity: 2,
        name: "agent-slot",
        type: "INVOCATION_SLOT",
      }),
    ).toBe(false);
  });

  it("returns null token counts when snapshot data is unavailable", () => {
    expect(resourceTokenCountFromSnapshot(null, "agent-slot")).toBeNull();
    expect(resourceTokenCountFromSnapshot(undefined, "agent-slot")).toBeNull();
  });

  it("resolves runtime token counts from the resource available place id", () => {
    expect(resourceAvailablePlaceId("agent-slot")).toBe("agent-slot:available");
    expect(
      resourceTokenCountFromSnapshot(
        {
          factory: { resources: factory.resources },
          runtime: {
            place_token_counts: {
              "agent-slot:available": 1,
            },
          },
        } as never,
        "agent-slot",
      ),
    ).toBe(1);
    expect(
      resourceTokenCountFromSnapshot(
        {
          factory: { resources: factory.resources },
          runtime: { place_token_counts: {} },
        } as never,
        "agent-slot",
      ),
    ).toBeNull();
  });
});
